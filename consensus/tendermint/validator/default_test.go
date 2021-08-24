package validator

import (
	"log"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/crypto"
)

var (
	testAddress  = "ETPomY44tjk5aaQVhVSeT5WBkq6n3QbRLr"
	testAddress2 = "EZWrgXmSqtUc46GE7cr63r8sBh1MQefwDt"
)

func TestValidatorSet(t *testing.T) {
	testNewValidatorSet(t)
	testNormalValSet(t)
	testEmptyValSet(t)
	testMajorityFormulation(t)
}

func TestDefaultSet_GetNeighbor(t *testing.T) {
	var (
		a, _      = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8XVJyV9")
		b, _      = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8fmHkiJ")
		c, _      = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8hGraaK")
		d, _      = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8pZptqN")
		addresses = []common.Address{
			a,
			b,
			c,
			d,
		}
		valSet      = newDefaultSet(addresses, tendermint.RoundRobin, 0)
		neighbors   = valSet.GetNeighbors(addresses[0])
		expectedLen = 2
	)

	require.Equal(t, expectedLen, len(neighbors))
	require.True(t, neighbors[addresses[1]])
	require.True(t, neighbors[addresses[2]])
}

func testMajorityFormulation(t *testing.T) {
	var expectedMajority = map[int]int{
		1: 1, 2: 2, 3: 3, 4: 3, 5: 4, 6: 5, 7: 5,
	}

	for valSize, majority := range expectedMajority {
		var addresses []common.Address
		for i := 0; i < valSize; i++ {
			key, _ := crypto.GenerateKey()
			addresses = append(addresses, crypto.PubkeyToAddress(key.PublicKey))
		}
		valSet := NewSet(addresses, tendermint.RoundRobin, int64(0))
		require.Equal(t, majority, valSet.MinMajority())
	}
}

func testNewValidatorSet(t *testing.T) {
	const ValCnt = 3

	// Create 100 validators with random addresses
	var b []byte
	for i := 0; i < ValCnt; i++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)
		val := New(addr)
		log.Printf("index %d address %s", i, addr.String())

		b = append(b, val.Address().Bytes()...)
	}

	// Create ValidatorSet
	valSet := NewSet(ExtractValidators(b), tendermint.RoundRobin, int64(0))
	if valSet == nil {
		t.Errorf("the validator byte array cannot be parsed")
		t.FailNow()
	}

	// Check validators sorting: should be in ascending order
	for i := 0; i < ValCnt-1; i++ {
		val := valSet.GetByIndex(int64(i))
		nextVal := valSet.GetByIndex(int64(i + 1))
		if strings.Compare(val.String(), nextVal.String()) >= 0 {
			t.Errorf("validator set is not sorted in ascending order")
		}
	}
	valSet.CalcProposer(valSet.GetByIndex(0).Address(), 0)
	assert.Equal(t, valSet.GetProposer().Address().Hex(), valSet.GetByIndex(0).Address().Hex())
	valSet.CalcProposer(valSet.GetByIndex(0).Address(), 1)
	assert.Equal(t, valSet.GetProposer().Address().Hex(), valSet.GetByIndex(1).Address().Hex())
	valSet.CalcProposer(valSet.GetByIndex(0).Address(), 2)
	assert.Equal(t, valSet.GetProposer().Address().Hex(), valSet.GetByIndex(2).Address().Hex())
	valSet.CalcProposer(valSet.GetByIndex(0).Address(), 3)
	assert.Equal(t, valSet.GetProposer().Address().Hex(), valSet.GetByIndex(0).Address().Hex())

}

func testNormalValSet(t *testing.T) {
	addr1, _ := common.EvryAddressStringToAddressCheck(testAddress)
	addr2, _ := common.EvryAddressStringToAddressCheck(testAddress2)
	val1 := New(addr1)
	val2 := New(addr2)

	valSet := newDefaultSet([]common.Address{addr1, addr2}, tendermint.RoundRobin, int64(0))
	assert.NotNil(t, valSet, "the format of validator set is invalid")

	// check size
	if size := valSet.Size(); size != 2 {
		t.Errorf("the size of validator set is wrong: have %v, want 2", size)
	}
	// test get by index
	if val := valSet.GetByIndex(int64(0)); !reflect.DeepEqual(val, val1) {
		t.Errorf("validator mismatch: have %v, want %v", val, val1)
	}
	// test get by invalid index
	if val := valSet.GetByIndex(int64(2)); val != nil {
		t.Errorf("validator mismatch: have %v, want nil", val)
	}
	// test get by address
	if _, val := valSet.GetByAddress(addr2); !reflect.DeepEqual(val, val2) {
		t.Errorf("validator mismatch: have %v, want %v", val, val2)
	}
	// test get by invalid address
	invalidAddr, _ := common.EvryAddressStringToAddressCheck("EWkrWEDFaq1H5YgDWg6WWjW8JVBfZstza6")
	if _, val := valSet.GetByAddress(invalidAddr); val != nil {
		t.Errorf("validator mismatch: have %v, want nil", val)
	}

	blockHeight := 1
	valSetWilHeight := newDefaultSet([]common.Address{addr1, addr2}, tendermint.RoundRobin, int64(blockHeight))
	assert.NotNil(t, valSet, "the format of validator set is invalid")
	// test get by first index
	if val := valSetWilHeight.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("validator mismatch: have %v, want %v", val, val1)
	}
	valSetWilHeight.CalcProposer(addr1, int64(1))
	// test get by second index
	if val := valSetWilHeight.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("validator mismatch: have %v, want %v", val, val2)
	}
	//test Height of valSet
	if height := valSetWilHeight.Height(); !reflect.DeepEqual(height, int64(blockHeight)) {
		t.Errorf("height mismatch: have %v, want %v", height, blockHeight)
	}

}

func testEmptyValSet(t *testing.T) {
	valSet := NewSet(ExtractValidators([]byte{}), tendermint.RoundRobin, int64(0))
	if valSet == nil {
		t.Errorf("validator set should not be nil")
	}
}
