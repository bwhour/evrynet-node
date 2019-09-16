package core

import (
	"bytes"
	"sync"

	"github.com/evrynet-official/evrynet-client/common"
	"github.com/evrynet-official/evrynet-client/consensus/tendermint"
	"github.com/evrynet-official/evrynet-client/core/types"
	"github.com/evrynet-official/evrynet-client/event"
	"github.com/evrynet-official/evrynet-client/log"
	"github.com/evrynet-official/evrynet-client/rlp"
)

const (
	msgCommit uint64 = iota
)

// New creates an Tendermint consensus core
func New(backend tendermint.Backend, config *tendermint.Config) Engine {
	c := &core{
		handlerWg:     new(sync.WaitGroup),
		backend:       backend,
		timeout:       NewTimeoutTicker(),
		config:        config,
		mu:            &sync.RWMutex{},
		blockFinalize: new(event.TypeMux),
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
	//mutex mark critical section of core which should not be accessed parralel
	mu *sync.RWMutex
}

func (c *core) EventMux() *event.TypeMux {
	return c.blockFinalize
}

// Start implements core.Engine.Start

func (c *core) Start() error {
	// Tests will handle events itself, so we have to make subscribeEvents()
	// be able to call in test.
	c.currentState = c.getStoredState()
	c.subscribeEvents()
	if err := c.timeout.Start(); err != nil {
		return err
	}
	go c.handleEvents()
	c.startRoundZero()
	return nil
}

// Stop implements core.Engine.Stop
func (c *core) Stop() error {
	c.unsubscribeEvents()
	c.handlerWg.Wait()
	return nil
}

// PrepareCommittedSeal returns a committed seal for the given hash
func PrepareCommittedSeal(hash common.Hash) []byte {
	var buf bytes.Buffer
	buf.Write(hash.Bytes())
	buf.Write([]byte{byte(msgCommit)})
	return buf.Bytes()
}

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
func (c *core) SendPropose(propose *tendermint.Proposal) {
	//TODO: remove these log in production
	log.Debug("prepare to send proposal", "proposal", propose)
	msgData, err := rlp.EncodeToBytes(propose)
	if err != nil {
		log.Error("Failed to encode Proposal to bytes", "error", err)
		return
	}
	payload, err := c.FinalizeMsg(&message{
		Code: msgPropose,
		Msg:  msgData,
	})
	if err != nil {
		log.Error("Failed to Finalize Proposal", "error", err)
		return
	}

	if err := c.backend.Broadcast(c.valSet, payload); err != nil {
		log.Error("Failed to Broadcast proposal", "error", err)
		return
	}
	//TODO: remove this log in production
	log.Debug("sent proposal", "proposal", propose)
}

func (c *core) SetBlockForProposal(b *types.Block) {
	c.CurrentState().SetBlock(b)
}

//SendVote send broadcast its vote to the network
//it only accept 2 voteType: msgPrevote and msgcommit
func (c *core) SendVote(voteType uint64, block *types.Block, round int64) {
	//This should never happen, but it is a safe guard
	if i, _ := c.valSet.GetByAddress(c.backend.Address()); i == -1 {
		log.Debug("this node is not a validator of this round, skipping vote", "address", c.backend.Address().String(), "round", round)
		return
	}
	if voteType != msgPrevote && voteType != msgCommit {
		return
	}
	var blockHash = emptyBlockHash
	if block != nil {
		blockHash = block.Hash()
	}

	vote := &tendermint.Vote{
		BlockHash:   &blockHash,
		Round:       round,
		BlockNumber: c.CurrentState().BlockNumber(),
	}
	msgData, err := rlp.EncodeToBytes(vote)
	if err != nil {
		log.Error("Failed to encode Vote to bytes", "error", err)
		return
	}
	payload, err := c.FinalizeMsg(&message{
		Code: voteType,
		Msg:  msgData,
	})
	if err != nil {
		log.Error("Failed to Finalize Vote", "error", err)
		return
	}
	if err := c.backend.Broadcast(c.valSet, payload); err != nil {
		log.Error("Failed to Broadcast vote", "error", err)
		return
	}
	log.Debug("sent vote", "vote", vote)
}

func (c *core) CurrentState() *roundState {
	return c.currentState
}