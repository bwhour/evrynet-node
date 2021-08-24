package test

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	ethereum "github.com/Evrynetlabs/evrynet-node"
	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/common/hexutil"
	"github.com/Evrynetlabs/evrynet-node/evrclient"
)

//TestSendTxCreateContractWithProviderAndOwner test send tx to create a contract with provider attached.
//	require here the sender account wallet have to unlocked
//You can use sample test data through the cURL to JSON RPC call below instead of using test apiclient
//	curl -H "Content-Type: application/json" -X POST --data '{"jsonrpc":"2.0","method":"eth_sendTransaction","params":[{"from":"0x7b9160548b27a5c14f8819eda719f955437862d5", "provider":"0x8359d8C955DAef81e171C13659bA3Fb0dDa144b4", "owner":"0x7b9160548b27a5c14f8819eda719f955437862d5", "gas":"0xF4240", "data":"0x6060604052341561000f57600080fd5b6102e38061001e6000396000f30060606040526004361061004c576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff168063368b877214610051578063ce6d41de146100ae575b600080fd5b341561005c57600080fd5b6100ac600480803590602001908201803590602001908080601f0160208091040260200160405190810160405280939291908181526020018383808284378201915050505050509190505061013c565b005b34156100b957600080fd5b6100c1610156565b6040518080602001828103825283818151815260200191508051906020019080838360005b838110156101015780820151818401526020810190506100e6565b50505050905090810190601f16801561012e5780820380516001836020036101000a031916815260200191505b509250505060405180910390f35b80600090805190602001906101529291906101fe565b5050565b61015e61027e565b60008054600181600116156101000203166002900480601f0160208091040260200160405190810160405280929190818152602001828054600181600116156101000203166002900480156101f45780601f106101c9576101008083540402835291602001916101f4565b820191906000526020600020905b8154815290600101906020018083116101d757829003601f168201915b5050505050905090565b828054600181600116156101000203166002900490600052602060002090601f016020900481019282601f1061023f57805160ff191683800117855561026d565b8280016001018555821561026d579182015b8281111561026c578251825591602001919060010190610251565b5b50905061027a9190610292565b5090565b602060405190810160405280600081525090565b6102b491905b808211156102b0576000816000905550600101610298565b5090565b905600a165627a7a723058208a6eba9352e080994bc6a1041d71eff20de6686dbafb2341e23c07d938e706d60029"}],"id":1}' http://localhost:8545
func TestSendTxCreateContractWithProviderAndOwner(t *testing.T) {
	sender, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	providerAddr, _ := common.EvryAddressStringToAddressCheck(providerAddrStr)
	payLoadBytes, err := hexutil.Decode(payload)
	assert.NoError(t, err)
	data := hexutil.Bytes(payLoadBytes)

	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)
	gPrice := hexutil.Big(*big.NewInt(gasPrice.Int64()))
	value := hexutil.Big(*big.NewInt(0))
	gas := hexutil.Uint64(testGasLimit)
	args := ethereum.SendTxArgs{
		From:     sender,
		To:       nil,
		GasPrice: &gPrice,
		Gas:      &gas,
		Value:    &value,
		Input:    nil,
		Data:     &data,
		Provider: &providerAddr,
		Owner:    &sender,
	}

	emptyHash := common.Hash{}
	hash, err := ethClient.SendTx(context.Background(), args)
	assert.NoError(t, err)
	assert.NotEqual(t, emptyHash, hash)
	if hash != emptyHash {
		tx, _, err := ethClient.TransactionByHash(context.Background(), hash)
		assert.Equal(t, args.Provider, tx.Provider())
		assert.Equal(t, args.Owner, tx.Owner())
		assert.NoError(t, err)
		assertTransactionSuccess(t, ethClient, hash, false, sender)
	}
}

//TestSendTxCreateContractNormal test send tx to create a contract without a provider attached.
//	require here the sender account wallet have to unlocked
//You can use sample test data through the cURL to JSON RPC call below instead of using test apiclient
//	curl -H "Content-Type: application/json" -X POST --data '{"jsonrpc":"2.0","method":"eth_sendTransaction","params":[{"from":"0x608fe17b82cdc38bcda9aba3bf1e90d61b412248", "gas":"0xF4240", "data":"0x608060405260d0806100126000396000f30060806040526004361060525763ffffffff7c01000000000000000000000000000000000000000000000000000000006000350416633fb5c1cb811460545780638381f58a14605d578063f2c9ecd8146081575b005b60526004356093565b348015606857600080fd5b50606f6098565b60408051918252519081900360200190f35b348015608c57600080fd5b50606f609e565b600055565b60005481565b600054905600a165627a7a723058209573e4f95d10c1e123e905d720655593ca5220830db660f0641f3175c1cdb86e0029"}],"id":1}' http://localhost:8545
func TestSendTxCreateContractNormal(t *testing.T) {
	sender, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	payLoadBytes, err := hexutil.Decode(payload)
	assert.NoError(t, err)
	data := hexutil.Bytes(payLoadBytes)

	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	gPrice := hexutil.Big(*big.NewInt(testGasPrice))
	gas := hexutil.Uint64(testGasLimit)
	value := hexutil.Big(*big.NewInt(0))
	args := ethereum.SendTxArgs{
		From: sender,
		//To:       &sender,
		GasPrice: &gPrice,
		Gas:      &gas,
		Value:    &value,
		Input:    nil,
		Data:     &data,
	}

	emptyHash := common.Hash{}
	hash, err := ethClient.SendTx(context.Background(), args)
	assert.NoError(t, err)
	assert.NotEqual(t, emptyHash, hash)
	assertTransactionSuccess(t, ethClient, hash, true, sender)
}

//TestSendTxNormal test send normal tx without a provider and not create a contract.
// require here the sender account wallet have to unlocked
//you can view sample test data via cURL call to JSON RPC below instead of go test apiclient
//curl -H "Content-Type: application/json" -X POST --data '{"jsonrpc":"2.0","method":"eth_sendTransaction","params":[{"from":"0x7b9160548b27a5c14f8819eda719f955437862d5", "to":"0x7b9160548b27a5c14f8819eda719f955437862d5", "gas":"0xF4240"}],"id":1}' http://localhost:8545
func TestSendTxNormal(t *testing.T) {
	sender, _ := common.EvryAddressStringToAddressCheck(senderAddrStr)
	data := hexutil.Bytes(payload)

	ethClient, err := evrclient.Dial(ethRPCEndpoint)
	assert.NoError(t, err)
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	assert.NoError(t, err)
	gPrice := hexutil.Big(*big.NewInt(gasPrice.Int64()))
	gas := hexutil.Uint64(1000000)
	value := hexutil.Big(*big.NewInt(0))
	args := ethereum.SendTxArgs{
		From:     sender,
		To:       &sender,
		GasPrice: &gPrice,
		Gas:      &gas,
		Value:    &value,
		Input:    nil,
		Data:     &data,
	}

	emptyHash := common.Hash{}
	hash, err := ethClient.SendTx(context.Background(), args)
	assert.NoError(t, err)
	assert.NotEqual(t, emptyHash, hash)
	if hash != emptyHash {
		assertTransactionSuccess(t, ethClient, hash, false, sender)
	}
}
