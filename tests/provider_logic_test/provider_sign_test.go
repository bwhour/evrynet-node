package test

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/evrclient"
)

/* These tests are done on a chain with already setup account/ contracts.
To run these test, please deploy your own account/ contract and extract privatekey inorder to get the expected result
Adjust these params to match deployment on local machine:
*/

/*
	Test Send ETH to a normal address
		- No provider signature is required
*/
func TestSendToNormalAddress(t *testing.T) {
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	normalAddr, _ := common.EvryAddressStringToAddressCheck(normalAddress)
	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)

	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	transaction := types.NewTransaction(nonce, normalAddr, big.NewInt(testAmountSend), testGasLimit, gasPrice, nil)
	transaction, err = types.SignTx(transaction, signer, spk)
	require.NoError(t, ethClient.SendTransaction(context.Background(), transaction))
	assertTransactionSuccess(t, ethClient, transaction.Hash(), false, senderAddr)
}

/*
	Test send to a normal address with provider's signature
		- Expect to get error with redundant provider's signature
*/
func TestSendToNormalAddressWithProviderSignature(t *testing.T) {
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	normalAddr, _ := common.EvryAddressStringToAddressCheck(normalAddress)
	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)

	ppk, err := crypto.HexToECDSA(providerPK)
	assert.NoError(t, err)
	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	transaction := types.NewTransaction(nonce, normalAddr, big.NewInt(testAmountSend), testGasLimit, gasPrice, nil)
	transaction, err = types.SignTx(transaction, signer, spk)
	assert.NoError(t, err)
	transaction, err = types.ProviderSignTx(transaction, signer, ppk)
	assert.NoError(t, err)
	require.Error(t, ethClient.SendTransaction(context.Background(), transaction))
}

/*
	Test Send ETH to a Smart Contract without provider's signature
		- Provider's signature is not required
*/
func TestSendToNonEnterpriseSmartContractWithoutProviderSignature(t *testing.T) {
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	contractAddr, _ := common.EvryAddressStringToAddressCheck(contractAddrStrWithoutProvider)
	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)

	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	transaction := types.NewTransaction(nonce, contractAddr, big.NewInt(testAmountSend), testGasLimit, gasPrice, nil)
	// return newTransaction(nonce, &to, amount, gasLimit, gasPrice, data)
	transaction, err = types.SignTx(transaction, signer, spk)
	require.NoError(t, ethClient.SendTransaction(context.Background(), transaction))
	assertTransactionSuccess(t, ethClient, transaction.Hash(), false, senderAddr)
}

/*
	Test send ETH to a Non-enterprise Smart Contract with provider's signature
		- Expect to get error as provider's signature is redundant
*/
func TestSendToNonEnterpriseSmartContractWithProviderSignature(t *testing.T) {
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	contractAddr, _ := common.EvryAddressStringToAddressCheck(contractAddrStrWithoutProvider)
	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)
	ppk, err := crypto.HexToECDSA(providerPK)
	assert.NoError(t, err)

	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	transaction := types.NewTransaction(nonce, contractAddr, big.NewInt(testAmountSend), testGasLimit, gasPrice, nil)
	transaction, err = types.SignTx(transaction, signer, spk)
	assert.NoError(t, err)
	transaction, err = types.ProviderSignTx(transaction, signer, ppk)
	assert.NoError(t, err)
	require.Error(t, ethClient.SendTransaction(context.Background(), transaction))
}

/*
	Test interact with Non-Enterprise Smart Contract
		- Update value inside Smart Contract and expect to get no error (skip provider check)
	Note: Please change data to your own function data
*/
func TestInteractWithNonEnterpriseSmartContractWithoutProviderSignature(t *testing.T) {
	//This should be a contract with provider address
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	contractAddr, _ := common.EvryAddressStringToAddressCheck(contractAddrStrWithoutProvider)
	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)

	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	// data to interact with a function of this contract
	dataBytes := []byte("0x3fb5c1cb0000000000000000000000000000000000000000000000000000000000000002")
	transaction := types.NewTransaction(nonce, contractAddr, big.NewInt(testAmountSend), testGasLimit, gasPrice, dataBytes)
	transaction, err = types.SignTx(transaction, signer, spk)
	require.NoError(t, ethClient.SendTransaction(context.Background(), transaction))
	assertTransactionSuccess(t, ethClient, transaction.Hash(), false, senderAddr)
}

/*
	Test Send ETH to an Enterprise Smart Contract with invalid provider's signature
*/
func TestSendToEnterPriseSmartContractWithInvalidProviderSignature(t *testing.T) {
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	contractAddr, _ := common.EvryAddressStringToAddressCheck(contractAddrStrWithProvider)
	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)

	ppk, err := crypto.HexToECDSA(invadlidProviderPK)
	assert.NoError(t, err)

	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	transaction := types.NewTransaction(nonce, contractAddr, big.NewInt(testAmountSend), testGasLimit, gasPrice, nil)
	transaction, err = types.SignTx(transaction, signer, spk)
	assert.NoError(t, err)
	transaction, err = types.ProviderSignTx(transaction, signer, ppk)
	assert.NoError(t, err)

	require.Error(t, ethClient.SendTransaction(context.Background(), transaction))
}

/*
	Test Send ETH to an enterprise Smart Contract with valid provider's signature
*/
func TestSendToEnterPriseSmartContractWithValidProviderSignature(t *testing.T) {
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	contractAddr := prepareNewContract(true)
	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)

	ppk, err := crypto.HexToECDSA(providerPK)
	assert.NoError(t, err)

	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	transaction := types.NewTransaction(nonce, *contractAddr, big.NewInt(testAmountSend), testGasLimit, gasPrice, nil)
	transaction, err = types.SignTx(transaction, signer, spk)
	assert.NoError(t, err)
	transaction, err = types.ProviderSignTx(transaction, signer, ppk)
	assert.NoError(t, err)

	require.NoError(t, ethClient.SendTransaction(context.Background(), transaction))
	providerAddr, _ := common.EvryAddressStringToAddressCheck(providerAddrStr)
	assertTransactionSuccess(t, ethClient, transaction.Hash(), false, providerAddr)
}

/*
	Test interact with Enterprise Smart Contract
		- Update value inside Smart Contract and expect to get error with invalid provider signature
	Note: Please change data to your own function data
*/
func TestInteractToEnterpriseSmartContractWithInvalidProviderSignature(t *testing.T) {
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	contractAddr := prepareNewContract(true)
	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)

	ppk, err := crypto.HexToECDSA(invadlidProviderPK)
	assert.NoError(t, err)

	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	// data to interact with a function of this contract
	dataBytes := []byte("0x3fb5c1cb0000000000000000000000000000000000000000000000000000000000000002")
	transaction := types.NewTransaction(nonce, *contractAddr, big.NewInt(testAmountSend), testGasLimit, gasPrice, dataBytes)
	transaction, err = types.SignTx(transaction, signer, spk)
	assert.NoError(t, err)
	transaction, err = types.ProviderSignTx(transaction, signer, ppk)
	assert.NoError(t, err)

	require.Error(t, ethClient.SendTransaction(context.Background(), transaction))
}

/*
	Test interact with Enterprise Smart Contract
		- Update value inside Smart Contract and expect to get error with invalid provider signature
	Note: Please change data to your own function data
*/
func TestInteractToEnterpriseSmartContractWithoutProviderSignature(t *testing.T) {
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	contractAddr := prepareNewContract(true)
	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)

	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	// data to interact with a function of this contract
	dataBytes := []byte("0x3fb5c1cb0000000000000000000000000000000000000000000000000000000000000002")
	transaction := types.NewTransaction(nonce, *contractAddr, big.NewInt(testAmountSend), testGasLimit, gasPrice, dataBytes)
	transaction, err = types.SignTx(transaction, signer, spk)
	assert.NoError(t, err)

	require.Error(t, ethClient.SendTransaction(context.Background(), transaction))
}

/*
	Test interact with Enterprise Smart Contract
		- Update value inside Smart Contract and expect to successfully update data with valid provider signature
	Note: Please change data to your own function data
*/
func TestInteractToEnterpriseSmartContractWithValidProviderSignature(t *testing.T) {
	senderAddr, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	contractAddr := prepareNewContract(true)
	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)

	ppk, err := crypto.HexToECDSA(providerPK)
	assert.NoError(t, err)

	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)

	// data to interact with a function of this contract
	dataBytes := []byte("0x3fb5c1cb0000000000000000000000000000000000000000000000000000000000000002")
	transaction := types.NewTransaction(nonce, *contractAddr, big.NewInt(testAmountSend), testGasLimit, gasPrice, dataBytes)
	transaction, err = types.SignTx(transaction, signer, spk)
	assert.NoError(t, err)
	transaction, err = types.ProviderSignTx(transaction, signer, ppk)
	assert.NoError(t, err)

	require.NoError(t, ethClient.SendTransaction(context.Background(), transaction))
	providerAddr, _ := common.EvryAddressStringToAddressCheck(providerAddrStr)
	assertTransactionSuccess(t, ethClient, transaction.Hash(), false, providerAddr)
}
