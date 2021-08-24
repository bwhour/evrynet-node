package staking_test

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Evrynetlabs/evrynet-node/accounts/abi/bind"
	"github.com/Evrynetlabs/evrynet-node/accounts/abi/bind/backends"
	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus/staking_contracts"
	"github.com/Evrynetlabs/evrynet-node/core"
	"github.com/Evrynetlabs/evrynet-node/core/state/staking"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/params"
)

const (
	privateKeyHex     = "ce900e4057ef7253ce737dccf3979ec4e74a19d595e8cc30c6c5ea92dfdd37f1"
	newCandidatePkHex = "4BCADFCEB52765412B7CF3C4BA8B64D47E50A50AE78902C0CC5522B09562613E"
	gasLimit          = 10000000
)

func TestEvmStakingCaller_GetValidators(t *testing.T) {
	testGetValidators(t, nil)
}

func TestStateDBStakingCaller_GetValidators(t *testing.T) {
	testGetValidators(t, staking.DefaultConfig)
}

func testGetValidators(t *testing.T, indexCfg *staking.IndexConfigs) {
	var (
		a, _       = common.EvryAddressStringToAddressCheck("EQzeFSroGjB4xodbMYP1qydXeWYgypGSJe")
		b, _       = common.EvryAddressStringToAddressCheck("EWmMyKETQCsTYEC3W51dZ3bpUWvn3XtrwG")
		c, _       = common.EvryAddressStringToAddressCheck("ENDA7pzFPhjW1FKiXSKXkGrUPxvPTxBBCQ")
		candidates = []common.Address{
			a,
			b,
		}
		epoch             = big.NewInt(300000)
		startBlock        = common.Big0
		maxValidatorSize  = big.NewInt(100)
		minValidatorStake = big.NewInt(20)
		minVoteCap        = big.NewInt(10)
		adminAddr         = a
		newCandidate      = c
	)

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	require.NoError(t, err)
	publicKey := privateKey.Public()
	addr := crypto.PubkeyToAddress(*publicKey.(*ecdsa.PublicKey))

	be := backends.NewSimulatedBackend(core.GenesisAlloc{
		addr: core.GenesisAccount{
			Balance: big.NewInt(0).Exp(big.NewInt(10), big.NewInt(18), nil),
		},
		newCandidate: core.GenesisAccount{
			Balance: new(big.Int).Mul(big.NewInt(gasLimit), big.NewInt(params.GasPriceConfig)),
		},
	}, gasLimit)

	authOpts := bind.NewKeyedTransactor(privateKey)
	authOpts.Nonce = big.NewInt(0)

	addr, tx, contract, err := staking_contracts.DeployStakingContracts(authOpts, be, candidates, candidates, epoch, startBlock, maxValidatorSize, minValidatorStake, minVoteCap, adminAddr)
	require.NoError(t, err)
	be.Commit()
	assertTxSuccess(t, be, tx.Hash())

	stakingCaller, err := be.GetStakingCaller(indexCfg)
	require.NoError(t, err)

	validators, err := stakingCaller.GetValidators(addr)
	require.NoError(t, err)
	require.Equal(t, len(validators), 2)
	//register new candidate, with 0 stake from owner
	authOpts = bind.NewKeyedTransactor(privateKey)
	authOpts.Nonce = big.NewInt(1)
	tx2, err := contract.Register(authOpts, newCandidate, newCandidate)
	require.NoError(t, err)
	be.Commit()
	assertTxSuccess(t, be, tx2.Hash())
	stakingCaller, err = be.GetStakingCaller(indexCfg)
	require.NoError(t, err)
	validators, err = stakingCaller.GetValidators(addr)
	require.NoError(t, err)
	require.Equal(t, len(validators), 2)

	data, err := contract.GetListCandidates(nil)
	require.Equal(t, len(data.Candidates), 3)
	require.NotNil(t, data)
	// new validator is voted
	ownerPk, _ := crypto.HexToECDSA(newCandidatePkHex)
	authOpts = bind.NewKeyedTransactor(ownerPk)
	authOpts.Nonce = big.NewInt(0)
	authOpts.Value = big.NewInt(1000)
	tx3, err := contract.Vote(authOpts, newCandidate)
	require.NoError(t, err)

	authOpts = bind.NewKeyedTransactor(privateKey)
	authOpts.Nonce = big.NewInt(2)
	authOpts.Value = big.NewInt(30)
	tx4, err := contract.Vote(authOpts, newCandidate)
	require.NoError(t, err)
	be.Commit()
	assertTxSuccess(t, be, tx3.Hash())
	assertTxSuccess(t, be, tx4.Hash())
	stakingCaller, err = be.GetStakingCaller(indexCfg)
	require.NoError(t, err)
	validators, err = stakingCaller.GetValidators(addr)
	require.NoError(t, err)
	require.Equal(t, 3, len(validators))
	for _, val := range validators {
		fmt.Println(common.AddressToEvryAddressString(val))
	}
}

func TestEvmStakingCaller_GetValidatorsData(t *testing.T) {
	testGetValidatorsData(t, nil)
}

func TestStateDBStakingCaller_GetValidatorsData(t *testing.T) {
	testGetValidatorsData(t, staking.DefaultConfig)
}

func testGetValidatorsData(t *testing.T, indexCfg *staking.IndexConfigs) {
	var (
		a, _       = common.EvryAddressStringToAddressCheck("EQzeFSroGjB4xodbMYP1qydXeWYgypGSJe")
		b, _       = common.EvryAddressStringToAddressCheck("EWmMyKETQCsTYEC3W51dZ3bpUWvn3XtrwG")
		c, _       = common.EvryAddressStringToAddressCheck("ENDA7pzFPhjW1FKiXSKXkGrUPxvPTxBBCQ")
		candidates = []common.Address{
			a,
			b,
		}
		epoch             = big.NewInt(300000)
		startBlock        = common.Big0
		maxValidatorSize  = big.NewInt(100)
		minValidatorStake = big.NewInt(20)
		minVoteCap        = big.NewInt(10)
		adminAddr         = a
		newCandidate      = c
	)

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	require.NoError(t, err)
	publicKey := privateKey.Public()
	addr := crypto.PubkeyToAddress(*publicKey.(*ecdsa.PublicKey))

	be := backends.NewSimulatedBackend(core.GenesisAlloc{
		addr: core.GenesisAccount{
			Balance: big.NewInt(0).Exp(big.NewInt(10), big.NewInt(18), nil),
		},
		newCandidate: core.GenesisAccount{
			Balance: new(big.Int).Mul(big.NewInt(gasLimit), big.NewInt(params.GasPriceConfig)),
		},
	}, gasLimit)

	authOpts := bind.NewKeyedTransactor(privateKey)
	authOpts.Nonce = big.NewInt(0)

	addr, tx, contract, err := staking_contracts.DeployStakingContracts(authOpts, be, candidates, candidates, epoch, startBlock, maxValidatorSize, minValidatorStake, minVoteCap, adminAddr)
	require.NoError(t, err)
	be.Commit()
	assertTxSuccess(t, be, tx.Hash())

	stakingCaller, err := be.GetStakingCaller(indexCfg)
	require.NoError(t, err)

	validators, err := stakingCaller.GetValidators(addr)
	require.NoError(t, err)
	require.Equal(t, len(validators), 2)
	//register new candidate, with 0 stake from owner
	authOpts = bind.NewKeyedTransactor(privateKey)
	authOpts.Nonce = big.NewInt(1)
	tx2, err := contract.Register(authOpts, newCandidate, newCandidate)
	require.NoError(t, err)
	be.Commit()
	assertTxSuccess(t, be, tx2.Hash())
	stakingCaller, err = be.GetStakingCaller(indexCfg)
	require.NoError(t, err)
	validators, err = stakingCaller.GetValidators(addr)
	require.NoError(t, err)
	require.Equal(t, len(validators), 2)

	data, err := contract.GetListCandidates(nil)
	require.Equal(t, len(data.Candidates), 3)
	// new validator is voted
	ownerPk, _ := crypto.HexToECDSA(newCandidatePkHex)
	authOpts = bind.NewKeyedTransactor(ownerPk)
	authOpts.Nonce = big.NewInt(0)
	authOpts.Value = big.NewInt(1000)
	tx3, err := contract.Vote(authOpts, newCandidate)
	require.NoError(t, err)

	authOpts = bind.NewKeyedTransactor(privateKey)
	authOpts.Nonce = big.NewInt(2)
	authOpts.Value = big.NewInt(30)
	tx4, err := contract.Vote(authOpts, newCandidate)
	require.NoError(t, err)
	be.Commit()
	assertTxSuccess(t, be, tx3.Hash())
	assertTxSuccess(t, be, tx4.Hash())
	stakingCaller, err = be.GetStakingCaller(indexCfg)
	require.NoError(t, err)
	validators, err = stakingCaller.GetValidators(addr)
	require.NoError(t, err)
	require.Equal(t, 3, len(validators))
	// test get voter stake
	voterStakes, err := stakingCaller.GetValidatorsData(addr, validators)
	require.NoError(t, err)
	assert.Equal(t, len(voterStakes), 3)
	require.Contains(t, voterStakes, newCandidate)
	assert.Equal(t, voterStakes[newCandidate].Owner, newCandidate)
	require.Contains(t, voterStakes[newCandidate].VoterStakes, newCandidate)
	assert.Equal(t, voterStakes[newCandidate].VoterStakes[newCandidate], big.NewInt(1000))
	require.Contains(t, voterStakes[newCandidate].VoterStakes, adminAddr)
	assert.Equal(t, voterStakes[newCandidate].VoterStakes[adminAddr], big.NewInt(30))
}

func assertTxSuccess(t *testing.T, be *backends.SimulatedBackend, txHash common.Hash) {
	receipt, err := be.TransactionReceipt(context.Background(), txHash)
	require.NoError(t, err)
	require.Equal(t, uint64(1), receipt.Status)
}
