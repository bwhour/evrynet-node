package tests_utils

import (
	"bytes"
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/utils"
	"github.com/Evrynetlabs/evrynet-node/core"
	"github.com/Evrynetlabs/evrynet-node/core/rawdb"
	"github.com/Evrynetlabs/evrynet-node/core/state"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

// DefaultTestConfig with time is smaller than DefaultConfig for speed-up
var DefaultTestConfig = &tendermint.Config{
	ProposerPolicy:        tendermint.RoundRobin,
	Epoch:                 30000,
	BlockPeriod:           1,
	TimeoutPropose:        30 * time.Millisecond,
	TimeoutProposeDelta:   50 * time.Millisecond,
	TimeoutPrevote:        100 * time.Millisecond,
	TimeoutPrevoteDelta:   50 * time.Millisecond,
	TimeoutPrecommit:      100 * time.Millisecond,
	TimeoutPrecommitDelta: 50 * time.Millisecond,
	TimeoutCommit:         100 * time.Millisecond,
	FaultyMode:            tendermint.Disabled.Uint64(),
}

func MustGeneratePrivateKey(key string) *ecdsa.PrivateKey {
	privateKey, err := crypto.HexToECDSA(key)
	if err != nil {
		panic(err)
	}
	return privateKey
}

// ------------------------------------

func MakeNodeKey() *ecdsa.PrivateKey {
	key, _ := GeneratePrivateKey()
	return key
}

func MustCreateStateDB(t *testing.T) *state.StateDB {
	var (
		statedb, err = state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()))
	)
	if err != nil {
		t.Fatalf("failed to create stateDB, error %s", err)

	}
	return statedb
}

func MakeGenesisHeader(validators []common.Address) *types.Header {
	var header = &types.Header{
		Number:     big.NewInt(int64(0)),
		ParentHash: common.HexToHash("0x01"),
		UncleHash:  types.CalcUncleHash(nil),
		Root:       common.HexToHash("0x0"),
		Difficulty: big.NewInt(1),
		MixDigest:  types.TendermintDigest,
	}
	extra, _ := PrepareExtra(header)

	var buf bytes.Buffer
	buf.Write(extra[:types.TendermintExtraVanity])
	valSetData, _ := rlp.EncodeToBytes(validators)
	tdm := &types.TendermintExtra{
		Seal:          []byte{},
		CommittedSeal: [][]byte{},
		ValidatorAdds: valSetData,
	}
	payload, _ := rlp.EncodeToBytes(&tdm)

	header.Extra = append(buf.Bytes(), payload...)
	return header
}

func MakeBlockWithoutSeal(pHeader *types.Header) *types.Block {
	header := makeHeaderFromParent(types.NewBlockWithHeader(pHeader))
	return types.NewBlockWithHeader(header)
}

func MakeBlockWithSeal(be tendermint.Backend, pHeader *types.Header) *types.Block {
	header := makeHeaderFromParent(types.NewBlockWithHeader(pHeader))
	AppendSeal(header, be)
	return types.NewBlockWithHeader(header)
}

func MustMakeBlockWithCommittedSealInvalid(be tendermint.Backend, pHeader *types.Header) *types.Block {
	header := makeHeaderFromParent(types.NewBlockWithHeader(pHeader))
	AppendSeal(header, be)
	// generate a random private key not in validator set
	privateKey, _ := GeneratePrivateKey()
	//sign header with rand private key
	hashData := crypto.Keccak256(utils.SigHash(header).Bytes())
	invalidCommitSeal, _ := crypto.Sign(hashData, privateKey)
	AppendCommittedSeal(header, invalidCommitSeal)
	return types.NewBlockWithHeader(header)
}

func MustMakeBlockWithCommittedSeal(be tendermint.Backend, pHeader *types.Header) *types.Block {
	header := makeHeaderFromParent(types.NewBlockWithHeader(pHeader))
	AppendSeal(header, be)
	commitHash := utils.PrepareCommittedSeal(header.Hash())
	committedSeal, err := be.Sign(commitHash)
	if err != nil {
		panic(err)
	}
	AppendCommittedSeal(header, committedSeal)
	block := types.NewBlockWithHeader(header)
	return block.WithSeal(header)
}

//AppendSeal sign the header with the engine's key and write the seal to the input header's extra data
func AppendSeal(header *types.Header, be tendermint.Backend) {
	// sign the hash
	seal, _ := be.Sign(utils.SigHash(header).Bytes())
	_ = utils.WriteSeal(header, seal)
}

//AppendCommittedSeal
func AppendCommittedSeal(header *types.Header, committedSeal []byte) {
	//TODO: make this logic as the same as AppendSeal, which involve signing commit before writeCommittedSeal
	committedSeals := make([][]byte, 1)
	committedSeals[0] = make([]byte, types.TendermintExtraSeal)
	copy(committedSeals[0][:], committedSeal[:])
	_ = utils.WriteCommittedSeals(header, committedSeals)
}

//makeHeaderFromParent return a new block With valid information from its parents.
func makeHeaderFromParent(parent *types.Block) *types.Header {
	header := &types.Header{
		Coinbase:   GetAddress(),
		ParentHash: parent.Hash(),
		Number:     parent.Number().Add(parent.Number(), common.Big1),
		GasLimit:   core.CalcGasLimit(parent, parent.GasLimit(), parent.GasLimit()),
		GasUsed:    0,
		Difficulty: big.NewInt(1),
		MixDigest:  types.TendermintDigest,
	}
	extra, _ := PrepareExtra(header)
	header.Extra = extra
	return header
}

func GetAddress() common.Address {
	addr, _ := common.EvryAddressStringToAddressCheck("ETPomY44tjk5aaQVhVSeT5WBkq6n3QbRLr")
	return addr
}

func GeneratePrivateKey() (*ecdsa.PrivateKey, error) {
	return crypto.GenerateKey()
}

// PrepareExtra returns a extra-data of the given header and validators
func PrepareExtra(header *types.Header) ([]byte, error) {
	var buf bytes.Buffer

	// compensate the lack bytes if header.Extra is not enough TendermintExtraVanity bytes.
	if len(header.Extra) < types.TendermintExtraVanity {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, types.TendermintExtraVanity-len(header.Extra))...)
	}
	buf.Write(header.Extra[:types.TendermintExtraVanity])

	tdm := &types.TendermintExtra{}
	payload, err := rlp.EncodeToBytes(&tdm)
	if err != nil {
		return nil, err
	}

	return append(buf.Bytes(), payload...), nil
}
