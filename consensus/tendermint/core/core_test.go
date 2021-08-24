package core

import (
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/tests_utils"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/event"
	"github.com/Evrynetlabs/evrynet-node/params"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

func TestRecoverCoreTimeoutWithNewHeight(t *testing.T) {
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr       = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		validators     = []common.Address{
			nodeAddr,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
	)
	//create New test backend and newMockChain
	be, _ := tests_utils.MustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)

	core := newTestCore(be, tendermint.DefaultConfig)
	require.NoError(t, core.Start())
	state := core.CurrentState()
	assert.Equal(t, RoundStepNewHeight, state.Step())

	state.UpdateRoundStep(0, RoundStepNewHeight)
	core.currentState = state
	require.NoError(t, core.Stop())
	time.Sleep(2 * time.Second)
	require.NoError(t, core.Start())

	//wait for 4 second to core's state jump from RoundStepPropose to RoundStepPrevote
	time.Sleep(1 * time.Second)
	assert.Equal(t, RoundStepPropose, core.CurrentState().Step())
}

func TestRecoverCoreTimeoutWithPropose(t *testing.T) {
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr       = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		nodeAddr2, _   = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8XVJyV9")
		validators     = []common.Address{
			nodeAddr,
			nodeAddr2,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
	)
	//create New test backend and newMockChain
	be, _ := tests_utils.MustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)

	core := newTestCore(be, tendermint.DefaultConfig)
	require.NoError(t, core.Start())
	state := core.CurrentState()
	assert.Equal(t, RoundStepNewHeight, state.Step())

	state.UpdateRoundStep(0, RoundStepPropose)
	core.currentState = state
	require.NoError(t, core.Stop())
	time.Sleep(2 * time.Second)
	require.NoError(t, core.Start())

	//wait for 4 second to core's state jump from RoundStepPropose to RoundStepPrevote
	time.Sleep(tendermint.DefaultConfig.TimeoutPropose + (1 * time.Second))
	assert.Equal(t, RoundStepPrevote, core.CurrentState().Step())
}

func TestRecoverCoreTimeoutWithPrevoteWait(t *testing.T) {
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr       = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		nodeAddr2, _   = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8XVJyV9")
		validators     = []common.Address{
			nodeAddr,
			nodeAddr2,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
	)
	//create New test backend and newMockChain
	be, _ := tests_utils.MustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)

	core := newTestCore(be, tendermint.DefaultConfig)
	require.NoError(t, core.Start())
	state := core.CurrentState()
	assert.Equal(t, RoundStepNewHeight, state.Step())

	state.UpdateRoundStep(0, RoundStepPrevoteWait)
	require.NoError(t, core.Stop())
	time.Sleep(2 * time.Second)
	require.NoError(t, core.Start())
	assert.Equal(t, RoundStepPrevoteWait, core.CurrentState().Step())

	//wait for 4 second to core's state jump from RoundStepPrevoteWait to RoundStepPrecommit
	time.Sleep(tendermint.DefaultConfig.TimeoutPrevote + (500 * time.Millisecond))
	assert.Equal(t, RoundStepPrecommit, core.CurrentState().Step())
}

func TestRecoverCoreTimeoutWithPreCommit(t *testing.T) {
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr       = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		validators     = []common.Address{
			nodeAddr,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
	)
	//create New test backend and newMockChain
	be, _ := tests_utils.MustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)

	core := newTestCore(be, tendermint.DefaultConfig)
	require.NoError(t, core.Start())
	state := core.CurrentState()
	assert.Equal(t, RoundStepNewHeight, state.Step())

	state.UpdateRoundStep(0, RoundStepPrecommitWait)
	require.NoError(t, core.Stop())
	time.Sleep(2 * time.Second)
	require.NoError(t, core.Start())
	assert.Equal(t, RoundStepPrecommitWait, core.CurrentState().Step())

	//wait for core's state jump from RoundStepPrecommit to RoundStepPropose
	time.Sleep(tendermint.DefaultConfig.TimeoutPrecommit + (1 * time.Second))
	assert.Equal(t, RoundStepPropose, core.CurrentState().Step())
}

func TestCoreFutureMessage(t *testing.T) {
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr       = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		validators     = []common.Address{
			nodeAddr,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
		err           error
	)
	//create New test backend and newMockChain
	be, _ := tests_utils.MustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)

	core := newTestCore(be, tendermint.DefaultConfig)
	require.NoError(t, core.Start())

	logger := zap.NewExample().Sugar()
	zap.ReplaceGlobals(logger.Desugar())

	// create fake block
	to, _ := common.EvryAddressStringToAddressCheck("EPXt4N4aPgV4BEKLtYJgz2NdiAHfa7c2ao")
	tx := types.NewTransaction(0, to, big.NewInt(10), 800000, big.NewInt(params.GasPriceConfig), nil)
	tx, err = types.SignTx(tx, types.HomesteadSigner{}, nodePrivateKey)
	assert.NoError(t, err)

	header := types.CopyHeader(genesisHeader)
	header.Number = big.NewInt(2)
	block := types.NewBlock(header, []*types.Transaction{tx}, []*types.Header{}, []*types.Receipt{})
	assert.Len(t, block.Transactions(), 1)
	assert.Equal(t, tx.Hash(), block.Transactions()[0].Hash())
	// create fake proposal
	proposal := Proposal{
		Block: block,
		Round: 0,
	}
	msgData, err := rlp.EncodeToBytes(&proposal)
	require.NoError(t, err)
	msg := message{
		Code:    msgPropose,
		Msg:     msgData,
		Address: crypto.PubkeyToAddress(nodePrivateKey.PublicKey),
	}
	sign(t, &msg, nodePrivateKey)
	require.NoError(t, core.handleMsg(msg))
	// create a fake prevote
	hash := block.Hash()
	vote := Vote{
		BlockNumber: big.NewInt(2),
		Round:       0,
		BlockHash:   &hash,
		Seal:        []byte("abc"),
	}
	msgData, err = rlp.EncodeToBytes(&vote)
	require.NoError(t, err)
	prevoteMsg := message{
		Code:    msgPrevote,
		Address: crypto.PubkeyToAddress(nodePrivateKey.PublicKey),
		Msg:     msgData,
	}
	sign(t, &prevoteMsg, nodePrivateKey)
	require.NoError(t, core.handleMsg(prevoteMsg))

	_, err = core.processFutureMessages(logger)
	require.NoError(t, err)
}

// TestCore_Start_NotValidators assures that core does not jump to propose if it is not a validator
func TestCore_Start_NotValidators(t *testing.T) {
	t.Parallel()
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		validator, _   = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeWAULFYZT")
		validators     = []common.Address{
			validator,
		}
	)
	testCoreStartNewRound(t, nodePrivateKey, validators, RoundStepNewHeight)
}

// TestCore_Start_Validators assures that core jump to propose if it is a validator
func TestCore_Start_Validators(t *testing.T) {
	t.Parallel()
	var (
		nodePrivateKey = tests_utils.MakeNodeKey()
		nodeAddr       = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		validators     = []common.Address{
			nodeAddr,
		}
	)
	testCoreStartNewRound(t, nodePrivateKey, validators, RoundStepPropose)
}

func testCoreStartNewRound(t *testing.T, nodePk *ecdsa.PrivateKey, validators []common.Address, expectedStep RoundStepType) {
	var (
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
	)
	//create New test backend and newMockChain
	be, _ := tests_utils.MustCreateAndStartNewBackend(t, nodePk, genesisHeader, validators)

	core := newTestCore(be, tendermint.DefaultConfig)
	require.NoError(t, core.Start())
	state := core.CurrentState()
	assert.Equal(t, RoundStepNewHeight, state.Step())
	time.Sleep(1 * time.Second)
	assert.Equal(t, expectedStep, core.CurrentState().Step())
}

func sign(t *testing.T, msg *message, privateKey *ecdsa.PrivateKey) {
	rawPayLoad, err := msg.PayLoadWithoutSignature()
	require.NoError(t, err)
	hashData := crypto.Keccak256(rawPayLoad)
	msg.Signature, err = crypto.Sign(hashData, privateKey)
	require.NoError(t, err)
}

func assertNextMsg(t *testing.T, sentMsgSub *event.TypeMuxSubscription, msgType uint64, timeout time.Duration, assertAddress func(address common.Address), assertMsg func([]byte)) {
	select {
	case ev := <-sentMsgSub.Chan():
		sendMsgEvent := ev.Data.(tests_utils.SentMsgEvent)
		if assertAddress != nil {
			assertAddress(sendMsgEvent.Target)
		}
		var msg message
		require.NoError(t, rlp.DecodeBytes(sendMsgEvent.Payload, &msg))
		require.Equal(t, msgType, msg.Code)
		if assertMsg != nil {
			assertMsg(msg.Msg)
		}
	case <-time.After(timeout):
		panic("timeout")
	}
}
