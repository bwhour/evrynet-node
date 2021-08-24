package core

import (
	"crypto/ecdsa"
	"math/big"
	"sync"
	"testing"

	"github.com/Workiva/go-datastructures/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/tests_utils"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/event"
	"github.com/Evrynetlabs/evrynet-node/params"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

func newTestCore(backend tendermint.Backend, config *tendermint.Config) *core {
	return &core{
		handlerWg:      new(sync.WaitGroup),
		backend:        backend,
		timeout:        NewTimeoutTicker(),
		config:         config,
		mu:             &sync.RWMutex{},
		blockFinalize:  new(event.TypeMux),
		futureMessages: queue.NewPriorityQueue(0, true),
		sentMsgStorage: NewMsgStorage(),
		rebroadcast:    false,
	}
}

func TestVerifyProposal(t *testing.T) {
	var (
		nodePrivateKey     = tests_utils.MakeNodeKey()
		nodeFakePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr           = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		validators         = []common.Address{
			nodeAddr,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
		err           error
	)
	//create New test backend and newMockChain
	be, _ := tests_utils.MustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)

	core := newTestCore(be, tendermint.DefaultConfig)
	require.NoError(t, core.Start())
	// --------CASE 1--------
	// Will get errMismatchTxhashes
	block1 := tests_utils.MakeBlockWithoutSeal(genesisHeader)

	// --------CASE 2--------
	// Pass all validation
	to, _ := common.EvryAddressStringToAddressCheck("EYXe2YrqoxNYHW7tMLw63xtyZ7JB7tZf3Z")
	tx1 := types.NewTransaction(0, to, big.NewInt(10), 800000, big.NewInt(params.GasPriceConfig), nil)
	tx1, err = types.SignTx(tx1, types.HomesteadSigner{}, nodePrivateKey)
	assert.NoError(t, err)

	block2 := types.NewBlock(genesisHeader, []*types.Transaction{tx1}, []*types.Header{}, []*types.Receipt{})
	assert.Len(t, block2.Transactions(), 1)
	assert.Equal(t, tx1.Hash(), block2.Transactions()[0].Hash())

	// --------CASE 3--------
	// Will get ErrInsufficientFunds
	tx2 := types.NewTransaction(0, to, big.NewInt(10), params.GasPriceConfig, big.NewInt(params.GasPriceConfig), nil)
	tx2, err = types.SignTx(tx2, types.HomesteadSigner{}, nodePrivateKey)
	assert.NoError(t, err)

	block3 := types.NewBlock(genesisHeader, []*types.Transaction{tx2}, []*types.Header{}, []*types.Receipt{})
	assert.Len(t, block3.Transactions(), 1)
	assert.Equal(t, tx2.Hash(), block3.Transactions()[0].Hash())

	// --------CASE 4--------
	// Node propose fake block1 (fake signature)
	// backend.VerifyHeader() will return error
	tx3 := types.NewTransaction(0, to, big.NewInt(10), 800000, big.NewInt(params.GasPriceConfig), nil)
	tx3, err = types.SignTx(tx3, types.HomesteadSigner{}, nodePrivateKey)
	assert.NoError(t, err)

	block4 := types.NewBlock(genesisHeader, []*types.Transaction{tx3}, []*types.Header{}, []*types.Receipt{})
	assert.Len(t, block4.Transactions(), 1)
	assert.Equal(t, tx3.Hash(), block4.Transactions()[0].Hash())

	for _, testCase := range []struct {
		block      *types.Block
		privateKey *ecdsa.PrivateKey
		assertFn   func(err error)
	}{
		{
			block:      block1,
			privateKey: nodePrivateKey,
			assertFn: func(err error) {
				assert.EqualError(t, err, tendermint.ErrMismatchTxhashes.Error())
			},
		},
		{
			block:      block2,
			privateKey: nodePrivateKey,
			assertFn: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			block:      block3,
			privateKey: nodePrivateKey,
			assertFn: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			block:      block4,
			privateKey: nodeFakePrivateKey,
			assertFn: func(err error) {
				assert.EqualError(t, err, ErrInvalidProposalSignature.Error())
			},
		},
	} {
		proposal := Proposal{
			Block: testCase.block,
			Round: 1,
		}

		msgData, err := rlp.EncodeToBytes(&proposal)
		require.NoError(t, err)

		// Create fake message from another node address
		msg := message{
			Code:    0,
			Msg:     msgData,
			Address: crypto.PubkeyToAddress(nodePrivateKey.PublicKey),
		}

		msgPayLoadWithoutSignature, err := rlp.EncodeToBytes(&message{
			Code:      msg.Code,
			Address:   msg.Address,
			Msg:       msg.Msg,
			Signature: []byte{},
		})
		require.NoError(t, err)

		signature, err := crypto.Sign(crypto.Keccak256(msgPayLoadWithoutSignature), testCase.privateKey)
		require.NoError(t, err)
		msg.Signature = signature
		testCase.assertFn(core.VerifyProposal(proposal, msg))
	}
}

func TestCore_HandleMsg(t *testing.T) {
	var (
		nodePrivateKey     = tests_utils.MakeNodeKey()
		nodeFakePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr           = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		validators         = []common.Address{
			nodeAddr,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
		err           error
	)
	//create New test backend and newMockChain
	be, _ := tests_utils.MustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)

	core := newTestCore(be, tendermint.DefaultConfig)
	msg := message{
		Msg:     []byte("aaaa"),
		Address: nodeAddr,
		Code:    msgPrevote,
	}
	err = core.handleMsg(msg)
	require.Error(t, err, msg)

	rawPayload, err := msg.PayLoadWithoutSignature()
	require.NoError(t, err)
	hashData := crypto.Keccak256(rawPayload)
	signature, err := crypto.Sign(hashData, nodeFakePrivateKey)
	msg.Signature = signature
	err = core.handleMsg(msg)
	require.EqualError(t, err, ErrSignerMessageMissMatch.Error())
}
