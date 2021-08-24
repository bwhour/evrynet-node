package staking

import (
	"context"
	"math/big"
	"sort"
	"strings"

	"github.com/pkg/errors"

	evrynet "github.com/Evrynetlabs/evrynet-node"
	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/common/math"
	"github.com/Evrynetlabs/evrynet-node/consensus"
	"github.com/Evrynetlabs/evrynet-node/consensus/staking_contracts"
	"github.com/Evrynetlabs/evrynet-node/core"
	"github.com/Evrynetlabs/evrynet-node/core/state"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/core/vm"
	"github.com/Evrynetlabs/evrynet-node/params"
)

// evmStakingCaller creates a wrapper with statedb to implements ContractCaller
type evmStakingCaller struct {
	blockNumber  *big.Int
	header       *types.Header
	stateDB      *state.StateDB
	chainContext core.ChainContext
	chainConfig  *params.ChainConfig
	vmConfig     vm.Config
}

// GetValidators returns validators from stateDB and block number of the caller by smart-contract's address
func (caller *evmStakingCaller) GetValidators(scAddress common.Address) ([]common.Address, error) {
	var (
		candidatesArr []common.Address
		stakesArr     []*big.Int
	)
	sc, err := staking_contracts.NewStakingContractsCaller(scAddress, caller)
	if err != nil {
		return nil, err
	}
	data, err := sc.GetListCandidates(nil)
	if err != nil {
		return nil, err
	}
	// sanity checks
	if len(data.Candidates) != len(data.Stakes) {
		return nil, ErrLengthOfCandidatesAndStakesMisMatch
	}
	// check and remove if owner stake of candidate is greater or equal minValidatorStake
	minValidatorStake := data.MinValidatorCap
	for i, candidate := range data.Candidates {
		owner, err := sc.GetCandidateOwner(nil, candidate)
		if err != nil {
			return nil, err
		}
		stake, err := sc.GetVoterStake(nil, candidate, owner)
		if err != nil {
			return nil, err
		}
		if stake.Cmp(minValidatorStake) < 0 {
			continue
		}
		candidatesArr = append(candidatesArr, data.Candidates[i])
		stakesArr = append(stakesArr, data.Stakes[i])
	}
	if len(candidatesArr) == 0 || len(stakesArr) == 0 {
		return nil, ErrEmptyValidatorSet
	}

	if len(candidatesArr) < int(data.ValidatorSize.Int64()) {
		return candidatesArr, nil
	}
	// sort and returns topN
	stakes := make(map[common.Address]*big.Int)
	for i := 0; i < len(candidatesArr); i++ {
		stakes[candidatesArr[i]] = stakesArr[i]
	}
	sort.Slice(candidatesArr, func(i, j int) bool {
		if stakes[candidatesArr[i]].Cmp(stakes[candidatesArr[j]]) == 0 {
			return strings.Compare(candidatesArr[i].String(), candidatesArr[j].String()) > 0
		}
		return stakes[candidatesArr[i]].Cmp(stakes[candidatesArr[j]]) > 0
	})
	return candidatesArr[:int(data.ValidatorSize.Int64())], err
}

// GetValidatorsData return information of validators including owner, totalStake and voterStakes
func (caller *evmStakingCaller) GetValidatorsData(scAddress common.Address, candidates []common.Address) (map[common.Address]CandidateData, error) {
	sc, err := staking_contracts.NewStakingContractsCaller(scAddress, caller)
	if err != nil {
		return nil, err
	}

	allVoterStake := make(map[common.Address]CandidateData)
	for _, candidate := range candidates {
		candidateData, err := sc.GetCandidateData(nil, candidate)
		if err != nil {
			return nil, err
		}
		voters, err := sc.GetVoters(nil, candidate)
		if err != nil {
			return nil, err
		}
		voterStakes, err := sc.GetVoterStakes(nil, candidate, voters)
		if err != nil {
			return nil, err
		}
		if len(voterStakes) != len(voters) { // if this happens, blame Mike not me
			return nil, ErrLengthOfVotesAndStakesMisMatch
		}
		voteStakes := make(map[common.Address]*big.Int)
		for i := range voterStakes {
			voteStakes[voters[i]] = voterStakes[i]
		}
		allVoterStake[candidate] = CandidateData{
			VoterStakes: voteStakes,
			Owner:       candidateData.Owner,
			TotalStake:  candidateData.TotalStake,
		}
	}
	return allVoterStake, nil
}

// Deprecated: Using NewStateDbStakingCaller instead of
// NewBECaller returns staking caller which reads data from staking smart-contract by execute a call from evm
func NewEVMStakingCaller(stateDB *state.StateDB, chainContext core.ChainContext, header *types.Header,
	chainConfig *params.ChainConfig, vmConfig vm.Config) StakingCaller {
	return &evmStakingCaller{
		stateDB:      stateDB,
		chainContext: chainContext,
		blockNumber:  header.Number,
		header:       header,
		chainConfig:  chainConfig,
		vmConfig:     vmConfig,
	}
}

// CodeAt returns the code of the given account. This is needed to differentiate
// between contract internal errors and the local chain being out of sync.
func (caller *evmStakingCaller) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	return caller.stateDB.GetCode(contract), nil
}

// ContractCall executes an Evrynet contract call with the specified data as the
// input.
func (caller *evmStakingCaller) CallContract(ctx context.Context, call evrynet.CallMsg, blockNumber *big.Int) ([]byte, error) {
	clonedStateDB := caller.stateDB.Copy()
	if blockNumber != nil && blockNumber.Cmp(caller.blockNumber) != 0 {
		return nil, errors.New("blockNumber is not supported")
	}
	if call.GasPrice == nil {
		call.GasPrice = big.NewInt(1)
	}
	if call.Gas == 0 {
		call.Gas = maxGasGetValSet
	}
	if call.Value == nil {
		call.Value = new(big.Int)
	}
	from := clonedStateDB.GetOrNewStateObject(call.From)
	from.SetBalance(math.MaxBig256)
	// Execute the call.
	msg := callmsg{call}
	evmContext := core.NewEVMContext(msg, caller.header, caller.chainContext, nil)
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	vmenv := vm.NewEVM(evmContext, clonedStateDB, caller.chainConfig, caller.vmConfig)
	defer vmenv.Cancel()
	gaspool := new(core.GasPool).AddGas(maxGasGetValSet)
	rval, _, _, err := core.NewStateTransition(vmenv, msg, gaspool).TransitionDb()
	return rval, err
}

// callmsg implements core.Message to allow passing it as a transaction simulator.
type callmsg struct {
	evrynet.CallMsg
}

func (m callmsg) GasPayer() common.Address      { return m.CallMsg.From }
func (m callmsg) Owner() *common.Address        { return nil }
func (m callmsg) Provider() *common.Address     { return nil }
func (m callmsg) From() common.Address          { return m.CallMsg.From }
func (m callmsg) Nonce() uint64                 { return 0 }
func (m callmsg) CheckNonce() bool              { return false }
func (m callmsg) To() *common.Address           { return m.CallMsg.To }
func (m callmsg) GasPrice() *big.Int            { return m.CallMsg.GasPrice }
func (m callmsg) Gas() uint64                   { return m.CallMsg.Gas }
func (m callmsg) Value() *big.Int               { return m.CallMsg.Value }
func (m callmsg) Data() []byte                  { return m.CallMsg.Data }
func (m callmsg) TxType() types.TransactionType { return types.NormalTxType }
func (m callmsg) ExtraData() interface{}        { return nil }
func (m callmsg) HasProviderSignature() bool    { return false }

type chainContextWrapper struct {
	engine      consensus.Engine
	getHeaderFn func(common.Hash, uint64) *types.Header
}

func (w *chainContextWrapper) Engine() consensus.Engine {
	return w.engine
}

func (w *chainContextWrapper) GetHeader(hash common.Hash, height uint64) *types.Header {
	return w.getHeaderFn(hash, height)
}

func NewChainContextWrapper(engine consensus.Engine, getHeaderFn func(common.Hash, uint64) *types.Header) core.ChainContext {
	return &chainContextWrapper{
		engine:      engine,
		getHeaderFn: getHeaderFn,
	}
}
