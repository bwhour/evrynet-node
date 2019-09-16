// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"io"
	"math/big"
	"time"

	"github.com/evrynet-official/evrynet-client/common"
	"github.com/evrynet-official/evrynet-client/consensus/tendermint"
	"github.com/evrynet-official/evrynet-client/core/types"
	"github.com/evrynet-official/evrynet-client/rlp"
)

//newRoundState creates a new roundState instance with the given view and validatorSet
func newRoundState(view *tendermint.View, prevotesReceived, precommitsReceived map[int64]*messageSet, block *types.Block,
	lockedRound int64, lockedBlock *types.Block,
	validRound int64, validBlock *types.Block,
	proposalReceived *tendermint.Proposal, step RoundStepType) *roundState {
	return &roundState{
		view:               view,
		block:              block,
		lockedRound:        lockedRound,
		lockedBlock:        lockedBlock,
		validRound:         validRound,
		validBlock:         validBlock,
		proposalReceived:   proposalReceived,
		PrevotesReceived:   prevotesReceived,
		PrecommitsReceived: precommitsReceived,
		step:               step,
	}
}

// roundState stores the consensus state
type roundState struct {
	view  *tendermint.View // view contains round and height
	block *types.Block     // current proposed block

	lockedRound int64        // lockedRound is latest round it is locked
	lockedBlock *types.Block // lockedBlock is block it is locked at lockedRound above

	validRound int64        // validRound is last known round with PoLC for non-nil valid block, i.e, a block with a valid polka
	validBlock *types.Block // validBlock is last known block of PoLC above

	commitRound int64     //commit Round is the round where it receive 2/3 precommit and enter commit stage.
	commitTime  time.Time // commit timestamp
	startTime   time.Time // time to start new round

	proposalReceived   *tendermint.Proposal  //
	PrevotesReceived   map[int64]*messageSet //This is the prevote received for each round
	PrecommitsReceived map[int64]*messageSet //this is the precommit received for each round
	PrecommitWaited    bool                  //we only wait for precommit once each round

	//step is the enumerate Step that currently the core is at.
	//to jump to the next step, UpdateRoundStep is called.
	step RoundStepType
}

func (s *roundState) Step() RoundStepType {
	return s.step
}

func (s *roundState) BlockNumber() *big.Int {
	return s.view.BlockNumber
}

func (s *roundState) Round() int64 {
	return s.view.Round
}

func (s *roundState) UpdateRoundStep(round int64, step RoundStepType) {
	s.view.Round = round
	s.step = step
}

func (s *roundState) ProposalReceived() *tendermint.Proposal {
	return s.proposalReceived
}

func (s *roundState) SetProposalReceived(proposalReceived *tendermint.Proposal) {

	s.proposalReceived = proposalReceived
}

func (s *roundState) SetView(v *tendermint.View) {
	s.view = v
}

// IsProposalComplete Returns true if the proposal block is complete &&
// (if POLRound was proposed, we have +2/3 prevotes from there).
func (s *roundState) IsProposalComplete() bool {
	if s.proposalReceived == nil {
		return false
	}
	if s.proposalReceived.POLRound < 0 {
		return true
	}
	prevotes, ok := s.PrevotesReceived[s.proposalReceived.POLRound]
	if !ok {
		return false
	}

	return prevotes.HasMajority()
}

func (s *roundState) View() *tendermint.View {
	return s.view
}

func (s *roundState) SetBlock(bl *types.Block) {
	s.block = bl
}

func (s *roundState) Block() *types.Block {
	return s.block
}

func (s *roundState) SetLockedRoundAndBlock(lockedR int64, lockedBl *types.Block) {
	s.lockedRound = lockedR
	s.lockedBlock = lockedBl
}

func (s *roundState) Unlock() {
	s.lockedRound = -1
	s.lockedBlock = nil
}

func (s *roundState) LockedRound() int64 {
	return s.lockedRound
}

func (s *roundState) LockedBlock() *types.Block {
	return s.lockedBlock
}

func (s *roundState) SetValidRoundAndBlock(validR int64, validBl *types.Block) {
	s.validRound = validR
	s.validBlock = validBl
}

func (s *roundState) ValidRound() int64 {
	return s.validRound
}

func (s *roundState) ValidBlock() *types.Block {
	return s.validBlock
}

// Last round and block that has +2/3 prevotes for a particular block or nil.
// Returns -1 if no such round exists.
func (s *roundState) POLInfo() (polRound int64, polBlockHash *common.Hash) {
	// TODO: Just a sample
	for r := s.Round(); r >= 0; r-- {
		prevotes, ok := s.GetPrevotesByRound(r)
		if ok {
			polBlockHash, ok = prevotes.TwoThirdMajority()
		}
		if ok {
			return r, polBlockHash
		}
	}
	return -1, nil
}

// The DecodeRLP method should read one value from the given
// Stream. It is not forbidden to read less or more, but it might
// be confusing.
func (s *roundState) DecodeRLP(stream *rlp.Stream) error {
	var ss struct {
		View               *tendermint.View
		Block              *types.Block
		LockedRound        int64
		LockedBlock        *types.Block
		ValidRound         int64
		ValidBlock         *types.Block
		proposalReceived   *tendermint.Proposal
		PrevotesReceived   map[int64]*messageSet
		PrecommitsReceived map[int64]*messageSet
	}

	if err := stream.Decode(&ss); err != nil {
		return err
	}
	s.view, s.block = ss.View, ss.Block
	s.lockedRound, s.lockedBlock = ss.LockedRound, ss.LockedBlock
	s.validRound, s.validBlock = ss.ValidRound, ss.ValidBlock
	s.proposalReceived = ss.proposalReceived
	s.PrevotesReceived = ss.PrevotesReceived
	s.PrecommitsReceived = ss.PrecommitsReceived

	return nil
}

// EncodeRLP should write the RLP encoding of its receiver to w.
// If the implementation is a pointer method, it may also be
// called for nil pointers.
//
// Implementations should generate valid RLP. The data written is
// not verified at the moment, but a future version might. It is
// recommended to write only a single value but writing multiple
// values or no value at all is also permitted.
func (s *roundState) EncodeRLP(w io.Writer) error {

	return rlp.Encode(w, []interface{}{
		s.view,
		s.block,
		s.lockedRound,
		s.lockedBlock,
		s.validRound,
		s.validBlock,
		s.proposalReceived,
		s.PrevotesReceived,
		s.PrecommitsReceived,
	})
}

func (s *roundState) addPrevote(msg message, vote *tendermint.Vote, valset tendermint.ValidatorSet) (bool, error) {
	msgSet, ok := s.PrevotesReceived[vote.Round]
	if !ok {
		msgSet = newMessageSet(valset, msgPrevote, s.view)
		s.PrevotesReceived[vote.Round] = msgSet
	}
	return msgSet.AddVote(msg, vote)
}

//GetPrevotesByRound return prevote messageSet for that round, if there is no prevotes message on the said round, return nil and false
func (s *roundState) GetPrevotesByRound(round int64) (*messageSet, bool) {
	msgSet, ok := s.PrevotesReceived[round]
	return msgSet, ok
}

func (s *roundState) addPrecommit(msg message, vote *tendermint.Vote, valset tendermint.ValidatorSet) (bool, error) {
	msgSet, ok := s.PrecommitsReceived[vote.Round]
	if !ok {
		msgSet = newMessageSet(valset, msgPrecommit, s.view)
		s.PrecommitsReceived[vote.Round] = msgSet
	}
	return msgSet.AddVote(msg, vote)
}

//GetPrecommitsByRound return precommit messageSet for that round, if there is no precommit message on the said round, return nil and false
func (s *roundState) GetPrecommitsByRound(round int64) (*messageSet, bool) {
	msgSet, ok := s.PrevotesReceived[round]
	return msgSet, ok
}

func (s *roundState) getPrecommitWaited() bool {
	return s.PrecommitWaited
}

func (s *roundState) setPrecommitWaited(waited bool) {
	s.PrecommitWaited = waited
}