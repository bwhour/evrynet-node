package backend

import (
	"crypto/ecdsa"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/tests_utils"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/validator"
	evrynetCore "github.com/Evrynetlabs/evrynet-node/core"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/event"
	"github.com/Evrynetlabs/evrynet-node/log"
	"github.com/Evrynetlabs/evrynet-node/params"
)

func TestSign(t *testing.T) {
	privateKey, err := tests_utils.GeneratePrivateKey()
	require.NoError(t, err)
	b := &Backend{
		privateKey: privateKey,
	}
	data := []byte("Here is a string....")
	sig, err := b.Sign(data)
	require.NoError(t, err)
	// Check signature recover
	hashData := crypto.Keccak256([]byte(data))
	pubkey, _ := crypto.Ecrecover(hashData, sig)
	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])

	// Get Address from private key
	publicKeyECDSA, ok := privateKey.Public().(*ecdsa.PublicKey)
	require.True(t, ok)
	address := crypto.PubkeyToAddress(*publicKeyECDSA)
	assert.Equal(t, signer, address, "address mismatch")
}

func TestValidators(t *testing.T) {
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr       = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		validators     = []common.Address{
			nodeAddr,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
		be            = mustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)
	)

	valSet0 := be.Validators(big.NewInt(0))
	assert.Equal(t, 1, valSet0.Size())

	list := valSet0.List()
	log.Info("validator set of block 0 is")

	for _, val := range list {
		log.Info(val.String())
	}

	valSet1 := be.Validators(big.NewInt(1))
	assert.Equal(t, 1, valSet1.Size())

	list = valSet1.List()
	log.Info("validator set of block 1 is")

	for _, val := range list {
		log.Info(val.String())
	}

	valSet2 := be.Validators(big.NewInt(2))
	assert.Equal(t, 1, valSet2.Size())
}

func mustCreateAndStartNewBackend(t *testing.T, nodePrivateKey *ecdsa.PrivateKey, genesisHeader *types.Header, validators []common.Address) *Backend {
	var (
		address = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		trigger = false
		statedb = tests_utils.MustCreateStateDB(t)

		testTxPoolConfig evrynetCore.TxPoolConfig
		blockchain       = &tests_utils.MockChainReader{
			GenesisHeader: genesisHeader,
			MockBlockChain: &tests_utils.MockBlockChain{
				Statedb:       statedb,
				GasLimit:      1000000000,
				ChainHeadFeed: new(event.Feed),
			},
			Address: address,
			Trigger: &trigger,
		}
		pool   = evrynetCore.NewTxPool(testTxPoolConfig, params.TendermintTestChainConfig, blockchain)
		config = tendermint.DefaultConfig
	)

	config.FixedValidators = validators
	be := New(config, nodePrivateKey).(*Backend)
	statedb.SetBalance(address, new(big.Int).SetUint64(params.Ether))
	defer pool.Stop()
	be.chain = blockchain
	be.currentBlock = blockchain.CurrentBlock

	return be
}

type mockBroadcaster struct {
	handleFn     func(interface{}) error
	isDisconnect bool
	isSendFailed bool
}

// FindPeers returns a map of mockPeer but only one with trigger HandleMsg
func (m *mockBroadcaster) FindPeers(targets map[common.Address]bool) map[common.Address]consensus.Peer {
	if m.isDisconnect {
		return nil
	}
	out := make(map[common.Address]consensus.Peer)

	if m.isSendFailed {
		for addr := range targets {
			out[addr] = &tests_utils.MockPeer{SendFn: func(data interface{}) error {
				return errors.New("test send failed")
			}}
		}

		return out
	}

	hasHandle := false
	for addr := range targets {
		if !hasHandle {
			out[addr] = &tests_utils.MockPeer{SendFn: m.handleFn}
			hasHandle = true
			continue
		}
		out[addr] = &tests_utils.MockPeer{}
	}
	return out
}

func (m *mockBroadcaster) Enqueue(id string, block *types.Block) {
	panic("implement me")
}

func TestBackend_Gossip(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr       = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		validators     = []common.Address{
			nodeAddr,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
		be            = mustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)
		a, _          = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8fmHkiJ")
		b, _          = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8hGraaK")
		c, _          = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8pZptqN")
		nodeAddrs     = []common.Address{
			a,
			b,
			c,
			nodeAddr,
		}
		expectedData = "aaa"
	)

	be.coreStarted = true
	dataCh := make(chan string)

	broadcaster := &mockBroadcaster{
		handleFn: func(data interface{}) error {
			dataCh <- string(data.([]byte))
			return nil
		},
		isDisconnect: false,
		isSendFailed: false,
	}
	be.SetBroadcaster(broadcaster)
	valSet := validator.NewSet(nodeAddrs, tendermint.RoundRobin, 100)

	//test basic
	require.NoError(t, be.Gossip(valSet, big.NewInt(0), 0, 0, []byte(expectedData)))
	select {
	case <-time.After(time.Millisecond * 20):
		t.Fatal("not receive msg to peer")
	case data := <-dataCh:
		assert.Equal(t, expectedData, data)
	}

	//test retrying broadcast data
	broadcaster.isDisconnect = true
	err := be.Gossip(valSet, big.NewInt(0), 0, 0, []byte(expectedData))
	require.NoError(t, err)
	select {
	case <-time.After(time.Millisecond * 80):
	case <-dataCh:
		t.Fatal("expected not send to peer when disconnect")
	}

	broadcaster.isDisconnect = false
	select {
	case <-time.After(time.Millisecond * 40):
		t.Fatal("not receive msg to peer")
	case data := <-dataCh:
		assert.Equal(t, expectedData, data)
	}

	//test not passed when sending failed
	broadcaster.isSendFailed = true
	require.NoError(t, be.Gossip(valSet, big.NewInt(0), 0, 0, []byte(expectedData)))

	select {
	case <-time.After(time.Millisecond * 80):
	case <-dataCh:
		t.Fatal("expected not send to peer when disconnect")
	}

	broadcaster.isSendFailed = false
	select {
	case <-time.After(time.Millisecond * 40):
		t.Fatal("not receive msg to peer")
	case data := <-dataCh:
		assert.Equal(t, expectedData, data)
	}

	// test gossip is cancelled when new head event
	broadcaster.isDisconnect = true
	require.NoError(t, be.Gossip(valSet, big.NewInt(1), 0, 0, []byte(expectedData)))
	select {
	case <-time.After(time.Millisecond * 80):
	case <-dataCh:
		t.Fatal("expected not send to peer when disconnect")
	}

	require.NoError(t, be.HandleNewChainHead(big.NewInt(2)))
	broadcaster.isDisconnect = false
	select {
	case <-time.After(time.Millisecond * 40):
	case <-dataCh:
		t.Fatal("broadcast task is not cancel as expected")
	}

}

func TestBackend_Multicast(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr       = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		validators     = []common.Address{
			nodeAddr,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
		be            = mustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)

		//nodeAddrs = []common.Address{
		//	common.HexToAddress("1"),
		//	common.HexToAddress("2"),
		//	common.HexToAddress("3"),
		//	nodeAddr,
		//}

		a, _      = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8fmHkiJ")
		b, _      = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8hGraaK")
		sentAddrs = map[common.Address]bool{
			a: true,
			b: true,
		}
		expectedData = "aaa"
	)

	dataCh := make(chan string)

	broadcaster := &mockBroadcaster{
		handleFn: func(data interface{}) error {
			dataCh <- string(data.([]byte))
			return nil
		},
		isDisconnect: false,
		isSendFailed: false,
	}
	be.SetBroadcaster(broadcaster)
	go func() {
		require.NoError(t, be.Multicast(sentAddrs, []byte(expectedData)))
	}()

	select {
	case <-time.After(time.Millisecond * 200):
		t.Fatal("not receive msg to peer")
	case data := <-dataCh:
		assert.Equal(t, expectedData, data)
	}

	broadcaster.isDisconnect = true
	require.EqualError(t, be.Multicast(sentAddrs, []byte(expectedData)), "failed to multicast: failed to send 0 address, not found 2 address")

	broadcaster.isDisconnect = false
	broadcaster.isSendFailed = true
	require.EqualError(t, be.Multicast(sentAddrs, []byte(expectedData)), "failed to multicast: failed to send 2 address, not found 0 address")
}
