package tests

import (
	"context"
	"math/big"
	"testing"

	"github.com/Evrynetlabs/evrynet-node/crypto"

	"github.com/stretchr/testify/assert"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/evrclient"
)

/* These tests are done on a chain with already setup account/ contracts.
To run these test, please deploy your own account/ contract and extract privatekey inorder to get the expected result
*/

// TestSendNormalTxWithFixedFee
func TestSendNormalTxWithFixedFee(t *testing.T) {
	const (
		normalAddress = "EaHAtNKwh5NMnVQCR1Tjs4HPypncDwAA8H"
		senderPK      = "62199ECEC394ED8B6BEB52924B8AF3AE41D1887D624A368A3305ED8894B99DCF"
		senderAddrStr = "EapmLgEVZtT1Um8QksnVxnv1dR8sb9wRiW"

		testBal1     = 1000000 //1e6
		testBal2     = 2000000 //2e6
		testGasLimit = 100000000
	)

	var (
		senderAddr, _ = common.EvryAddressStringToAddressCheck(senderAddrStr)
		normalAddr, _ = common.EvryAddressStringToAddressCheck(normalAddress)
		fixedGasPrice = big.NewInt(1000000000)
	)

	spk, err := crypto.HexToECDSA(senderPK)
	assert.NoError(t, err)
	signer := types.HomesteadSigner{}
	ethClient, err := evrclient.Dial("http://localhost:22001")
	assert.NoError(t, err)
	nonce, err := ethClient.PendingNonceAt(context.Background(), senderAddr)
	assert.NoError(t, err)

	//SuggestGasPrice will return fixedGasPrice
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, gasPrice, fixedGasPrice)

	//this transaction should be reject since its gas price is not the fixed gas price
	transaction := types.NewTransaction(nonce, normalAddr, big.NewInt(1000000), 1000000, big.NewInt(2000000), nil)
	transaction, err = types.SignTx(transaction, signer, spk)
	assert.NoError(t, err)
	assert.NotEqual(t, nil, ethClient.SendTransaction(context.Background(), transaction))

	//only transaction with gixedGasPrice/nil gas price is success
	transaction = types.NewTransaction(nonce, normalAddr, big.NewInt(1000000), 1000000, fixedGasPrice, nil)
	transaction, err = types.SignTx(transaction, signer, spk)
	assert.NoError(t, ethClient.SendTransaction(context.Background(), transaction))
}
