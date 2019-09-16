package core

import (
	"fmt"
	"math/big"
	"time"

	"github.com/evrynet-official/evrynet-client/common"
	"github.com/evrynet-official/evrynet-client/consensus/tendermint"
	"github.com/evrynet-official/evrynet-client/core/types"
	"github.com/evrynet-official/evrynet-client/log"
)

//enterNewRound switch the core state to new round,
//it checks core state to make sure that it's legal to enterNewRound
//it set core.currentState with new params and call enterPropose
//enterNewRound is called after:
// - `timeoutNewHeight` by startTime (commitTime+timeoutCommit),
// 	or, if SkipTimeout==true, after receiving all precommits from (height,round-1)
// - `timeoutPrecommits` after any +2/3 precommits from (height,round-1)
// - +2/3 precommits for nil at (height,round-1)
// - +2/3 prevotes any or +2/3 precommits for block or any from (height, round)
// NOTE: cs.StartTime was already set for height.
func (c *core) enterNewRound(blockNumber *big.Int, round int64) {
	//This is strictly use with pointer for state update.
	var (
		state         = c.CurrentState()
		sBlockNunmber = state.BlockNumber()
		sRound        = state.Round()
		sStep         = state.Step()
	)
	if sBlockNunmber.Cmp(blockNumber) != 0 || round < sRound || (sRound == round && sStep != RoundStepNewHeight) {
		log.Debug("enterNewRound ignore: we are in a state that is ahead of the input state",
			"current_block_number", sBlockNunmber.String(), "input_block_number", blockNumber.String(),
			"current_round", sRound, "input_round", round,
			"current_step", sStep.String(), "input_step", RoundStepNewRound.String())
		return
	}

	log.Debug("enterNewRound",
		"current_block_number", sBlockNunmber.String(), "input_block_number", blockNumber.String(),
		"current_round", sRound, "input_round", round,
		"current_step", sStep.String(), "input_step", RoundStepNewRound.String())

	//if the round we enter is higher than current round, we'll have to adjust the proposer.
	if sRound < round {
		currentProposer := c.valSet.GetProposer()
		c.valSet.CalcProposer(currentProposer.Address(), round)
	}

	//Update to RoundStepNewRound
	state.UpdateRoundStep(round, RoundStepNewRound)
	state.setPrecommitWaited(false)

	c.enterPropose(blockNumber, round)

}

//defaultDecideProposal is the default proposal selector
//it will prioritize validBlock, else will get its own block from tx_pool
func (c *core) defaultDecideProposal(round int64) tendermint.Proposal {
	var (
		state = c.CurrentState()
	)
	// if there is validBlock, propose it.
	if state.ValidRound() != -1 {
		log.Debug("getting the core's valid", "block", state.ValidBlock())

		return tendermint.Proposal{
			Block:    state.ValidBlock(),
			Round:    round,
			POLRound: state.ValidRound(),
		}
	}
	//TODO: remove this
	log.Debug("getting the core's block", "block", state.Block())
	//get the block node currently received from tx_pool
	return tendermint.Proposal{
		Block:    state.Block(),
		Round:    round,
		POLRound: -1,
	}
}

//enterPropose switch core state to propose step.
//it checks core state to make sure that it's legal to enterPropose
//it check if this core is proposer and send Propose
//otherwise it will set timeout and eventually call enterPrevote
//enterPropose is called after:
// enterNewRound(blockNumber,round)
func (c *core) enterPropose(blockNumber *big.Int, round int64) {
	//This is strictly use with pointer for state update.
	var (
		state         = c.CurrentState()
		sBlockNunmber = state.BlockNumber()
		sRound        = state.Round()
		sStep         = state.Step()
	)
	if sBlockNunmber.Cmp(blockNumber) != 0 || sRound > round || (sRound == round && sStep >= RoundStepPropose) {
		log.Debug("enterPropose ignore: we are in a state that is ahead of the input state",
			"current_block_number", sBlockNunmber.String(), "input_block_number", blockNumber.String(),
			"current_round", sRound, "input_round", round,
			"current_step", sStep.String(), "input_step", RoundStepPropose.String())
		return
	}

	log.Debug("enterPropose",
		"current_block_number", sBlockNunmber.String(), "input_block_number", blockNumber.String(),
		"current_round", sRound, "input_round", round,
		"current_step", sStep.String(), "input_step", RoundStepPropose.String())

	defer func() {
		// Done enterPropose:
		state.UpdateRoundStep(round, RoundStepPropose)

		// If we have the whole proposal + POL, then goto PrevoteTimeout now.
		// else, we'll enterPrevote when the rest of the proposal is received (in AddProposalBlockPart),
		if state.IsProposalComplete() {
			c.enterPrevote(blockNumber, sRound)
		}
	}()

	// if timeOutPropose, it will eventually come to enterPrevote, but the timeout might interrupt the timeOutPropose
	// to jump to a better state. Imagine that at line 91, we come to enterPrevote and a new timeout is call from there,
	// the timeout can skip this timeOutPropose.
	c.timeout.ScheduleTimeout(timeoutInfo{
		Duration:    c.config.ProposeTimeout(round),
		BlockNumber: blockNumber,
		Round:       round,
		Step:        RoundStepPropose,
	})

	if i, _ := c.valSet.GetByAddress(c.backend.Address()); i == -1 {
		log.Debug("this node is not a validator of this round", "address", c.backend.Address().String(), "block_number", blockNumber.String(), "round", round)
		return
	}
	//if we are proposer, find the latest block we're having to propose
	if c.valSet.IsProposer(c.backend.Address()) {
		log.Info("this node is proposer of this round")
		//TODO : find out if this is better than current Tendermint implementation
		//var (
		//	lockedRound = state.LockedRound()
		//	lockedBlock = state.LockedBlock()
		//)
		//// if there is a lockedBlock, set validRound and validBlock to locked one

		//if lockedRound != -1 {
		//	state.SetValidRoundAndBlock(lockedRound, lockedBlock)
		//
		//}
		proposal := c.defaultDecideProposal(round)

		c.SendPropose(&proposal)
	}
}

//defaultDoPrevote is the default process of select a block for pretoe
//it will: - prevote lockedBlock if lockedBlock !=nil
//		   - prevote for proposalReceived if valid
//		   - prevote nil otherwise
func (c *core) defaultDoPrevote(round int64) {
	var (
		state = c.CurrentState()
	)
	// If a block is locked, prevote that.
	if state.LockedRound() != -1 {
		log.Info("prevote for locked Block")
		c.SendVote(msgPrevote, state.LockedBlock(), round)
		return
	}

	// If ProposalBlock is nil, prevote nil.
	if state.ProposalReceived() == nil {
		log.Info("prevote nil")
		c.SendVote(msgPrevote, nil, round)
		return
	}

	// TODO: Validate proposal block
	//}

	// PrevoteTimeout cs.ProposalBlock
	// NOTE: the proposal signature is validated when it is received,
	log.Info("prevote for proposal block")
	c.SendVote(msgPrevote, state.ProposalReceived().Block, round)
	//core.signAddVote(types.PrevoteType, cs.ProposalBlock.Hash(), cs.ProposalBlockParts.Header())
}

// enterPrevote set core to prevote state, at which step it will:
// - decide to whether it needs to unlock if PoLCR>LLR
// - broadcastPrevote on lockedBlock if locked, or prevote for a valid proposal, else prevote nil
// - wait until it receveid 2F+1 prevotes
// - set timer if the prevotes receives dont reach majority
// enterPrevote is called
// - when `timeoutPropose` after entering Propose.
// - when proposal block and POL is ready.
func (c *core) enterPrevote(blockNumber *big.Int, round int64) {
	//TODO: write a function for this at all enter step
	//This is strictly use with pointer for state update.
	var (
		state         = c.CurrentState()
		sBlockNunmber = state.BlockNumber()
		sRound        = state.Round()
		sStep         = state.Step()
	)
	if sBlockNunmber.Cmp(blockNumber) != 0 || round < sRound || (sRound == round && sStep >= RoundStepPrevote) {
		log.Debug("enterPrevote ignore: we are in a state that is ahead of the input state",
			"current_block_number", sBlockNunmber.String(), "input_block_number", blockNumber.String(),
			"current_round", sRound, "input_round", round,
			"current_step", sStep.String(), "input_step", RoundStepPrevote.String())
		return
	}

	log.Debug("enterPrevote",
		"current_block_number", sBlockNunmber.String(),
		"current_round", sRound, "input_round", round,
		"current_step", sStep.String())

	//eventually we'll enterPrevote
	defer func() {
		state.UpdateRoundStep(round, RoundStepPrevote)
	}()
	c.defaultDoPrevote(round)
}

// Enter: if received +2/3 precommits for next round.
// Enter: any +2/3 prevotes at next round.
func (c *core) enterPrevoteWait(blockNumber *big.Int, round int64) {
	var (
		state        = c.CurrentState()
		sBlockNumber = state.BlockNumber()
		sRound       = state.Round()
		sStep        = state.Step()
	)

	if sBlockNumber.Cmp(blockNumber) != 0 || round < sRound || (sRound == round && RoundStepPrevoteWait <= sStep) {
		log.Debug("enterPrevoteWait ignore: we are in a state that is ahead of the input state",
			"current_block_number", sBlockNumber.String(), "input_block_number", blockNumber.String(),
			"current_round", sRound, "input_round", round,
			"current_step", sStep.String(), "input_step", RoundStepPrevote.String())
		return
	}
	prevotes, ok := state.GetPrevotesByRound(round)
	if !ok {
		log.Debug("enterPrevoteWait ignore: there is no prevotes", "round", round)
	}
	if !prevotes.HasTwoThirdAny() {
		log.Debug("enterPrevoteWait ignore: there is no two third votes received", "round", round)
	}
	log.Debug("enterPrevoteWait",
		"current_block_number", sBlockNumber.String(),
		"current_round", sRound, "input_round", round,
		"current_step", sStep.String())

	defer func() {
		// Done enterPrevoteWait:
		state.UpdateRoundStep(round, RoundStepPrevoteWait)
	}()

	// Wait for some more prevotes; enterPrecommit
	c.timeout.ScheduleTimeout(timeoutInfo{
		Duration:    c.config.PrevoteTimeout(round),
		BlockNumber: blockNumber,
		Round:       round,
		Step:        RoundStepPrevoteWait,
	})
}

func (c *core) enterPrecommitWait(blockNumber *big.Int, round int64) {
	var (
		state        = c.CurrentState()
		sBlockNumber = state.BlockNumber()
		sRound       = state.Round()
		sStep        = state.Step()
	)

	if sBlockNumber.Cmp(blockNumber) != 0 || round < sRound || (sRound == round && state.getPrecommitWaited()) {
		log.Debug("enterPrecommitWait ignore: we are in a state that is not suitable to enter precommit with input state",
			"current_block_number", sBlockNumber.String(), "input_block_number", blockNumber.String(),
			"current_round", sRound, "input_round", round, "precommitWaited", state.getPrecommitWaited())
		return
	}

	precommits, ok := state.GetPrecommitsByRound(round)
	if !ok {
		log.Error("enterPrecommitWait with no precommit votes", "block_number", sBlockNumber, "round", sRound)
		panic("enterPrecommitWait with no precommit votes")
	}
	if !precommits.HasTwoThirdAny() {
		panic("enterPrecommitWait without precommits has 2/3 of votes")
	}
	log.Debug("enterPrecommitWait",
		"current_block_number", sBlockNumber.String(),
		"current_round", sRound, "input_round", round,
		"current_step", sStep.String())

	//after this we setPrecommitWaited to true to make sure that the wait happens only once each round
	defer func() {
		state.setPrecommitWaited(true)
	}()

	c.timeout.ScheduleTimeout(timeoutInfo{
		Duration:    c.config.PrecommitTimeout(round),
		BlockNumber: blockNumber,
		Round:       round,
		Step:        RoundStepPrecommitWait,
	})

}

// enterPrecommit sets core to precommit state:
// Enter: `timeoutPrecommit` after any +2/3 precommits.
// Enter: +2/3 precomits for block or nil.
// Lock & precommit the ProposalBlock if we have enough prevotes for it (a POL in this round)
// else, unlock an existing lock and precommit nil if +2/3 of prevotes were nil,
// else, precommit nil otherwise.
func (c *core) enterPrecommit(blockNumber *big.Int, round int64) {
	var (
		state         = c.currentState
		sBlockNunmber = state.BlockNumber()
		sRound        = state.Round()
		sStep         = state.Step()
	)

	if sBlockNunmber.Cmp(blockNumber) != 0 || round < sRound || (sRound == round && sStep >= RoundStepPrecommit) {
		log.Debug("enterPrecommit ignore: we are in a state that is ahead of the input state",
			"current_block_number", sBlockNunmber.String(), "input_block_number", blockNumber.String(),
			"current_round", sRound, "input_round", round,
			"current_step", sStep.String(), "input_step", RoundStepPrevote.String())
		return
	}

	log.Debug("enterPrecommit",
		"current_block_number", sBlockNunmber.String(),
		"current_round", sRound, "input_round", round,
		"current_step", sStep.String())

	defer func() {
		// Done enterPrecommit:
		state.UpdateRoundStep(round, RoundStepPrecommit)
	}()

	// Note: Liem has already implemented GetPrevotesByRound(round), will change once the PR is merged
	var blockHash *common.Hash
	prevotes, ok := state.GetPrevotesByRound(round)
	if ok {
		blockHash, ok = prevotes.TwoThirdMajority()
	}

	// if we don't have polka, must precommit nil
	if !ok {
		if state.LockedBlock() != nil {
			log.Info("enterPrecommit: No +2/3 prevotes during enterPrecommit while we're locked. Precommitting nil")
		} else {
			log.Info("enterPrecommit: No +2/3 prevotes during enterPrecommit. Precommitting nil.")
		}
		c.SendVote(msgPrecommit, nil, round)
		return
	}

	// The last PoLR should be this round
	polRound, _ := state.POLInfo()
	if polRound < round {
		panic(fmt.Sprintf("This POLRound should be %v but got %v", round, polRound))
	}

	// +2/3 prevoted nil. Unlock and precommit nil.
	if len(blockHash) == 0 {
		if state.LockedBlock() == nil {
			log.Info("enterPrecommit: +2/3 prevoted for nil.")
		} else {
			log.Info("enterPrecommit: +2/3 prevoted for nil. Unlocking")
			state.Unlock()
		}
		c.SendVote(msgPrecommit, nil, round)
		return
	}

	// At this point, +2/3 prevoted for a particular block.
	// If we're already locked on that block, precommit it, and update the LockedRound
	if state.LockedBlock() != nil && state.LockedBlock().Hash().Hex() == blockHash.Hex() {
		log.Info("enterPrecommit: +2/3 prevoted locked block. Relocking")
		state.SetLockedRoundAndBlock(round, state.LockedBlock())
		c.SendVote(msgPrecommit, state.LockedBlock(), round)
		return
	}

	// If +2/3 prevoted for proposal block, stage and precommit it
	if state.ProposalReceived() != nil && state.ProposalReceived().Block.Hash().Hex() == blockHash.Hex() {
		log.Info("enterPrecommit: +2/3 prevoted proposal block. Locking", "hash", blockHash)
		// TODO: Validate the block before locking and precommit
		state.SetLockedRoundAndBlock(round, state.ProposalReceived().Block)
		c.SendVote(msgPrecommit, state.ProposalReceived().Block, round)
		return
	}

	// There was a polka in this round for a block we don't have.
	// TODO: Fetch that block, unlock, and precommit nil.
	// The +2/3 prevotes for this round is the POL for our unlock.
	log.Info("enterPrecommit: +2/3 prevoted a block we don't have. Fetch. Unlock and Precommit nil", "hash", blockHash.Hex())
	state.Unlock()
	c.SendVote(msgPrecommit, nil, round)
}

func (c *core) enterCommit(blockNumber *big.Int, commitRound int64) {
	var state = c.currentState
	if state.BlockNumber().Cmp(blockNumber) != 0 || state.Step() >= RoundStepPrecommit {
		log.Debug("enterCommit ignore: we are in a state that is ahead of the input state",
			"current_block_number", state.BlockNumber().String(), "input_block_number", blockNumber.String(),
			"current_step", state.Step().String(), "input_step", RoundStepPrevote.String())
		return
	}

	defer func() {
		// Done enterCommit:
		// keep state.Round the same, commitRound points to the right Precommits set.
		state.UpdateRoundStep(state.Round(), RoundStepCommit)
		state.commitRound = commitRound
		state.commitTime = time.Now()

		c.finalizeCommit(blockNumber)
	}()

	precommits, ok := state.GetPrecommitsByRound(commitRound)

	if !ok {
		panic("commit round must have a set of precommits")
	}

	blockHash, ok := precommits.TwoThirdMajority()

	if !ok {
		panic("commit round must has a majority block")
	}
	var (
		lockedBlock = state.LockedBlock()
	)
	//if lockBlock is the same as the hash, move it to Proposal
	//it will be cleared upon entering newHeight
	if lockedBlock != nil && lockedBlock.Hash().Hex() == blockHash.Hex() {
		log.Info("Commit is for locked block. Set ProposalBlock=LockedBlock", "blockHash", blockHash.Hex())
		state.SetProposalReceived(&tendermint.Proposal{
			Block: lockedBlock,
			Round: commitRound,
		})
	}
	var (
		proposalReceived = state.ProposalReceived()
	)
	// If we don't have the block being commit, we set proposalReceived to nil and wait
	if proposalReceived != nil && proposalReceived.Block.Hash().Hex() != blockHash.Hex() {
		state.SetProposalReceived(nil)
	}

}

func (c *core) finalizeCommit(blockNumber *big.Int) {
	var state = c.CurrentState()
	if state.BlockNumber().Cmp(blockNumber) != 0 {
		log.Error("finalize a commit at different state block number", "current_block_number", state.BlockNumber(), "commit_block_number", blockNumber)
		panic("finalize a commit at different block number")
	}
	if state.Step() != RoundStepCommit {
		log.Error("finalizeCommit invalid: we are in a state that is invalid for commit",
			"current_block_number", state.BlockNumber().String(), "input_block_number", blockNumber.String(),
			"current_step", state.Step().String(), "input_step", RoundStepCommit.String())
		return
	}
	precommits, ok := state.GetPrecommitsByRound(state.commitRound)
	if !ok {
		log.Error("no precommits at commitRound")
		return
	}
	blockHash, ok := precommits.TwoThirdMajority()
	if !ok {
		log.Error("no 2/3 majority for a block at commitRound")
		return
	}
	if blockHash.Hex() == emptyBlockHash.Hex() {
		log.Error("nil majority at commitRound")
		return
	}
	proposal := state.ProposalReceived()
	if proposal == nil {
		log.Info("empty proposal at finalizeCommit: no proposal has been received")
		return
	}
	if proposal.Block != nil && proposal.Block.Hash().Hex() != blockHash.Hex() {
		log.Info("the proposal received was not the commit hash. Finalize failed")
		return
	}

	//TODO: do we need revalidating block at this step?

	log.Info("finalizing Block", "block_hash", blockHash.Hex())

	block := c.FinalizeBlock(state.ProposalReceived().Block)
	c.blockFinalize.Post(tendermint.BlockFinalizedEvent{
		Block: block,
	})

	//TODO: after block is finalized, is there any event that backend should fire to update core's status?

	c.updateStateForNewblock()
	c.startRoundZero()
}

//FinalizeBlock will fill extradata with signature and return the ready to store block
func (c *core) FinalizeBlock(block *types.Block) *types.Block {
	return block
}

func (c *core) startRoundZero() {
	var state = c.CurrentState()
	sleepDuration := state.startTime.Sub(time.Now())
	c.timeout.ScheduleTimeout(timeoutInfo{
		Duration:    sleepDuration,
		BlockNumber: state.BlockNumber(),
		Round:       0,
		Step:        RoundStepNewHeight,
	})
}

func (c *core) updateStateForNewblock() {
	var state = c.CurrentState()

	if state.commitRound > -1 {
		// having commit round, should have seen +2/3 precommits
		precommits, ok := state.GetPrecommitsByRound(state.commitRound)
		_, ok = precommits.TwoThirdMajority()
		if !ok {
			log.Error("updateStateForNewblock(): Having commitRound with no +2/3 precommits")
			return
		}
	}

	// Update all roundState's fields
	height := state.BlockNumber()
	state.SetView(&tendermint.View{
		Round:       0,
		BlockNumber: height.Add(height, big.NewInt(1)),
	})
	state.UpdateRoundStep(0, RoundStepNewHeight)

	if state.commitTime.IsZero() {
		// "Now" makes it easier to sync up dev nodes.
		// We add timeoutCommit to allow transactions
		// to be gathered for the first block.
		// And alternative solution that relies on clocks:
		state.startTime = c.config.Commit(time.Now())
	} else {
		state.startTime = c.config.Commit(state.commitTime)
	}

	state.SetBlock(nil)
	state.SetLockedRoundAndBlock(-1, nil)
	state.SetValidRoundAndBlock(-1, nil)
	state.SetProposalReceived(nil)

	state.commitRound = -1
	state.PrevotesReceived = nil
	state.PrecommitsReceived = nil
	state.PrecommitWaited = false

	c.currentState = state

	if c.valSet == nil {
		c.valSet = c.backend.Validators(state.BlockNumber())
	}
}