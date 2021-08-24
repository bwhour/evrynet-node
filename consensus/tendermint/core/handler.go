package core

import (
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	evrynetCore "github.com/Evrynetlabs/evrynet-node/core"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/log"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

var (
	ErrInvalidProposalPOLRound      = errors.New("invalid proposal POL round")
	ErrInvalidProposalSignature     = errors.New("invalid proposal signature")
	ErrVoteHeightMismatch           = errors.New("vote height mismatch")
	ErrVoteInvalidValidatorAddress  = errors.New("invalid validator address")
	ErrEmptyBlockProposal           = errors.New("empty block proposal")
	ErrSignerMessageMissMatch       = errors.New("deprived signer and address field of msg are miss-match")
	ErrCatchUpReplyAddressMissMatch = errors.New("address of catch up reply msg and its child are miss match")
	emptyBlockHash                  = common.Hash{}
	catchUpReplyBatchSize           = 3 // send 3 votes as the number of msg to jump to next round
)

// ----------------------------------------------------------------------------

// Subscribe both internal and external events
func (c *core) subscribeEvents() {
	c.events = c.backend.EventMux().Subscribe(
		// external events
		tendermint.NewBlockEvent{},
		tendermint.MessageEvent{},
	)

	c.finalCommitted = c.backend.EventMux().Subscribe(
		tendermint.FinalCommittedEvent{},
	)
}

// Unsubscribe all events
func (c *core) unsubscribeEvents() {
	c.events.Unsubscribe()
	c.finalCommitted.Unsubscribe()
}

// handleEvents will receive messages as well as timeout and is solely responsible for state change.
func (c *core) handleEvents() {
	// Clear state
	defer func() {
		c.handlerWg.Done()
	}()

	c.handlerWg.Add(1)

	for {
		var logger = c.getLogger()
		select {
		case event, ok := <-c.events.Chan(): //backend sending something...
			if !ok {
				return
			}
			// A real event arrived, process interesting content
			switch ev := event.Data.(type) {
			case tendermint.NewBlockEvent:
				c.handleNewBlock(ev.Block)
			case tendermint.MessageEvent:
				//TODO: Handle ev.Payload, if got error then call c.backend.Gossip()
				var msg message
				if err := rlp.DecodeBytes(ev.Payload, &msg); err != nil {
					logger.Errorw("failed to decode msg", "error", err)
				} else {
					//log.Info("received message event", "from", msg.Address, "msg_Code", msg.Code)
					if err := c.handleMsg(msg); err != nil {
						logger.Errorw("failed to handle msg", "error", err)
					}
				}
			default:
				c.getLogger().Infow("Unknown event ", "event", ev)
			}
		case ti, ok := <-c.timeout.Chan(): //something from timeout...
			if !ok {
				return
			}
			c.handleTimeout(ti)
		case event, ok := <-c.finalCommitted.Chan():
			if !ok {
				return
			}
			switch ev := event.Data.(type) {
			case tendermint.FinalCommittedEvent:
				_ = c.handleFinalCommitted(ev.BlockNumber)
			}
		}
	}
}

// handleFinalCommitted is calling when received a final committed proposal
func (c *core) handleFinalCommitted(newHeadNumber *big.Int) error {
	var (
		state  = c.CurrentState()
		logger = c.getLogger()
	)
	c.mu.Lock()
	defer c.mu.Unlock()
	if state.BlockNumber().Cmp(newHeadNumber) > 0 {
		logger.Warnw("current state block number is ahead of new Head number. Ignore updating...",
			"current_block_number", state.BlockNumber().String(),
			"new_head_number", newHeadNumber.String())
		return nil
	}

	c.sentMsgStorage.truncateMsgStored(logger)
	c.updateStateForNewblock()
	c.startNewRound()
	if _, err := c.processFutureMessages(logger); err != nil {
		logger.Errorw("failed to process future msg", "err", err)
	}
	return nil
}

func (c *core) handleNewBlock(block *types.Block) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var (
		state  = c.CurrentState()
		logger = c.getLogger()
	)
	logger.Infow("received New Block event", "new_block_number", block.Number(), "new_block_hash", block.Hash().Hex())

	if block.Number() == nil || state.BlockNumber().Cmp(block.Number()) > 0 {
		//This is temporary to let miner come up with a newer block
		logger.Errorw("new block number is smaller than current block",
			"new_block_number", block.Number(), "state.BlockNumber", state.BlockNumber())
		//return a nil block to allow miner to send over a new one
		c.backend.Cancel(block)

		return
	}
	state.SetBlock(block)
	// in case handleNewBlock is called after enterPropose
	if state.step == RoundStepPropose {
		if i, _ := c.valSet.GetByAddress(c.backend.Address()); i == -1 {
			logger.Infow("this node is not a validator of this round", "address", c.backend.Address())
			return
		}
		if c.valSet.IsProposer(c.backend.Address()) {
			logger.Infow("this node is proposer of this round", "node_address", c.backend.Address())
			proposal := c.getDefaultProposal(logger, state.Round())
			if proposal != nil {
				c.SendPropose(proposal)
			}
		}
	}
}

//VerifyProposal validate msg & proposal when get from other nodes
func (c *core) VerifyProposal(proposal Proposal, msg message) error {
	// Verify POLRound, which must be -1 or in range [0, proposal.Round).
	if proposal.POLRound < -1 ||
		((proposal.POLRound >= 0) && proposal.POLRound >= proposal.Round) {
		return ErrInvalidProposalPOLRound
	}

	// Verify signature
	signer, err := msg.GetAddressFromSignature()
	if err != nil {
		return err
	}

	// signature must come from Proposer of this round
	if c.valSet.GetProposer().Address() != signer {
		return ErrInvalidProposalSignature
	}

	if proposal.Block == nil || (proposal.Block != nil && proposal.Block.Hash().Hex() == emptyBlockHash.Hex()) {
		return ErrEmptyBlockProposal
	}

	// verify the header of proposed block
	// ignore ErrEmptyCommittedSeals error because we don't have the committed seals yet
	if err := c.backend.VerifyProposalHeader(proposal.Block.Header()); err != nil && err != tendermint.ErrEmptyCommittedSeals {
		return err
	}

	if err := c.backend.VerifyProposalBlock(proposal.Block); err != nil {
		return err
	}

	return nil
}

func (c *core) handlePropose(msg message) error {
	var (
		state    = c.CurrentState()
		proposal Proposal
	)

	if err := rlp.DecodeBytes(msg.Msg, &proposal); err != nil {
		return err
	}
	logger := c.getLogger().With("proposal_round", proposal.Round, "proposal_block_hash", proposal.Block.Hash().Hex(),
		"proposal_block_number", proposal.Block.Number().String())
	logger.Infow("received a proposal", "from", msg.Address)

	// Already have one
	// TODO: possibly catch double proposals
	if state.ProposalReceived() != nil {
		return nil
	}

	// Does not apply, this is not an error but may happen due to network lattency
	if proposal.Block.Number().Cmp(state.BlockNumber()) != 0 {
		logger.Warnw("received proposal with different height.")
		if proposal.Block.Number().Cmp(state.BlockNumber()) > 0 {
			// vote from future block, save to future message queue
			logger.Infow("store proposal vote from future block", "from", msg.Address)
			if err := c.futureMessages.Put(&msgItem{message: msg, height: proposal.Block.Number().Uint64()}); err != nil {
				logger.Errorw("failed to store future proposal message to queue", "err", err, "from", msg.Address)
			}
		}
		return nil
	}

	// Does not apply, this is not an error but may happen due to network latency
	if proposal.Round != state.Round() {
		logger.Warnw("received proposal with different round.")
		if proposal.Round > state.Round() {
			logger.Warnw("received proposal from future round.")
			// make sure this is the proposer of next round
			valSet := c.valSet.Copy()
			valSet.CalcProposer(c.valSet.GetProposer().Address(), proposal.Round-state.Round())
			if valSet.GetProposer().Address() == msg.Address {
				logger.Infow("store proposal from next round", "from", msg.Address)
				c.futureProposals[proposal.Round] = msg
			}
		}
		return nil
	}

	if err := c.VerifyProposal(proposal, msg); err != nil {
		if err == evrynetCore.ErrKnownBlock { // block is already inserted into chain
			return nil
		}
		return err
	}
	logger.Infow("setProposal receive...")

	go c.reBroadcastMsg(msg, logger)

	state.SetProposalReceived(&proposal)
	//TODO: Simulate and test the case where core receives proposal at these steps: prevote/ precommit
	if state.Step() <= RoundStepPropose && state.IsProposalComplete() {
		log.Info("handle proposal: received proposal, proposal completed. before enterPrevote Jump to enterPrevote")
		// Move onto the next step
		c.enterPrevote(state.BlockNumber(), state.Round())
	} else if state.Step() == RoundStepCommit {
		log.Info("handle proposal: received proposal, proposal completed. at commit waiting for proposal block. Jump to finalizeCommit")

		// If we're waiting on the proposal block...
		c.finalizeCommit(proposal.Block.Number())
	} //// TODO: We can check if Proposal is for a different block as this is a sign of misbehavior!
	return nil
}

func (c *core) handlePrevote(msg message) error {
	var (
		vote  Vote
		state = c.CurrentState()
	)
	if err := rlp.DecodeBytes(msg.Msg, &vote); err != nil {
		return err
	}

	if vote.BlockHash == nil || vote.BlockNumber == nil {
		c.getLogger().Panic("nil block hash is not allowed. Please make sure that prevote nil send an emptyBlockHash")
	}
	logger := c.getLogger().With("vote_block", vote.BlockNumber, "from", msg.Address, "vote_round", vote.Round, "block_hash", vote.BlockHash.Hex())

	if vote.BlockNumber.Cmp(state.BlockNumber()) != 0 {
		logger.Warnw("vote's block is different with current block")
		if vote.BlockNumber.Cmp(state.BlockNumber()) > 0 {
			// vote from future block, save to future message queue
			logger.Infow("store prevote vote from future block")
			if err := c.futureMessages.Put(&msgItem{message: msg, height: vote.BlockNumber.Uint64()}); err != nil {
				logger.Errorw("failed to store future prevote message to queue", "err", err)
			}
		}
		return nil
	}
	//log.Info("received prevote", "from", msg.Address, "round", vote.Round, "block_hash", vote.BlockHash.Hex())
	added, err := state.addPrevote(msg, &vote, c.valSet)
	if err != nil {
		return err
	}
	if !added {
		return nil
	}

	logger.Infow("added prevote vote into roundState")
	prevotes, ok := state.GetPrevotesByRound(vote.Round)
	if !ok {
		logger.Panic("expect prevotes to exist now")
	}
	//at this stage, state.PrevoteReceived[vote.Round] is guaranteed to exist.
	if blockHash, ok := prevotes.TwoThirdMajority(); ok {
		logger.Infow("got 2/3 majority on a block", "prevote_block", blockHash.Hex())
		var (
			lockedRound = state.LockedRound()
			lockedBlock = state.LockedBlock()
		)
		//if there is a lockedRound<vote.Round <= state.Round
		//and lockedBlock != nil
		if lockedRound != -1 && lockedRound < vote.Round && vote.Round <= state.Round() && lockedBlock.Hash().Hex() != blockHash.Hex() {
			logger.Infow("unlocking because of POL", "locked_round", lockedRound, "POL_round", vote.Round)
			state.Unlock()
		}

		//set valid Block if the polka is not emptyBlock
		if blockHash.Hex() != emptyBlockHash.Hex() && state.ValidRound() < vote.Round && vote.Round == state.Round() {
			if state.ProposalReceived() != nil && state.ProposalReceived().Block.Hash().Hex() == blockHash.Hex() {
				logger.Infow("updating validblock because of POL", "valid_round", state.ValidRound(), "POL_round", vote.Round)
				state.SetValidRoundAndBlock(vote.Round, state.ProposalReceived().Block)
			} else {
				logger.Infow("updating proposalBlock to nil since we received a valid block we don't know about")
				state.SetProposalReceived(nil)
			}
		}
	}

	go c.reBroadcastMsg(msg, logger)
	//if we receive a future roundthat come to 2/3 of prevotes on any block
	switch {
	case state.Round() < vote.Round && prevotes.HasTwoThirdAny():
		//Skip to vote.round
		c.enterNewRound(state.BlockNumber(), vote.Round)
	case state.Round() == vote.Round && RoundStepPrevote <= state.Step(): // current round
		blockHash, ok := prevotes.TwoThirdMajority()
		if ok && (state.IsProposalComplete() || blockHash.Hex() == emptyBlockHash.Hex()) {
			c.enterPrecommit(state.BlockNumber(), vote.Round)
		} else if prevotes.HasTwoThirdAny() {
			//wait till we got a majority
			c.enterPrevoteWait(state.BlockNumber(), vote.Round)
		}
	case state.ProposalReceived() != nil && 0 <= state.ProposalReceived().POLRound && state.ProposalReceived().POLRound == vote.Round:
		if state.IsProposalComplete() {
			c.enterPrevote(state.BlockNumber(), vote.Round)
		}
	}
	return nil
}

func (c *core) handlePrecommit(msg message) error {
	var (
		vote  Vote
		state = c.CurrentState()
	)
	if err := rlp.DecodeBytes(msg.Msg, &vote); err != nil {
		return err
	}
	if vote.BlockHash == nil || vote.BlockNumber == nil {
		c.getLogger().Panic("nil block hash is not allowed. Please make sure that prevote nil send an emptyBlockHash")
	}

	logger := c.getLogger().With("vote_block", vote.BlockNumber, "vote_round", vote.Round,
		"from", msg.Address.String(), "block_hash", vote.BlockHash.Hex())
	if vote.BlockNumber.Cmp(state.BlockNumber()) != 0 {
		logger.Warnw("vote's block is different with current block")
		if vote.BlockNumber.Cmp(state.BlockNumber()) > 0 {
			// vote from future block, save to future message queue
			logger.Infow("store precommit vote from future block")
			if err := c.futureMessages.Put(&msgItem{message: msg, height: vote.BlockNumber.Uint64()}); err != nil {
				logger.Errorw("failed to store future prevote message to queue", "err", err)
			}
		}
		logger.Warnw("vote's block is different with current block")
		return nil
	}
	//log.Info("received precommit", "from", msg.Address, "round", vote.Round, "block_hash", vote.BlockHash.Hex())
	added, err := state.addPrecommit(msg, &vote, c.valSet)
	if err != nil {
		return err
	}
	if !added {
		return nil
	}
	logger.Infow("added precommit vote into roundState")

	go c.reBroadcastMsg(msg, logger)

	precommits, ok := state.GetPrecommitsByRound(vote.Round)
	if !ok {
		panic("expect precommits to exist now")
	}
	//at this stage, state.PrevoteReceived[vote.Round] is guaranteed to exist.

	blockHash, ok := precommits.TwoThirdMajority()
	if ok {
		log.Info(" got 2/3 precommits  majority on a block", "block", blockHash)
		//this will go through the roundstep again to update core's roundState accordingly in case the vote Round is higher than core's Round
		c.enterNewRound(state.BlockNumber(), vote.Round)
		c.enterPrecommit(state.BlockNumber(), vote.Round)
		//if the precommit are not nil, enter commit
		if blockHash.Hex() != emptyBlockHash.Hex() {
			c.enterCommit(state.BlockNumber(), vote.Round)
			//TODO: if we need to skip when precommits has all votes
		} else { // enter new Round for consensus
			c.enterNewRound(state.BlockNumber(), vote.Round+1)
		}
		return nil
	}

	//if there is no majority block
	if state.Round() <= vote.Round && precommits.HasTwoThirdAny() {
		//go through roundstep again to update round state
		c.enterNewRound(state.BlockNumber(), vote.Round)
		//wait for more precommit
		c.enterPrecommitWait(state.BlockNumber(), vote.Round)
	}
	return nil
}

//TODO: keep track of the CatchupRequest to stop other nodes from attacking us by sending continuous catchup request
func (c *core) handleCatchupRequest(msg message) error {
	var (
		catchUpMsg  CatchUpRequestMsg
		state       = c.currentState
		blockNumber = state.BlockNumber()
		round       = state.Round()
		step        = state.Step()
	)
	if err := rlp.DecodeBytes(msg.Msg, &catchUpMsg); err != nil {
		return err
	}

	logger := c.getLogger().With("catchup_block", catchUpMsg.BlockNumber, "catchup_round", catchUpMsg.Round,
		"catchup_step", catchUpMsg.Step, "from", msg.Address.String())
	if catchUpMsg.BlockNumber.Cmp(blockNumber) != 0 || catchUpMsg.Round > round || (catchUpMsg.Round == round && catchUpMsg.Step > step) {
		logger.Debugw(" Ignoring timeout because we're behind or different with block")
		return nil
	}
	// re-send to from address
	var payloads [][]byte
	index := c.sentMsgStorage.lookup(catchUpMsg.Step, catchUpMsg.Round)
	if index == -1 {
		logger.Infow("no msg for catchup request available")
		return nil
	}
	for i := index; ; i++ {
		data, err := c.sentMsgStorage.get(i)
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Errorw("Failed to retrieve msg", "err", err)
		}
		payloads = append(payloads, data)
		if len(payloads) >= catchUpReplyBatchSize {
			c.SendCatchupReply(msg.Address, payloads)
			payloads = make([][]byte, 0)
		}
	}
	if len(payloads) != 0 {
		c.SendCatchupReply(msg.Address, payloads)
	}
	return nil
}

func (c *core) handleCatchUpReply(msg message) error {
	var (
		catchUpReplyMsg CatchUpReplyMsg
		state           = c.currentState
	)

	if err := rlp.DecodeBytes(msg.Msg, &catchUpReplyMsg); err != nil {
		return err
	}
	logger := c.getLogger().With("num_msg", len(catchUpReplyMsg.Payloads), "block", catchUpReplyMsg.BlockNumber, "from", msg.Address.String())
	if state.BlockNumber().Cmp(catchUpReplyMsg.BlockNumber) != 0 {
		logger.Debugw("catchUpReplyMsg block is different with current block, skipping")
		return nil
	}
	logger.Infow("Handle catchUpReplyMsg")
	for _, payload := range catchUpReplyMsg.Payloads {
		var subMsg message
		if err := rlp.DecodeBytes(payload, &subMsg); err != nil {
			fmt.Println("xxxxx")
			return err
		}
		if subMsg.Address != msg.Address {
			logger.Debugw("Address of catch up reply msg and its child are miss match, skipping", "sub_address", subMsg.Address)
			return ErrCatchUpReplyAddressMissMatch
		}
		if err := c.handleMsgLocked(subMsg); err != nil {
			return err
		}
	}
	return nil
}

// handleMsgLocked assume that c.mu is locked
func (c *core) handleMsgLocked(msg message) error {
	logger := c.getLogger()
	signer, err := msg.GetAddressFromSignature()
	if err != nil {
		logger.Debugw("Failed to get signer from msg", "err", err)
		return err
	}
	if signer != msg.Address {
		logger.Debugw("Deprived signer and address field of msg are miss-match",
			"signer", signer, "from", msg.Address)
		return ErrSignerMessageMissMatch
	}

	switch msg.Code {
	case msgPropose:
		return c.handlePropose(msg)
	case msgPrevote:
		return c.handlePrevote(msg)
	case msgPrecommit:
		return c.handlePrecommit(msg)
	case msgCatchUpRequest:
		return c.handleCatchupRequest(msg)
	case msgCatchUpReply:
		return c.handleCatchUpReply(msg)
	default:
		return fmt.Errorf("unknown msg code %d", msg.Code)
	}
}

func (c *core) handleMsg(msg message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handleMsgLocked(msg)
}

func (c *core) handleTimeout(ti timeoutInfo) {
	var (
		round       = c.CurrentState().Round()
		blockNumber = c.CurrentState().BlockNumber()
		step        = c.CurrentState().Step()
		logger      = c.getLogger().With("ti_block_number", ti.BlockNumber, "ti_round", ti.Round, "ti_step", ti.Step)
	)
	logger.Infow("Received timeout signal from core.timeout", "timeout", ti.Duration)
	// timeouts must be for current height, round, step
	if ti.BlockNumber.Cmp(blockNumber) != 0 || ti.Round < round || (ti.Round == round && ti.Step < step) {
		logger.Infow("Ignoring timeout because we're ahead")
		return
	}

	// the timeout will now cause a state transition
	c.mu.Lock()
	defer c.mu.Unlock()

	switch ti.Step {
	case RoundStepNewHeight:
		c.enterNewRound(ti.BlockNumber, 0)
	case RoundStepNewRound:
		c.enterPropose(ti.BlockNumber, 0)
	case RoundStepPropose:
		c.enterPrevote(ti.BlockNumber, ti.Round)
	case RoundStepPrevote, RoundStepPrecommit:
		c.enterCatchup(ti.BlockNumber, ti.Round, ti.Step, ti.Retry)
	case RoundStepPrevoteWait:
		c.enterPrecommit(ti.BlockNumber, ti.Round)
	case RoundStepPrecommitWait:
		c.enterPrecommit(ti.BlockNumber, ti.Round)
		c.enterNewRound(ti.BlockNumber, ti.Round+1)
	default:
		logger.Panicw("Invalid timeout step")
	}
}
