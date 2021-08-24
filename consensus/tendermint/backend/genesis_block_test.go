// This test to init a node with first set of validators
package backend

import (
	"crypto/ecdsa"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/tests_utils"
	"github.com/Evrynetlabs/evrynet-node/core"
	"github.com/Evrynetlabs/evrynet-node/core/rawdb"
	coreStaking "github.com/Evrynetlabs/evrynet-node/core/state/staking"
	"github.com/Evrynetlabs/evrynet-node/core/vm"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/evrdb"
	"github.com/Evrynetlabs/evrynet-node/log"
)

const (
	StakingSC       = "../tests/genesis_staking_sc.json" // 4 validator
	stakingEpoch    = 40
	FixedValidators = "../tests/genesis_fixed_validators.json" // 1 validators
)

var (
	nodePKString           = "bb047e5940b6d83354d9432db7c449ac8fca2248008aaa7271369880f9f11cc1"
	stakingScAddress, _    = common.EvryAddressStringToAddressCheck("EMHju71XkmyxKJgaqf1QnzxdwGu627GEnN")
	stakingSCValidatorKeys = []string{ // list privatekey for validators in genesis file
		"ce900e4057ef7253ce737dccf3979ec4e74a19d595e8cc30c6c5ea92dfdd37f1",
		"e74f3525fb69f193b51d33f4baf602c4572d81ede57907c61a62eaf9ed95374a",
		"276cd299f350174a6005525a523b59fccd4c536771e4876164adb9f1459b79e4",
	}
	stakingFaucetKeys = []string{
		"4BCADFCEB52765412B7CF3C4BA8B64D47E50A50AE78902C0CC5522B09562613E",
		"BCDEE223C39FBC96A76B4DDC65661ED4DDC0656FEA5A73A20CD00300F35D367F",
	}
)

func TestGenesisblockWithStakingSC(t *testing.T) {
	testCases := []struct {
		name        string
		genesisType string
		validators  int
	}{
		{
			name:        "StakingSC",
			genesisType: StakingSC,
			validators:  3,
		},
		{
			name:        "FixedValidators",
			genesisType: FixedValidators,
			validators:  1,
		},
	}
	for _, tc := range testCases {
		getValidators := func(t *testing.T) {
			backend, blockchain, _, err := createBlockchainAndBackendFromGenesis(tc.genesisType)
			assert.NoError(t, err)

			// Tested with 4 valset but it will break the test TestBackend_HandleMsg (not enough 2f+1)
			// So I only test 1 valset
			valSet, err := backend.valSetInfo.GetValSet(blockchain, big.NewInt(0))
			assert.NoError(t, err)
			assert.Equal(t, tc.validators, len(valSet.List()))

			valSet2 := backend.Validators(big.NewInt(0))
			assert.Equal(t, tc.validators, len(valSet2.List()))

			validator := valSet.GetByIndex(0)
			assert.NotNil(t, validator)
		}
		t.Run(tc.name, getValidators)
	}
}

func TestBackendCallGetListCandidateFromSC(t *testing.T) {
	// Must init log to show error when using log.Debug
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))

	backend, blockchain, _, err := createBlockchainAndBackendFromGenesis(StakingSC)
	assert.NoError(t, err)

	state, err := backend.chain.StateAt(backend.CurrentHeadBlock().Root())
	assert.NoError(t, err)

	header := backend.chain.CurrentHeader()
	stakingCaller := coreStaking.NewEVMStakingCaller(state, blockchain, header, backend.chain.Config(), vm.Config{})
	validators, err := stakingCaller.GetValidators(backend.stakingContractAddr)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(validators))
}

func TestBackendCallGetListCandidateFromStateDB(t *testing.T) {
	// Must init log to show error when using log.Debug
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))

	backend, _, _, err := createBlockchainAndBackendFromGenesis(StakingSC)
	assert.NoError(t, err)

	state, err := backend.chain.StateAt(backend.CurrentHeadBlock().Root())
	assert.NoError(t, err)

	stakingCaller := coreStaking.NewStateDbStakingCaller(state, coreStaking.DefaultConfig)
	validators, err := stakingCaller.GetValidators(backend.stakingContractAddr)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(validators))
}

type Config struct {
	Genesis    *core.Genesis
	Tendermint *tendermint.Config
}

func makeNodeConfig(genesisPath string) (*Config, error) {
	genesisConf, err := getGenesisConf(genesisPath)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	config.Genesis = genesisConf
	config.Tendermint = tendermint.DefaultConfig
	config.Tendermint.ProposerPolicy = tendermint.ProposerPolicy(genesisConf.Config.Tendermint.ProposerPolicy)
	config.Tendermint.Epoch = genesisConf.Config.Tendermint.Epoch
	config.Tendermint.FixedValidators = genesisConf.Config.Tendermint.FixedValidators
	config.Tendermint.StakingSCAddress = genesisConf.Config.Tendermint.StakingSCAddress
	return config, nil
}

func getGenesisConf(genesisPath string) (*core.Genesis, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Read file genesis generated from pupeth
	genesisFile, err := ioutil.ReadFile(filepath.Join(workingDir, genesisPath))
	if err != nil {
		return nil, err
	}

	config := &core.Genesis{}
	err = json.Unmarshal(genesisFile, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// createBlockchainAndBackendFromGenesis returns backend, blockchain and evr.Database for testing
func createBlockchainAndBackendFromGenesis(genesisPath string) (*Backend, *core.BlockChain, evrdb.Database, error) {
	config, err := makeNodeConfig(genesisPath)
	if err != nil {
		return nil, nil, nil, err
	}

	nodePK, err := crypto.HexToECDSA(nodePKString)
	if err != nil {
		return nil, nil, nil, err
	}

	dir, err := ioutil.TempDir("", "eth-chain-genesis")
	if err != nil {
		return nil, nil, nil, err
	}

	//create db instance with implement leveldb
	db, err := rawdb.NewLevelDBDatabase(dir, 128, 1024, "")
	if err != nil {
		return nil, nil, nil, err
	}

	//init tendermint backend
	backend := New(config.Tendermint, nodePK).(*Backend)
	backend.SetBroadcaster(&tests_utils.MockProtocolManager{})

	//set up genesis block
	chainConfig, _, err := core.SetupGenesisBlock(db, config.Genesis)
	if err != nil {
		return nil, nil, nil, err
	}

	//init block chain with tendermint engine
	blockchain, err := core.NewBlockChain(db, nil, chainConfig, backend, vm.Config{}, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	backend.chain = blockchain
	backend.currentBlock = blockchain.CurrentBlock
	return backend, blockchain, db, nil
}

func getValidatorAccounts() ([]*ecdsa.PrivateKey, []common.Address) {
	pks := make([]*ecdsa.PrivateKey, len(stakingSCValidatorKeys))
	addrs := make([]common.Address, len(stakingSCValidatorKeys))
	for i, pkHex := range stakingSCValidatorKeys {
		pk, _ := crypto.HexToECDSA(pkHex)
		pks[i] = pk
		addrs[i] = crypto.PubkeyToAddress(pk.PublicKey)
	}
	return pks, addrs
}

func getFaucetAccounts() ([]*ecdsa.PrivateKey, []common.Address) {
	pks := make([]*ecdsa.PrivateKey, len(stakingFaucetKeys))
	addrs := make([]common.Address, len(stakingFaucetKeys))
	for i, pkHex := range stakingFaucetKeys {
		pk, _ := crypto.HexToECDSA(pkHex)
		pks[i] = pk
		addrs[i] = crypto.PubkeyToAddress(pk.PublicKey)
	}
	return pks, addrs
}
