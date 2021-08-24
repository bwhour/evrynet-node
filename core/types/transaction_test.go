// Copyright 2014 The go-ethereum Authors
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

package types

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/common/hexutil"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/params"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

// The values in those tests are from the Transaction Tests
// at github.com/ethereum/tests.
var (
	a, _    = common.EvryAddressStringToAddressCheck("EJ1Sm7ZPs136zds7axDPJ2LCGQtSi8B2AN")
	b, _    = common.EvryAddressStringToAddressCheck("Ea3jXjtJ3BqZneXsUKFhENkHDVT9y3TT1t")
	emptyTx = NewTransaction(
		0,
		a,
		big.NewInt(0), 0, big.NewInt(0),
		nil,
	)

	rightvrsTx, _ = NewTransaction(
		3,
		b,
		big.NewInt(10),
		2000,
		big.NewInt(1),
		common.FromHex("5544"),
	).WithSignature(
		HomesteadSigner{},
		common.Hex2Bytes("98ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4a8887321be575c8095f789dd4c743dfe42c1820f9231f98a962b210e3ac2452a301"),
	)

	testKey, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testAddr   = crypto.PubkeyToAddress(testKey.PublicKey)

	testKey2, _ = crypto.HexToECDSA("ce900e4057ef7253ce737dccf3979ec4e74a19d595e8cc30c6c5ea92dfdd37f1")
	testAddr2   = crypto.PubkeyToAddress(testKey2.PublicKey)
)

func TestTransactionCompatibility(t *testing.T) {
	unsignedEtherHashString := "0x32d0ec31372e18e22e4af25a00926e344c16473978dce150112a89d0bdf5795a"
	signedEthereHashString := "0x4aba6ccfa807932610b108dcb0f0c5491781bfb0d408e9909a967c6793a9c9ed"

	to, _ := common.EvryAddressStringToAddressCheck("EJ1Sm7ZPs136zds7axDPJ2LCGQtSi8B2AN")
	evrTx := NewTransaction(0, to, big.NewInt(1000), 0, big.NewInt(100), common.FromHex("123"))
	//unsigned version should be hashed to the same hash
	assert.Equal(t, evrTx.Hash().String(), unsignedEtherHashString)

	evrSignedTx := signTx(t, evrTx)
	//signed Tx should be hashed to the same hash
	assert.Equal(t, evrSignedTx.Hash().String(), signedEthereHashString)
}

func TestTransactionSigHash(t *testing.T) {
	var homestead HomesteadSigner
	if homestead.Hash(emptyTx) != common.HexToHash("c775b99e7ad12f50d819fcd602390467e28141316969f4b57f0626f74fe3b386") {
		t.Errorf("empty transaction hash mismatch, got %x", emptyTx.Hash())
	}
	if homestead.Hash(rightvrsTx) != common.HexToHash("fe7a79529ed5f7c3375d06b26b186a8644e0e16c373d7a12be41c62d6042b77a") {
		t.Errorf("RightVRS transaction hash mismatch, got %x", rightvrsTx.Hash())
	}
}

func TestTransactionEncode(t *testing.T) {
	txb, err := rlp.EncodeToBytes(rightvrsTx)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	should := common.FromHex("f86103018207d094b94f5374fce5edbc8e2a8697c15331677e6ebf0b0a8255441ca098ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4aa08887321be575c8095f789dd4c743dfe42c1820f9231f98a962b210e3ac2452a3")
	if !bytes.Equal(txb, should) {
		t.Errorf("encoded RLP mismatch, got %x", txb)
	}
}

func decodeTx(data []byte) (*Transaction, error) {
	var tx Transaction
	t, err := &tx, rlp.Decode(bytes.NewReader(data), &tx)

	return t, err
}

func defaultTestKey() (*ecdsa.PrivateKey, common.Address) {
	key, _ := crypto.HexToECDSA("45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8")
	addr := crypto.PubkeyToAddress(key.PublicKey)
	return key, addr
}

func TestRecipientEmpty(t *testing.T) {
	_, addr := defaultTestKey()
	tx, err := decodeTx(common.Hex2Bytes("f8498080808080011ca09b16de9d5bdee2cf56c28d16275a4da68cd30273e2525f3959f5d62557489921a0372ebd8fb3345f7db7b5a86d42e24d36e983e259b0664ceb8c227ec9af572f3d"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	from, err := Sender(HomesteadSigner{}, tx)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	if addr != from {
		t.Error("derived address doesn't match")
	}
}

func TestRecipientNormal(t *testing.T) {
	_, addr := defaultTestKey()

	tx, err := decodeTx(common.Hex2Bytes("f85d80808094000000000000000000000000000000000000000080011ca0527c0d8f5c63f7b9f41324a7c8a563ee1190bcbf0dac8ab446291bdbf32f5c79a0552c4ef0a09a04395074dab9ed34d3fbfb843c2f2546cc30fe89ec143ca94ca6"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	from, err := Sender(HomesteadSigner{}, tx)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if addr != from {
		t.Error("derived address doesn't match")
	}
}

// Tests that transactions can be correctly sorted according to their price in
// decreasing order, but at the same time with increasing nonces when issued by
// the same account.
func TestTransactionPriceNonceSort(t *testing.T) {
	// Generate a batch of accounts to start with
	keys := make([]*ecdsa.PrivateKey, 25)
	for i := 0; i < len(keys); i++ {
		keys[i], _ = crypto.GenerateKey()
	}

	signer := HomesteadSigner{}
	// Generate a batch of transactions with overlapping values, but shifted nonces
	groups := map[common.Address]Transactions{}
	for start, key := range keys {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		for i := 0; i < 25; i++ {
			tx, _ := SignTx(NewTransaction(uint64(start+i), common.Address{}, big.NewInt(100), 100, big.NewInt(int64(start+i)), nil), signer, key)
			groups[addr] = append(groups[addr], tx)
		}
	}
	// Sort the transactions and cross check the nonce ordering
	txset := NewTransactionsByPriceAndNonce(signer, groups)

	txs := Transactions{}
	for tx := txset.Peek(); tx != nil; tx = txset.Peek() {
		txs = append(txs, tx)
		txset.Shift()
	}
	if len(txs) != 25*25 {
		t.Errorf("expected %d transactions, found %d", 25*25, len(txs))
	}
	for i, txi := range txs {
		fromi, _ := Sender(signer, txi)

		// Make sure the nonce order is valid
		for j, txj := range txs[i+1:] {
			fromj, _ := Sender(signer, txj)

			if fromi == fromj && txi.Nonce() > txj.Nonce() {
				t.Errorf("invalid nonce ordering: tx #%d (A=%x N=%v) < tx #%d (A=%x N=%v)", i, fromi[:4], txi.Nonce(), i+j, fromj[:4], txj.Nonce())
			}
		}

		// If the next tx has different from account, the price must be lower than the current one
		if i+1 < len(txs) {
			next := txs[i+1]
			fromNext, _ := Sender(signer, next)
			if fromi != fromNext && txi.GasPrice().Cmp(next.GasPrice()) < 0 {
				t.Errorf("invalid gasprice ordering: tx #%d (A=%x P=%v) < tx #%d (A=%x P=%v)", i, fromi[:4], txi.GasPrice(), i+1, fromNext[:4], next.GasPrice())
			}
		}
	}
}

// TestTransactionJSON tests serializing/de-serializing to/from JSON.
func TestTransactionJSON(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("could not generate key: %v", err)
	}
	signer := NewEIP155Signer(common.Big1)

	transactions := make([]*Transaction, 0, 50)
	for i := uint64(0); i < 25; i++ {
		var tx *Transaction
		switch i % 2 {
		case 0:
			tx = NewTransaction(i, common.Address{1}, common.Big0, 1, common.Big2, []byte("abcdef"))
		case 1:
			tx = NewContractCreation(i, common.Big0, 1, common.Big2, []byte("abcdef"))
		}
		transactions = append(transactions, tx)

		signedTx, err := SignTx(tx, signer, key)
		if err != nil {
			t.Fatalf("could not sign transaction: %v", err)
		}

		transactions = append(transactions, signedTx)
	}

	for _, tx := range transactions {
		data, err := json.Marshal(tx)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		var parsedTx *Transaction
		if err := json.Unmarshal(data, &parsedTx); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		// compare nonce, price, gaslimit, recipient, amount, payload, V, R, S
		if tx.Hash() != parsedTx.Hash() {
			t.Errorf("parsed tx differs from original tx, want %v, got %v", tx, parsedTx)
		}
		if tx.ChainId().Cmp(parsedTx.ChainId()) != 0 {
			t.Errorf("invalid chain id, want %d, got %d", tx.ChainId(), parsedTx.ChainId())
		}
	}
}

func TestTransaction_AsMessage(t *testing.T) {
	var (
		chainID         = params.AllEthashProtocolChanges.ChainID
		err             error
		payload         = "0x608060405260d0806100126000396000f30060806040526004361060525763ffffffff7c01000000000000000000000000000000000000000000000000000000006000350416633fb5c1cb811460545780638381f58a14605d578063f2c9ecd8146081575b005b60526004356093565b348015606857600080fd5b50606f6098565b60408051918252519081900360200190f35b348015608c57600080fd5b50606f609e565b600055565b60005481565b600054905600a165627a7a723058209573e4f95d10c1e123e905d720655593ca5220830db660f0641f3175c1cdb86e0029"
		contractAddr, _ = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8hGraaK")
		to1, _          = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeWdYvGRyE")
		to2, _          = common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeWdfucv31")
		signer          = NewEIP155Signer(chainID)
	)
	tx := NewTransaction(uint64(0), to1, big.NewInt(100), 21000, big.NewInt(params.GasPriceConfig), nil)
	tx, err = SignTx(tx, signer, testKey)
	require.NoError(t, err)

	txWithProvider := NewTransaction(uint64(0), to2, big.NewInt(1), 21000, big.NewInt(params.GasPriceConfig), nil)
	txWithProvider, err = SignTx(txWithProvider, signer, testKey2)
	require.NoError(t, err)
	txWithProvider, err = ProviderSignTx(txWithProvider, signer, testKey)
	require.NoError(t, err)

	data := hexutil.MustDecode(payload)
	creationContractTx := NewContractCreation(uint64(1), big.NewInt(0), 1000000, big.NewInt(params.GasPriceConfig), data)
	creationContractTx, err = SignTx(creationContractTx, signer, testKey)
	require.NoError(t, err)

	invalidCreationContractTx, err := ProviderSignTx(creationContractTx, signer, testKey2)
	require.NoError(t, err)

	opts := CreateAccountOption{
		OwnerAddress:    &testAddr2,
		ProviderAddress: &testAddr2,
	}
	creationEnterpriseContractTx := NewContractCreation(uint64(1), big.NewInt(0), 1000000,
		big.NewInt(params.GasPriceConfig), data, opts)
	creationEnterpriseContractTx, err = SignTx(creationEnterpriseContractTx, signer, testKey)
	require.NoError(t, err)

	addProviderTx, err := NewModifyProvidersTransaction(uint64(2), contractAddr, 1000000,
		big.NewInt(params.GasPriceConfig), testAddr2, true)
	addProviderTx, err = SignTx(addProviderTx, signer, testKey)
	require.NoError(t, err)

	invalidAddProviderTx, err := NewModifyProvidersTransaction(uint64(2), contractAddr, 1000000,
		big.NewInt(params.GasPriceConfig), testAddr2, true)
	require.NoError(t, err)
	invalidAddProviderTx, err = SignTx(invalidAddProviderTx, signer, testKey)
	require.NoError(t, err)
	invalidAddProviderTx, err = ProviderSignTx(invalidAddProviderTx, signer, testKey2)
	require.NoError(t, err)

	var testCases = []struct {
		tx                      *Transaction
		expectedErr             error
		expectedFromAddress     common.Address
		expectedGasPayerAddress common.Address
		assertFn                func(msg Message)
	}{
		{
			tx:                      tx,
			expectedErr:             nil,
			expectedFromAddress:     testAddr,
			expectedGasPayerAddress: testAddr,
		}, {
			tx:                      txWithProvider,
			expectedErr:             nil,
			expectedFromAddress:     testAddr2,
			expectedGasPayerAddress: testAddr,
		}, {
			tx:                      creationContractTx,
			expectedErr:             nil,
			expectedFromAddress:     testAddr,
			expectedGasPayerAddress: testAddr,
		}, {
			tx:          invalidCreationContractTx,
			expectedErr: ErrRedundantProviderSignature,
		}, {
			tx:                      creationEnterpriseContractTx,
			expectedErr:             nil,
			expectedFromAddress:     testAddr,
			expectedGasPayerAddress: testAddr,
		}, {
			tx:                      addProviderTx,
			expectedErr:             nil,
			expectedFromAddress:     testAddr,
			expectedGasPayerAddress: testAddr,
			assertFn: func(msg Message) {
				require.Equal(t, *msg.to, contractAddr)
				require.Equal(t, msg.txType, AddProviderTxType)
				extraData, ok := msg.extraData.(ModifyProvidersMsg)
				require.True(t, ok)
				require.Equal(t, extraData.Provider, testAddr2)
			},
		},
	}

	for _, testCase := range testCases {
		msg, err := testCase.tx.AsMessage(signer)
		if testCase.expectedErr != nil {
			require.Error(t, err, testCase.expectedErr)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, msg.From(), testCase.expectedFromAddress, "unexpected from address")
		require.Equal(t, msg.GasPayer(), testCase.expectedGasPayerAddress, "unexpected gas payer address")
		if testCase.assertFn != nil {
			testCase.assertFn(msg)
		}
	}
}
