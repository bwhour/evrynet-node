package test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/common/hexutil"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/evrclient"
)

//TestProviderSignTransaction will sign a transaction with both sender's Key and Providers's Key
//Note that the account must be unlocked prior to run this test
//The JSON rpc test can be call as
//curl <rpcserver> -H "Content-Type: application/json" -X POST --data '{"jsonrpc":"2.0","method":"eth_providerSignTransaction","params":["<raw_tx>", "<provider_address>"],"id":1}'
func TestProviderSignTransaction(t *testing.T) {
	contractAddr := prepareNewContract(false)
	assert.NotNil(t, contractAddr)

	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	providerAddr, _ := common.EvryAddressStringToAddressCheck(providerAddrStr)
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	id, err := ethClient.ChainID(context.Background())
	signer := types.NewEIP155Signer(id)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	tx := types.NewTransaction(nonce, *contractAddr, big.NewInt(1000000), testGasLimit, gasPrice, nil)
	txSigned, err := types.SignTx(tx, signer, spk)
	assert.NoError(t, err)

	ppk, err := crypto.HexToECDSA(providerPK)
	// Check Tx for existion
	_, err = types.ProviderSignTx(txSigned, signer, ppk)
	assert.NoError(t, err)

	// Get Tx via RPC
	pTxSigned, err := ethClient.ProviderSignTx(context.Background(), txSigned, &providerAddr)
	assert.NoError(t, err)
	assert.NotEqual(t, nil, pTxSigned)
}

func prepareNewContract(hasProvider bool) *common.Address {
	var (
		tx              *types.Transaction
		providerAddr, _ = common.EvryAddressStringToAddressCheck(providerAddrStr)
		ownerAddr, _    = common.EvryAddressStringToAddressCheck(senderAddrStr)
		sender, _       = common.EvryAddressStringToAddressCheck(senderAddrStr)
	)

	spk, err := crypto.HexToECDSA(senderPK)
	if err != nil {
		return nil
	}
	payLoadBytes, err := hexutil.Decode(payload)
	if err != nil {
		return nil
	}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	if err != nil {
		return nil
	}
	nonce, err := ethClient.PendingNonceAt(context.Background(), sender)
	if err != nil {
		return nil
	}

	if hasProvider {
		var option types.CreateAccountOption
		option.ProviderAddress = &providerAddr
		option.OwnerAddress = &ownerAddr
		tx = types.NewContractCreation(nonce, big.NewInt(0), testGasLimit, big.NewInt(testGasPrice), payLoadBytes, option)
	} else {
		tx = types.NewContractCreation(nonce, big.NewInt(0), testGasLimit, big.NewInt(testGasPrice), payLoadBytes)
	}

	tx, err = types.SignTx(tx, types.HomesteadSigner{}, spk)
	if err != nil {
		return nil
	}

	err = ethClient.SendTransaction(context.Background(), tx)
	if err != nil {
		panic(err)
	}
	for i := 0; i < 10; i++ {
		var receipt *types.Receipt
		receipt, err = ethClient.TransactionReceipt(context.Background(), tx.Hash())
		if err == nil && receipt.Status == uint64(1) {
			return &receipt.ContractAddress
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}
