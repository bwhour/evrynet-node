package backend

import (
	"crypto/ecdsa"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/tests_utils"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/crypto/secp256k1"
)

func TestBackend_VerifyHeader(t *testing.T) {
	var (
		nodePKString = "bb047e5940b6d83354d9432db7c449ac8fca2248008aaa7271369880f9f11cc1"
		nodeAddr, _  = common.EvryAddressStringToAddressCheck("ETPomY44tjk5aaQVhVSeT5WBkq6n3QbRLr")
		validators   = []common.Address{
			nodeAddr,
		}
		genesisHeader = tests_utils.MakeGenesisHeader(validators)
	)
	nodePK, err := crypto.HexToECDSA(nodePKString)
	assert.NoError(t, err)

	cfg := tendermint.DefaultConfig
	cfg.FixedValidators = validators
	//create New test backend and newMockChain
	chain, engine := mustStartTestChainAndBackend(nodePK, genesisHeader, cfg)
	assert.NotNil(t, chain)
	assert.NotNil(t, engine)

	// without seal
	block := tests_utils.MakeBlockWithoutSeal(genesisHeader)
	assert.Equal(t, secp256k1.ErrInvalidSignatureLen, engine.VerifyHeader(engine.chain, block.Header(), false))

	// with seal but incorrect coinbase
	block = tests_utils.MakeBlockWithSeal(engine, genesisHeader)
	header := block.Header()
	header.Coinbase = common.Address{}
	tests_utils.AppendSeal(header, engine)
	assert.Equal(t, tendermint.ErrCoinBaseInvalid, engine.VerifyHeader(engine.chain, header, false))

	// without committed seal
	block = tests_utils.MakeBlockWithSeal(engine, genesisHeader)
	assert.Equal(t, tendermint.ErrEmptyCommittedSeals, engine.VerifyHeader(engine.chain, block.Header(), false))

	// with committed seal but is invalid
	block = tests_utils.MustMakeBlockWithCommittedSealInvalid(engine, genesisHeader)
	assert.Equal(t, tendermint.ErrInvalidCommittedSeals, engine.VerifyHeader(engine.chain, block.Header(), false))

	// with committed seal
	block = tests_utils.MustMakeBlockWithCommittedSeal(engine, genesisHeader)
	assert.NotNil(t, engine.chain)
	err = engine.VerifyHeader(engine.chain, block.Header(), false)
	assert.NoError(t, err)
}

func mustStartTestChainAndBackend(nodePK *ecdsa.PrivateKey, genesisHeader *types.Header, cfg *tendermint.Config) (*tests_utils.MockChainReader, *Backend) {
	var (
		config = tendermint.DefaultConfig
	)
	if cfg != nil {
		config = cfg
	}
	b, ok := New(config, nodePK).(*Backend)
	if !ok {
		panic("New() cannot be asserted back to backend")
	}
	b.SetBroadcaster(&tests_utils.MockProtocolManager{})

	currentBlock := func() *types.Block {
		tests_utils.AppendSeal(genesisHeader, b)
		return types.NewBlockWithHeader(genesisHeader)
	}

	chain := tests_utils.MockChainReader{
		GenesisHeader: genesisHeader,
		MockBlockChain: &tests_utils.MockBlockChain{
			MockCurrentBlock: currentBlock(),
		},
	}

	if err := b.Start(&chain, currentBlock, nil); err != nil {
		log.Panicf("cannot start backend, error:%v", err)
	}
	return &chain, b
}
