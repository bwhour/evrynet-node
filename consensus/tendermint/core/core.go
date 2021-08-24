package core

import (
	"math/big"
	"sync"
	"time"

	"github.com/Workiva/go-datastructures/queue"
	"go.uber.org/zap"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/utils"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/event"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

type Option func(c *core) error

//WithoutRebroadcast return an option to set whether or not core will rebroadcast its message
func WithoutRebroadcast() Option {
	return func(c *core) error {
		c.rebroadcast = false
		return nil
	}
}

// New creates an Tendermint consensus core
func New(backend tendermint.Backend, config *tendermint.Config, opts ...Option) Engine {
	c := &core{
		handlerWg:       new(sync.WaitGroup),
		backend:         backend,
		timeout:         NewTimeoutTicker(),
		config:          config,
		mu:              &sync.RWMutex{},
		blockFinalize:   new(event.TypeMux),
		futureMessages:  queue.NewPriorityQueue(0, true),
		futureProposals: make(map[int64]message),
		sentMsgStorage:  NewMsgStorage(),
		rebroadcast:     true,
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			panic(err)
		}
	}
	return c
}

// ----------------------------------------------------------------------------

type core struct {
	//backend implement tendermint.Backend
	//this component will send/receive data to other nodes and other components
	backend tendermint.Backend
	//events is the channel to receives 2 types of event:
	//- NewBlockEvent: when there is a new composed block from Tx_pool
	//- MessageEvent: when there is a new message from other validators/ peers
	events *event.TypeMuxSubscription

	// finalCommitted events is the chanel to raise when committed and run to new round
	finalCommitted *event.TypeMuxSubscription

	//BlockFinalizeEvent
	blockFinalize *event.TypeMux
	//handleWg will help core stop gracefully, i.e, core will wait till handlingEvents done before reutrning.
	handlerWg *sync.WaitGroup

	//valSet keep track of the current core's validator set.
	valSet tendermint.ValidatorSet // validators set
	//currentState store the state of current consensus
	//it contain round/ block number as well as how many votes this machine has received.
	currentState *roundState

	//timeout will schedule all timeout requirement and fire the timeout event once it's finished.
	timeout TimeoutTicker
	//config store the config of the chain
	config *tendermint.Config
	//mutex mark critical section of core which should not be accessed parallel
	mu *sync.RWMutex

	// a Helper supports to store message before send proposal/ vote for every block
	sentMsgStorage *msgStorage

	//proposeStart mark the time core enter propose. This is purely use for metrics
	proposeStart time.Time

	// futureMessages stores future messages (prevote and precommit) fromo other peers
	// and handle them later when we jump to that block number
	// futureMessages only accepts msgItem
	futureMessages *queue.PriorityQueue

	// futureProposals stores future proposal which is ahead in round from current state
	// In case: the current node is still at precommit but another node jumps to next round and sends the proposal
	futureProposals map[int64]message

	rebroadcast bool
}

// Start implements core.Engine.Start
// Note: this function is not thread-safe
func (c *core) Start() error {
	// Tests will handle events itself, so we have to make subscribeEvents()
	// be able to call in test.
	c.getLogger().Infow("starting Tendermint's core...")
	if c.currentState == nil {
		c.currentState = c.getInitializedState()
		c.valSet = c.backend.Validators(c.CurrentState().BlockNumber())
	}
	c.subscribeEvents()

	// Tests will handle events itself, so we have to make subscribeEvents()
	// be able to call in test.
	if err := c.timeout.Start(); err != nil {
		return err
	}
	c.startNewRound()
	go c.handleEvents()

	return nil
}

// Stop implements core.Engine.Stop
// Note: this function is not thread-safe
func (c *core) Stop() error {
	c.getLogger().Infow("stopping Tendermint's timeout core...")
	err := c.timeout.Stop()
	c.unsubscribeEvents()
	c.handlerWg.Wait()
	c.getLogger().Infow("Tendermint's timeout core stopped")
	return err
}

//FinalizeMsg set address, signature and encode msg to bytes
func (c *core) FinalizeMsg(msg *message) ([]byte, error) {
	msg.Address = c.backend.Address()
	msgPayLoadWithoutSignature, err := msg.PayLoadWithoutSignature()
	if err != nil {
		return nil, err
	}
	signature, err := c.backend.Sign(msgPayLoadWithoutSignature)
	if err != nil {
		return nil, err
	}
	msg.Signature = signature
	return rlp.EncodeToBytes(msg)
}

//SendPropose will Finalize the Proposal in term of signature and
//Gossip it to other nodes
func (c *core) SendPropose(propose *Proposal) {
	logger := c.getLogger().With("propose_round", propose.Round,
		"propose_block_number", propose.Block.Number(), "propose_block_hash", propose.Block.Hash())

	msgData, err := rlp.EncodeToBytes(propose)
	if err != nil {
		logger.Errorw("Failed to encode Proposal to bytes", "error", err)
		return
	}
	payload, err := c.FinalizeMsg(&message{
		Code: msgPropose,
		Msg:  msgData,
	})
	if err != nil {
		logger.Errorw("Failed to Finalize Proposal", "error", err)
		return
	}

	// store before send propose msg
	c.sentMsgStorage.storeSentMsg(c.getLogger(), RoundStepPropose, propose.Round, payload)

	if err := c.backend.Broadcast(c.valSet, c.currentState.CopyBlockNumber(), propose.Round, msgPropose, payload); err != nil {
		c.getLogger().Errorw("Failed to Broadcast proposal", "error", err)
		return
	}
	//TODO: remove this log in production
	logger.Infow("sent proposal")
}

//SetBlockForProposal define a method to allow Injecting a Block for testing purpose
func (c *core) SetBlockForProposal(b *types.Block) {
	c.CurrentState().SetBlock(b)
}

//SendVote send broadcast its vote to the network
//it only accept 2 voteType: msgPrevote and msgcommit
func (c *core) SendVote(voteType uint64, block *types.Block, round int64) {
	logger := c.getLogger().With("send_vote_type", voteType, "send_vote_round", round)
	if voteType != msgPrevote && voteType != msgPrecommit {
		logger.Errorw("vote type is invalid")
		return
	}
	var (
		blockHash = emptyBlockHash
		seal      []byte
	)
	if block != nil {
		var err error
		commitHash := utils.PrepareCommittedSeal(block.Header().Hash())
		seal, err = c.backend.Sign(commitHash)
		if err != nil {
			logger.Errorw("failed to sign seal", err, "err")
			return
		}
		blockHash = block.Hash()
	}
	vote := &Vote{
		BlockHash:   &blockHash,
		Round:       round,
		BlockNumber: c.CurrentState().BlockNumber(),
		Seal:        seal,
	}
	msgData, err := rlp.EncodeToBytes(vote)
	if err != nil {
		logger.Errorw("Failed to encode Vote to bytes", "error", err)
		return
	}
	payload, err := c.FinalizeMsg(&message{
		Code: voteType,
		Msg:  msgData,
	})
	if err != nil {
		logger.Errorw("Failed to Finalize Vote", "error", err)
		return
	}

	// store before send propose msg
	switch voteType {
	case msgPrevote:
		c.sentMsgStorage.storeSentMsg(c.getLogger(), RoundStepPrevote, round, payload)
	case msgPrecommit:
		c.sentMsgStorage.storeSentMsg(c.getLogger(), RoundStepPrecommit, round, payload)
	default:
	}

	if err := c.backend.Broadcast(c.valSet, c.currentState.CopyBlockNumber(), round, voteType, payload); err != nil {
		logger.Errorw("Failed to Broadcast vote", "error", err)
		return
	}
	logger.Infow("sent vote", "vote_round", vote.Round, "vote_block_number", vote.BlockNumber, "vote_block_hash", vote.BlockHash.Hex())
}

// SendCatchupReply sends catchup reply to target node
func (c *core) SendCatchupReply(target common.Address, payloads [][]byte) {
	logger := c.getLogger().With("num_msg", len(payloads), "target", target.String())
	catchUpReplyMsg := &CatchUpReplyMsg{
		BlockNumber: new(big.Int).Set(c.CurrentState().BlockNumber()),
		Payloads:    payloads,
	}
	msgData, err := rlp.EncodeToBytes(catchUpReplyMsg)
	if err != nil {
		logger.Errorw("Failed to encode Vote to bytes", "error", err)
		return
	}
	payload, err := c.FinalizeMsg(&message{
		Code: msgCatchUpReply,
		Msg:  msgData,
	})
	if err != nil {
		logger.Errorw("Failed to Finalize Vote", "error", err)
		return
	}
	if err := c.backend.Multicast(map[common.Address]bool{target: true}, payload); err != nil {
		logger.Errorw("Failed to send catchUpReply msgs", "err", err)
		return
	}
	logger.Infow("Reply catch up msgs")
}

func (c *core) CurrentState() *roundState {
	return c.currentState
}

// getLogger returns a zap logger with state info
func (c *core) getLogger() *zap.SugaredLogger {
	if c.currentState == nil {
		return zap.S()
	}
	return zap.L().With(
		zap.Stringer("block", c.currentState.BlockNumber()),
		zap.Int64("round", c.currentState.Round()),
		zap.Stringer("step", c.currentState.Step())).Sugar()
}

// address returns address of current nodes
func (c *core) getAddress() common.Address {
	return c.backend.Address()
}
