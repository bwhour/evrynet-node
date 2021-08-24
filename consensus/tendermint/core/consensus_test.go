package core

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/tests_utils"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/utils"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

const (
	Block1 = 1
	Block2 = 2
)

func TestFinalizeBlock(t *testing.T) {
	var (
		nodePrivateKey, _ = crypto.HexToECDSA("ce900e4057ef7253ce737dccf3979ec4e74a19d595e8cc30c6c5ea92dfdd37f1")
		nodeAddr          = crypto.PubkeyToAddress(nodePrivateKey.PublicKey)
		a, _              = common.EvryAddressStringToAddressCheck("EPXt4N4aPgV4BEKLtYJgz2NdiAHfa7c2ao")
		b, _              = common.EvryAddressStringToAddressCheck("ERXpWm2MrviVU2KjGMgq1xUQYLie2zUfkK")
		c, _              = common.EvryAddressStringToAddressCheck("EWmMyKETQCsTYEC3W51dZ3bpUWvn3XtrwG")
		validators        = []common.Address{
			a,
			nodeAddr,
			b,
			c,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
	)
	//create New test backend and newMockChain
	be, _ := tests_utils.MustCreateAndStartNewBackend(t, nodePrivateKey, genesisHeader, validators)

	core := newTestCore(be, tendermint.DefaultConfig)
	require.NoError(t, core.Start())
	state := core.CurrentState()
	assert.Equal(t, RoundStepNewHeight, state.Step())

	//fake core.CurrentState
	var (
		voteRound       = int64(0)
		voteBlockNumber = big.NewInt(1)
	)
	core.currentState = core.getInitializedState()
	core.currentState.commitRound = voteRound

	view := tendermint.View{
		BlockNumber: voteBlockNumber,
		Round:       voteRound,
	}

	testCases := []struct {
		name           string
		validatorVotes map[int]int
		totalReceived  int
		assertFn       func(block *types.Block, err error)
	}{
		{
			name: "Case 1",
			validatorVotes: map[int]int{
				0: 2,
				1: 2,
				2: 2,
				3: 2,
			},
			totalReceived: 4,
			assertFn: func(block *types.Block, err error) {
				assert.NotNil(t, block)
				assert.NoError(t, err)
			},
		},
		{
			name: "Case 2: Validator 0,2,3 vote for block 2 (voting of validator 1 is nil in block 2). Validator 1 votes for block 1",
			validatorVotes: map[int]int{
				0: 2,
				1: 1,
				2: 2,
				3: 2,
			},
			totalReceived: 3,
			assertFn: func(block *types.Block, err error) {
				assert.NotNil(t, block)
				assert.NoError(t, err)
			},
		},
		{
			name: "Case 3: Validator 0,1 will vote for block 2 (voting of validator 2,3 is nil in block 2). Validator 2,3 will vote for block 1",
			validatorVotes: map[int]int{
				0: 2,
				1: 2,
				2: 1,
				3: 1,
			},
			totalReceived: 2,
			assertFn: func(block *types.Block, err error) {
				assert.Nil(t, block)
				assert.Error(t, err) // Get error "not enough precommits received expect at least 3 received 2"
			},
		},
	}

	for _, tc := range testCases {
		validateVote := func(t *testing.T) {
			newMsgSet := newMessageSet(core.valSet, msgPrecommit, &view)

			//Create block 1
			genesisHeader.Number = big.NewInt(1)
			bl1 := tests_utils.MakeBlockWithoutSeal(genesisHeader)
			blHash1 := bl1.Hash()
			committedSeal1, err := core.backend.Sign(utils.PrepareCommittedSeal(blHash1))
			require.NoError(t, err)

			//Create block 2
			genesisHeader.Number = big.NewInt(2)
			bl2 := tests_utils.MakeBlockWithoutSeal(genesisHeader)
			blHash2 := bl2.Hash()
			committedSeal2, err := core.backend.Sign(utils.PrepareCommittedSeal(blHash2))
			require.NoError(t, err)
			require.NotEqual(t, bl1.Hash().Hex(), bl2.Hash().Hex(), "Block hash of 2 blocks must be different")

			var block2ExpectCommittedSeals [][]byte //It stores what commit seals were appended to block 2
			//Add vote from node 1,2,3,4
			for i := 0; i < len(validators); i++ {
				msg := message{
					Code:    msgPrecommit,
					Address: validators[i],
				}

				assert.Equal(t, validators[i].String(), core.valSet.GetByIndex(int64(i)).String(), "The order voting must be the same")
				switch tc.validatorVotes[i] {
				case Block1:
					ok, err := newMsgSet.AddVote(msg,
						&Vote{
							BlockHash:   &blHash1,
							BlockNumber: core.CurrentState().BlockNumber(),
							Round:       voteRound,
							Seal:        committedSeal1,
						})
					require.NoError(t, err)
					assert.True(t, ok)
				case Block2:
					vote := &Vote{
						BlockHash:   &blHash2,
						BlockNumber: core.CurrentState().BlockNumber(),
						Round:       voteRound,
						Seal:        committedSeal2,
					}
					ok, err := newMsgSet.AddVote(msg, vote)
					require.NoError(t, err)
					assert.True(t, ok)

					//Add committed seals will be added to block 2 to compare after finalizing
					if len(block2ExpectCommittedSeals) < core.valSet.MinMajority() {
						block2ExpectCommittedSeals = append(block2ExpectCommittedSeals, vote.Seal)
					}
				default:
					fmt.Println("Not support this case")
				}
			}

			//Check total received
			assert.Equal(t, 4, newMsgSet.totalReceived)
			core.currentState.PrecommitsReceived[voteRound] = newMsgSet
			assert.Equal(t, tc.totalReceived, core.currentState.PrecommitsReceived[voteRound].voteByBlock[blHash2].totalReceived, "Total Precommits Received on block 2 must be same when getting vote by block hash")

			//Check error after finalizing block
			finalizedBlock, err := core.FinalizeBlock(&Proposal{
				Block:    bl2,
				Round:    0,
				POLRound: 0,
			})
			tc.assertFn(finalizedBlock, err)

			if err == nil {
				//Check committed seals in header extra block after finalizing
				expectExtra, err := rlp.EncodeToBytes(&types.TendermintExtra{
					CommittedSeal: block2ExpectCommittedSeals,
				})
				require.Nil(t, err)
				expectCommittedSeals := append(bytes.Repeat([]byte{0x00}, types.TendermintExtraVanity), expectExtra...)
				assert.Equal(t, expectCommittedSeals, finalizedBlock.Header().Extra, "Make sure the committed seals is enough after finalizing")
			}
		}

		t.Run(tc.name, validateVote)
	}
}
