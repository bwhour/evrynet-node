package backend

import (
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/tests_utils"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/event"
	"github.com/Evrynetlabs/evrynet-node/log"
	"github.com/Evrynetlabs/evrynet-node/p2p"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

func TestHandleMsg(t *testing.T) {
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr       = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		validators     = []common.Address{
			nodeAddr,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
	)

	//create New test backend and newMockChain
	be := mustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)

	// generate one msg
	data := []byte("data1")
	msg := makeMsg(consensus.TendermintMsg, data)
	addr := tests_utils.GetAddress()

	// 2. this message should be in cache after we handle it
	handled, err := be.HandleMsg(addr, msg)
	if err != nil {
		t.Errorf("expected message being handled successfully but got %s", err)
	}
	if !handled {
		t.Errorf("expected message not being handled")
	}
}

func makeMsg(msgcode uint64, data interface{}) p2p.Msg {
	size, r, _ := rlp.EncodeToReader(data)
	return p2p.Msg{Code: msgcode, Size: uint32(size), Payload: r}
}

// mockCore is similar to real core with fixed time for processing each request
// mockCore also has 'numMsg' variable for testing
type mockCore struct {
	be        tendermint.Backend
	handlerWg sync.WaitGroup
	events    *event.TypeMuxSubscription
	numMsg    int64
}

func NewMockCore(be tendermint.Backend) *mockCore {
	return &mockCore{
		be: be,
	}
}

func (m *mockCore) Start() error {

	log.Debug("core start")
	m.events = m.be.EventMux().Subscribe(tendermint.MessageEvent{})
	go m.handleEvents()
	return nil
}

func (m *mockCore) handleEvents() {
	defer func() {
		m.handlerWg.Done()
	}()
	m.handlerWg.Add(1)
	for event := range m.events.Chan() {
		switch ev := event.Data.(type) {
		case tendermint.MessageEvent:
			_ = ev
			log.Debug("handling event", "payload", string(ev.Payload))
			time.Sleep(time.Millisecond)
			atomic.AddInt64(&m.numMsg, 1)
		default:
			panic("unexpected type")
		}
	}
	log.Debug("exit loop")
}

func (m *mockCore) Stop() error {
	m.events.Unsubscribe()
	m.handlerWg.Wait()
	return nil
}

func (m *mockCore) SetBlockForProposal(block *types.Block) {
	panic("implement me")
}

// This test case is when user start miner then stop it before core handles all msg in storingMsgs
func TestBackend_HandleMsg(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))

	be, blockchain, _, err := createBlockchainAndBackendFromGenesis(FixedValidators)
	require.NoError(t, err)
	mockCore := NewMockCore(be)
	be.core = mockCore

	count := 0
	// send msg when core is not started
	numMsg := 10
	for i := 0; i < numMsg; i++ {
		_, err := be.HandleMsg(common.Address{}, makeMsg(consensus.TendermintMsg, []byte(strconv.FormatInt(int64(count), 10))))
		count += 1
		require.NoError(t, err)
	}
	// start core
	require.NoError(t, be.Start(blockchain, blockchain.CurrentBlock, nil))
	// trigger to  dequeue and replay msg
	_, err = be.HandleMsg(common.Address{}, makeMsg(consensus.TendermintMsg, []byte(strconv.FormatInt(int64(count), 10))))
	count += 1
	require.NoError(t, err)
	time.Sleep(time.Millisecond)
	// immediately stop core
	require.NoError(t, be.Stop())

	require.NoError(t, be.Start(blockchain, blockchain.CurrentBlock, nil))
	_, err = be.HandleMsg(common.Address{}, makeMsg(consensus.TendermintMsg, []byte(strconv.FormatInt(int64(count), 10))))
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 16)
	require.Equal(t, int64(numMsg+2), mockCore.numMsg)
}

// test double start-stop is not blocking
func TestBackend_StartStop(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))

	be, blockchain, _, err := createBlockchainAndBackendFromGenesis(FixedValidators)
	require.NoError(t, err)
	mockCore := NewMockCore(be)
	be.core = mockCore
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(time.Second * 10):
			panic("timeout")
		case <-done:
			return
		}
	}()

	require.NoError(t, be.Start(blockchain, blockchain.CurrentBlock, nil))
	require.Error(t, be.Start(blockchain, blockchain.CurrentBlock, nil))
	require.NoError(t, be.Stop())
	require.NoError(t, be.Stop())
	close(done)
}

// test dequeue message loop does not work when close backend
func TestBackend_StartClose(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))

	be, blockchain, _, err := createBlockchainAndBackendFromGenesis(FixedValidators)
	require.NoError(t, err)
	mockCore := NewMockCore(be)
	be.core = mockCore
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(time.Second * 10):
			panic("timeout")
		case <-done:
			return
		}
	}()

	require.NoError(t, be.Start(blockchain, blockchain.CurrentBlock, nil))
	require.Error(t, be.Start(blockchain, blockchain.CurrentBlock, nil))
	require.NoError(t, be.Close())

	// Do not log out any "replay msg started" when backend receive message
	_, err = be.HandleMsg(common.Address{}, makeMsg(consensus.TendermintMsg, []byte("data1")))
	require.NoError(t, err)

	// Do not log out any "replay msg started" when backend receive message
	_, err = be.HandleMsg(common.Address{}, makeMsg(consensus.TendermintMsg, []byte("data2")))
	require.NoError(t, err)
	time.Sleep(2 * time.Second)

	close(done)
}
