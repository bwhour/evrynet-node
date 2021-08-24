// Copyright 2016 The go-ethereum Authors
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

package evrclient

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ethereum "github.com/Evrynetlabs/evrynet-node"
	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/common/hexutil"
	"github.com/Evrynetlabs/evrynet-node/consensus/ethash"
	"github.com/Evrynetlabs/evrynet-node/core"
	"github.com/Evrynetlabs/evrynet-node/core/rawdb"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/evr"
	"github.com/Evrynetlabs/evrynet-node/node"
	"github.com/Evrynetlabs/evrynet-node/params"
)

// Verify that Client implements the ethereum interfaces.
var (
	_ = ethereum.ChainReader(&Client{})
	_ = ethereum.TransactionReader(&Client{})
	_ = ethereum.ChainStateReader(&Client{})
	_ = ethereum.ChainSyncReader(&Client{})
	_ = ethereum.ContractCaller(&Client{})
	_ = ethereum.GasEstimator(&Client{})
	_ = ethereum.GasPricer(&Client{})
	_ = ethereum.LogFilterer(&Client{})
	_ = ethereum.PendingStateReader(&Client{})
	// _ = ethereum.PendingStateEventer(&Client{})
	_ = ethereum.PendingContractCaller(&Client{})
)

func TestToFilterArg(t *testing.T) {
	blockHashErr := fmt.Errorf("cannot specify both BlockHash and FromBlock/ToBlock")

	address, _ := common.EvryAddressStringToAddressCheck("EcRhd3AvnF4cMN82WaPoytZrizvi77jquf")
	addresses := []common.Address{
		address,
	}
	blockHash := common.HexToHash(
		"0xeb94bb7d78b73657a9d7a99792413f50c0a45c51fc62bdcb08a53f18e9a2b4eb",
	)

	for _, testCase := range []struct {
		name   string
		input  ethereum.FilterQuery
		output interface{}
		err    error
	}{
		{
			"without BlockHash",
			ethereum.FilterQuery{
				Addresses: addresses,
				FromBlock: big.NewInt(1),
				ToBlock:   big.NewInt(2),
				Topics:    [][]common.Hash{},
			},
			map[string]interface{}{
				"address":   addresses,
				"fromBlock": "0x1",
				"toBlock":   "0x2",
				"topics":    [][]common.Hash{},
			},
			nil,
		},
		{
			"with nil fromBlock and nil toBlock",
			ethereum.FilterQuery{
				Addresses: addresses,
				Topics:    [][]common.Hash{},
			},
			map[string]interface{}{
				"address":   addresses,
				"fromBlock": "0x0",
				"toBlock":   "latest",
				"topics":    [][]common.Hash{},
			},
			nil,
		},
		{
			"with blockhash",
			ethereum.FilterQuery{
				Addresses: addresses,
				BlockHash: &blockHash,
				Topics:    [][]common.Hash{},
			},
			map[string]interface{}{
				"address":   addresses,
				"blockHash": blockHash,
				"topics":    [][]common.Hash{},
			},
			nil,
		},
		{
			"with blockhash and from block",
			ethereum.FilterQuery{
				Addresses: addresses,
				BlockHash: &blockHash,
				FromBlock: big.NewInt(1),
				Topics:    [][]common.Hash{},
			},
			nil,
			blockHashErr,
		},
		{
			"with blockhash and to block",
			ethereum.FilterQuery{
				Addresses: addresses,
				BlockHash: &blockHash,
				ToBlock:   big.NewInt(1),
				Topics:    [][]common.Hash{},
			},
			nil,
			blockHashErr,
		},
		{
			"with blockhash and both from / to block",
			ethereum.FilterQuery{
				Addresses: addresses,
				BlockHash: &blockHash,
				FromBlock: big.NewInt(1),
				ToBlock:   big.NewInt(2),
				Topics:    [][]common.Hash{},
			},
			nil,
			blockHashErr,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			output, err := toFilterArg(testCase.input)
			if (testCase.err == nil) != (err == nil) {
				t.Fatalf("expected error %v but got %v", testCase.err, err)
			}
			if testCase.err != nil {
				if testCase.err.Error() != err.Error() {
					t.Fatalf("expected error %v but got %v", testCase.err, err)
				}
			} else if !reflect.DeepEqual(testCase.output, output) {
				t.Fatalf("expected filter arg %v but got %v", testCase.output, output)
			}
		})
	}
}

var (
	testKey, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testAddr    = crypto.PubkeyToAddress(testKey.PublicKey)
	testBalance = new(big.Int).Exp(big.NewInt(10), big.NewInt(25), nil)

	testKey2, _  = crypto.HexToECDSA("ce900e4057ef7253ce737dccf3979ec4e74a19d595e8cc30c6c5ea92dfdd37f1")
	testAddr2    = crypto.PubkeyToAddress(testKey2.PublicKey)
	testBalance2 = new(big.Int).Exp(big.NewInt(10), big.NewInt(25), nil)
)

func newTestBackend(t *testing.T, txs types.Transactions) (*node.Node, []*types.Block) {
	// Generate test chain.
	genesis, blocks := generateTestChain(txs)

	// Start Evrynet service.
	var ethservice *evr.Evrynet
	n, err := node.New(&node.Config{})
	n.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		config := &evr.Config{Genesis: genesis}
		config.Ethash.PowMode = ethash.ModeFake
		ethservice, err = evr.New(ctx, config)
		n.P2PServerInitDone <- struct{}{}
		return ethservice, err
	})

	// Import the test chain.
	if err := n.Start(); err != nil {
		t.Fatalf("can't start test node: %v", err)
	}
	if _, err := ethservice.BlockChain().InsertChain(blocks[1:]); err != nil {
		t.Fatalf("can't import test blocks: %v", err)
	}
	return n, blocks
}

func generateTestChain(txs types.Transactions) (*core.Genesis, []*types.Block) {
	db := rawdb.NewMemoryDatabase()
	config := params.AllEthashProtocolChanges
	genesis := &core.Genesis{
		Config:    config,
		Alloc:     core.GenesisAlloc{testAddr: {Balance: testBalance}, testAddr2: {Balance: testBalance2}},
		ExtraData: []byte("test genesis"),
		Timestamp: 9000,
	}
	generate := func(i int, g *core.BlockGen) {
		g.OffsetTime(5)
		g.SetExtra([]byte("test"))
		for _, tx := range txs {
			g.AddTx(tx)
		}
	}
	gblock := genesis.ToBlock(db)
	engine := ethash.NewFaker()
	blocks, _ := core.GenerateChain(config, gblock, engine, db, 1, generate)
	blocks = append([]*types.Block{gblock}, blocks...)
	return genesis, blocks
}

func TestHeader(t *testing.T) {
	backend, chain := newTestBackend(t, nil)
	client, _ := backend.Attach()
	defer backend.Stop()
	defer client.Close()

	tests := map[string]struct {
		block   *big.Int
		want    *types.Header
		wantErr error
	}{
		"genesis": {
			block: big.NewInt(0),
			want:  chain[0].Header(),
		},
		"first_block": {
			block: big.NewInt(1),
			want:  chain[1].Header(),
		},
		"future_block": {
			block: big.NewInt(1000000000),
			want:  nil,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ec := NewClient(client)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			got, err := ec.HeaderByNumber(ctx, tt.block)
			if tt.wantErr != nil && (err == nil || err.Error() != tt.wantErr.Error()) {
				t.Fatalf("HeaderByNumber(%v) error = %q, want %q", tt.block, err, tt.wantErr)
			}
			if got != nil && got.Number.Sign() == 0 {
				got.Number = big.NewInt(0) // hack to make DeepEqual work
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("HeaderByNumber(%v)\n   = %v\nwant %v", tt.block, got, tt.want)
			}
		})
	}
}

func TestBalanceAt(t *testing.T) {
	backend, _ := newTestBackend(t, nil)
	client, _ := backend.Attach()
	defer backend.Stop()
	defer client.Close()

	tests := map[string]struct {
		account common.Address
		block   *big.Int
		want    *big.Int
		wantErr error
	}{
		"valid_account": {
			account: testAddr,
			block:   big.NewInt(1),
			want:    testBalance,
		},
		"non_existent_account": {
			account: common.Address{1},
			block:   big.NewInt(1),
			want:    big.NewInt(0),
		},
		"future_block": {
			account: testAddr,
			block:   big.NewInt(1000000000),
			want:    big.NewInt(0),
			wantErr: errors.New("header not found"),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ec := NewClient(client)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			got, err := ec.BalanceAt(ctx, tt.account, tt.block)
			if tt.wantErr != nil && (err == nil || err.Error() != tt.wantErr.Error()) {
				t.Fatalf("BalanceAt(%x, %v) error = %q, want %q", tt.account, tt.block, err, tt.wantErr)
			}
			if got.Cmp(tt.want) != 0 {
				t.Fatalf("BalanceAt(%x, %v) = %v, want %v", tt.account, tt.block, got, tt.want)
			}
		})
	}
}

func TestTransactionInBlockInterrupted(t *testing.T) {
	backend, _ := newTestBackend(t, nil)
	client, _ := backend.Attach()
	defer backend.Stop()
	defer client.Close()

	ec := NewClient(client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx, err := ec.TransactionInBlock(ctx, common.Hash{1}, 1)
	if tx != nil {
		t.Fatal("transaction should be nil")
	}
	if err == nil {
		t.Fatal("error should not be nil")
	}
}

func TestChainID(t *testing.T) {
	backend, _ := newTestBackend(t, nil)
	client, _ := backend.Attach()
	defer backend.Stop()
	defer client.Close()
	ec := NewClient(client)

	id, err := ec.ChainID(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == nil || id.Cmp(params.AllEthashProtocolChanges.ChainID) != 0 {
		t.Fatalf("ChainID returned wrong number: %+v", id)
	}
}

//TestGetTransactionByHash adds a test for ProviderSignTx
// 4 cases: normalTx, normalTxWithProviderAddress, txCreateContract, txCreateContractWithProvider
func TestGetTransactionByHash(t *testing.T) {
	var (
		chainID = params.AllEthashProtocolChanges.ChainID
		err     error
		payload = "0x608060405260d0806100126000396000f30060806040526004361060525763ffffffff7c01000000000000000000000000000000000000000000000000000000006000350416633fb5c1cb811460545780638381f58a14605d578063f2c9ecd8146081575b005b60526004356093565b348015606857600080fd5b50606f6098565b60408051918252519081900360200190f35b348015608c57600080fd5b50606f609e565b600055565b60005481565b600054905600a165627a7a723058209573e4f95d10c1e123e905d720655593ca5220830db660f0641f3175c1cdb86e0029"
	)
	to1, _ := common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeWdYvGRyE")
	to2, _ := common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeWdfucv31")
	tx := types.NewTransaction(uint64(0), to1, big.NewInt(100), 21000, big.NewInt(params.GasPriceConfig), nil)
	tx, err = types.SignTx(tx, types.NewEIP155Signer(chainID), testKey)
	require.NoError(t, err)

	txWithProvider := types.NewTransaction(uint64(0), to2, big.NewInt(1), 21000, big.NewInt(params.GasPriceConfig), nil)
	txWithProvider, err = types.SignTx(txWithProvider, types.NewEIP155Signer(chainID), testKey2)
	require.NoError(t, err)
	txWithProvider, err = types.ProviderSignTx(txWithProvider, types.NewEIP155Signer(chainID), testKey)
	require.NoError(t, err)

	data := hexutil.MustDecode(payload)
	creationContractTx := types.NewContractCreation(uint64(1), big.NewInt(0), 1000000, big.NewInt(params.GasPriceConfig), data)
	creationContractTx, err = types.SignTx(creationContractTx, types.NewEIP155Signer(chainID), testKey)
	require.NoError(t, err)

	owner, _ := common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8fmHkiJ")
	provider, _ := common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeW8hGraaK")

	opts := types.CreateAccountOption{
		OwnerAddress:    &owner,
		ProviderAddress: &provider,
	}
	creationEnterpriseContractTx := types.NewContractCreation(uint64(1), big.NewInt(0), 1000000, big.NewInt(params.GasPriceConfig), data, opts)
	creationEnterpriseContractTx, err = types.SignTx(creationEnterpriseContractTx, types.NewEIP155Signer(chainID), testKey2)
	require.NoError(t, err)

	backend, _ := newTestBackend(t, types.Transactions{tx, txWithProvider, creationContractTx, creationEnterpriseContractTx})
	client, _ := backend.Attach()
	defer backend.Stop()
	defer client.Close()
	ec := NewClient(client)
	tx0, _, err := ec.TransactionByHash(context.Background(), tx.Hash())
	require.NoError(t, err)
	require.Equal(t, tx0.Hash(), tx.Hash())
	msg, err := tx0.AsMessage(types.NewEIP155Signer(chainID))
	require.NoError(t, err)
	require.Equal(t, msg.From(), testAddr)

	tx1, _, err := ec.TransactionByHash(context.Background(), txWithProvider.Hash())
	require.NoError(t, err)
	require.Equal(t, tx1.Hash(), txWithProvider.Hash())
	msg, err = tx1.AsMessage(types.NewEIP155Signer(chainID))
	require.NoError(t, err)
	require.Equal(t, msg.From().Hex(), testAddr2.Hex())
	require.Equal(t, msg.GasPayer().Hex(), testAddr.Hex())

	tx2, _, err := ec.TransactionByHash(context.Background(), creationContractTx.Hash())
	require.NoError(t, err)
	require.Equal(t, tx2.Hash(), creationContractTx.Hash())
	msg, err = tx2.AsMessage(types.NewEIP155Signer(chainID))
	require.NoError(t, err)
	require.Equal(t, msg.From().Hex(), testAddr.Hex())

	tx3, _, err := ec.TransactionByHash(context.Background(), creationEnterpriseContractTx.Hash())
	require.NoError(t, err)
	require.Equal(t, tx3.Hash(), creationEnterpriseContractTx.Hash())
	msg, err = tx3.AsMessage(types.NewEIP155Signer(chainID))
	require.NoError(t, err)
	require.Equal(t, msg.From().Hex(), testAddr2.Hex())
	require.Equal(t, msg.GasPayer().Hex(), testAddr2.Hex())
}

func TestReplayAttackWithProviderAddress(t *testing.T) {
	var (
		err          error
		chainID      = big.NewInt(15)
		senderKey    = testKey2
		providerKey  = testKey
		providerAddr = testAddr
	)
	//Create atx and sign it with senderKey
	to, _ := common.EvryAddressStringToAddressCheck("EH9uVaqWRxHuzJbroqzX18yxmeWdfucv31")
	txWithProvider := types.NewTransaction(uint64(0), to, big.NewInt(1), 21000, big.NewInt(params.GasPriceConfig), nil)
	txWithProvider, err = types.SignTx(txWithProvider, types.NewEIP155Signer(chainID), senderKey)
	require.NoError(t, err)
	txWithProvider, err = types.ProviderSignTx(txWithProvider, types.NewEIP155Signer(chainID), providerKey)
	require.NoError(t, err)

	//copy the Provider Signature from it
	pv, pr, ps := txWithProvider.RawProviderSignatureValues()

	var fSigner = &fakeSigner{pv: pv, pr: pr, ps: ps, base: types.NewEIP155Signer(chainID)}

	replayTx := types.NewTransaction(uint64(0), to, big.NewInt(1), 21000, big.NewInt(params.GasPriceConfig), nil)
	//sign the message with the copied signature from the sender
	replayTx, err = types.SignTx(replayTx, fSigner, senderKey)
	require.NoError(t, err)

	msg, err := replayTx.AsMessage(types.NewEIP155Signer(chainID))
	require.NoError(t, err)
	require.NotEqual(t, msg.From().Hex(), providerAddr.Hex(), "The address from this relay attack will not success")
}

type fakeSigner struct {
	base types.Signer
	pv   *big.Int
	pr   *big.Int
	ps   *big.Int
}

func (f *fakeSigner) Sender(tx *types.Transaction) (common.Address, error) {
	return f.base.Sender(tx)
}

func (f *fakeSigner) Provider(tx *types.Transaction) (common.Address, error) {
	panic("implement me")
}

func (f *fakeSigner) SignatureValues(tx *types.Transaction, sig []byte) (r, s, v *big.Int, err error) {
	return f.pr, f.ps, f.pv, nil
}

func (f *fakeSigner) Hash(tx *types.Transaction) common.Hash {
	return f.base.Hash(tx)
}

func (f *fakeSigner) HashWithSender(tx *types.Transaction) (common.Hash, error) {
	return f.base.HashWithSender(tx)
}

func (f *fakeSigner) Equal(signer types.Signer) bool {
	panic("implement me")
}
