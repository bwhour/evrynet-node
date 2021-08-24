package backend

import (
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/common/hexutil"
	"github.com/Evrynetlabs/evrynet-node/consensus"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/tests_utils"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/utils"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/crypto/secp256k1"
)

// TestSimulateSubscribeAndReceiveToSeal is a simple test to pass a block to backend.Seal()
// on core.handleEvents(), the block is received.
func TestSimulateSubscribeAndReceiveToSeal(t *testing.T) {
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

	//create New test backend and newMockChain
	be := mustCreateAndStartNewBackend(t, nodePK, genesisHeader, validators)

	// without seal
	block := tests_utils.MakeBlockWithoutSeal(genesisHeader)
	assert.Equal(t, secp256k1.ErrInvalidSignatureLen, be.VerifyHeader(be.chain, block.Header(), false))

	err = be.Seal(be.chain, block, nil, nil)

	// Sleep to make sure that the block can be received from
	time.Sleep(2 * time.Second)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

}

// TestAuthor is a simple test to pass a block to backend.Seal()
// on core.handleEvents(), the block is received.
func TestAuthor(t *testing.T) {
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

	//create New test backend and newMockChain
	be := mustCreateAndStartNewBackend(t, nodePK, genesisHeader, validators)

	block := tests_utils.MakeBlockWithSeal(be, genesisHeader)
	header := block.Header()
	signer, err := be.Author(header)
	assert.NoError(t, err)
	assert.Equal(t, be.Address(), signer)
}

// TestPrepare
func TestPrepare(t *testing.T) {
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

	//create New test backend and newMockChain
	be := mustCreateAndStartNewBackend(t, nodePK, genesisHeader, validators)

	block := tests_utils.MakeBlockWithoutSeal(genesisHeader)
	header := block.Header()

	err = be.Prepare(be.chain, header)
	assert.NoError(t, err)

	header.ParentHash = common.HexToHash("1234567890")
	err = be.Prepare(be.chain, header)
	assert.Equal(t, consensus.ErrUnknownAncestor, err)
}

// TestVerifySeal
func TestVerifySeal(t *testing.T) {
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

	//create New test backend and newMockChain
	be := mustCreateAndStartNewBackend(t, nodePK, genesisHeader, validators)

	// cannot verify genesis
	err = be.VerifySeal(be.chain, genesisHeader)
	assert.Equal(t, tendermint.ErrUnknownBlock, err)

	block := tests_utils.MakeBlockWithSeal(be, genesisHeader)
	err = be.VerifySeal(be.chain, block.Header())
	assert.NoError(t, err)
}

// TestPrepareExtra
// 0xc280c0 (empty tendermint extra)
func TestPrepareExtra(t *testing.T) {
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
	assert.Equal(t, true, engine.coreStarted)

	vanity := make([]byte, types.TendermintExtraVanity)
	data := hexutil.MustDecode("0xc380c080")
	expectedResult := append(vanity, data...)

	header := &types.Header{
		Extra:  vanity,
		Number: big.NewInt(0),
	}

	header.Extra = engine.prepareExtra(header)
	assert.Equal(t, expectedResult, header.Extra)

	// append useless information to extra-data
	header.Extra = append(vanity, make([]byte, 15)...)

	header.Extra = engine.prepareExtra(header)
	assert.Equal(t, expectedResult, header.Extra)
}

// TestBackend_VerifyHeaders tests a case when both transition block is in ChainReader and transition block is not in ChainReader
func TestBackend_VerifyHeaders(t *testing.T) {
	t.Parallel()
	var (
		config        = tendermint.DefaultConfig
		epochSize     = 5
		nodePKString  = "bb047e5940b6d83354d9432db7c449ac8fca2248008aaa7271369880f9f11cc1"
		nodeAddr, _   = common.EvryAddressStringToAddressCheck("ETPomY44tjk5aaQVhVSeT5WBkq6n3QbRLr")
		nodePKString2 = "ce900e4057ef7253ce737dccf3979ec4e74a19d595e8cc30c6c5ea92dfdd37f1"
		nodeAddr2, _  = common.EvryAddressStringToAddressCheck("EQzeFSroGjB4xodbMYP1qydXeWYgypGSJe")
		nodePKString3 = "e74f3525fb69f193b51d33f4baf602c4572d81ede57907c61a62eaf9ed95374a"
		nodeAddr3, _  = common.EvryAddressStringToAddressCheck("EWmMyKETQCsTYEC3W51dZ3bpUWvn3XtrwG")
	)

	pk, _ := crypto.HexToECDSA(nodePKString)
	pk2, _ := crypto.HexToECDSA(nodePKString2)
	pk3, _ := crypto.HexToECDSA(nodePKString3)

	config.Epoch = uint64(epochSize)
	stakingSC, _ := common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeWAULFYZT")
	config.StakingSCAddress = &stakingSC
	config.FixedValidators = nil
	be := New(config, pk).(*Backend)
	// create genesis headers
	header0 := &types.Header{
		Number:     big.NewInt(0),
		Root:       common.HexToHash("0x0"),
		ParentHash: common.HexToHash("0x0"),
	}
	extra, _ := tests_utils.PrepareExtra(header0)
	header0.Extra = extra
	require.NoError(t, utils.WriteValSet(header0, []common.Address{nodeAddr}))

	createHeader := func(nodePK *ecdsa.PrivateKey, number int64, parent common.Hash, validators []common.Address) *types.Header {
		addr := crypto.PubkeyToAddress(*(nodePK.Public().(*ecdsa.PublicKey)))
		header := &types.Header{
			Coinbase:   addr,
			Number:     big.NewInt(number),
			ParentHash: parent,
			Root:       common.BytesToHash(big.NewInt(number).Bytes()),
			GasLimit:   0,
			GasUsed:    0,
			Difficulty: big.NewInt(1),
			MixDigest:  types.TendermintDigest,
		}
		extra, _ := tests_utils.PrepareExtra(header)
		header.Extra = extra
		if validators != nil {
			require.NoError(t, utils.WriteValSet(header, validators))
		}

		hash := utils.SigHash(header).Bytes()
		seal, err := crypto.Sign(crypto.Keccak256(hash), nodePK)
		require.NoError(t, err)
		require.NoError(t, utils.WriteSeal(header, seal))

		commitHash := utils.PrepareCommittedSeal(header.Hash())
		committedSeal, err := crypto.Sign(crypto.Keccak256(commitHash), nodePK)
		if err != nil {
			panic(err)
		}
		tests_utils.AppendCommittedSeal(header, committedSeal)
		return header
	}
	hash := header0.Hash()

	var (
		chainHeaders   = []*types.Header{header0}
		verifiedHeader []*types.Header
	)

	//create header for epoch 0
	for i := 0; i < epochSize; i++ {
		var header *types.Header
		if i != epochSize-1 {
			header = createHeader(pk, int64(i+1), hash, nil)
		} else {
			header = createHeader(pk, int64(i+1), hash, []common.Address{nodeAddr2})
		}
		hash = header.Hash()
		chainHeaders = append(chainHeaders, header)
	}
	//create header for epoch 1
	for i := 0; i < epochSize; i++ {
		var header *types.Header
		if i != epochSize-1 {
			header = createHeader(pk2, int64(i+epochSize+1), hash, nil)
		} else {
			header = createHeader(pk2, int64(i+epochSize+1), hash, []common.Address{nodeAddr3})
		}
		hash = header.Hash()
		verifiedHeader = append(verifiedHeader, header)
	}
	// create 1 header for epoch 2
	finalizeHeader := createHeader(pk3, int64(2*epochSize+1), hash, nil)
	verifiedHeader = append(verifiedHeader, finalizeHeader)

	chainReader := tests_utils.NewHeadersMockChainReader(chainHeaders)
	//verify header if transition block is in ChainReader
	require.NoError(t, be.VerifyHeader(chainReader, verifiedHeader[0], true))
	//verify header if transition block is not in ChainReader
	abort, results := be.VerifyHeaders(chainReader, verifiedHeader, nil)
	defer close(abort)
	for i := 0; i < len(verifiedHeader); i++ {
		re := <-results
		require.NoError(t, re)
	}
}
