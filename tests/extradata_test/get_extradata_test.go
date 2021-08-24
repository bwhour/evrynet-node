package test

import (
	"context"
	"log"
	"math/big"
	"testing"
	"time"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/evrclient"
	"github.com/stretchr/testify/assert"
)

// TestGetExtraData run unit test for the GetBlockSignerByNumber and GetBlockSignerByHash
// expected get address of proposer and commit signers
// you can run via curl with
// curl -H "Content-Type: application/json" -X POST --data '{"jsonrpc":"2.0","method":"eth_getBlockSignerByNumber","params":["0x5"],"id":1}' http://localhost:8545
// curl -H "Content-Type: application/json" -X POST --data '{"jsonrpc":"2.0","method":"eth_getBlockSignerByHash","params":["0x4cddd578050781138591684e16674e7056ce89fb2706cf72b9f946e7e168d73b"],"id":1}' http://localhost:8545
func TestGetExtraData(t *testing.T) {
	ethClient, err := evrclient.Dial("http://localhost:8454")
	assert.NoError(t, err)
	fakeBlockNumber := big.NewInt(5)

	trie := 10
	for {
		if trie <= 0 {
			t.Error("You have to run a node and start miner before")
			break
		}
		block, err := ethClient.BlockByNumber(context.Background(), fakeBlockNumber)
		if err == nil {
			extra, err := ethClient.GetBlockSignerByNumber(context.Background(), block.Number())
			assert.NoError(t, err)
			assert.NotNil(t, extra)
			assert.NotNil(t, extra.BlockProposer)
			assert.NotEqual(t, common.Address{}, extra.BlockProposer)

			extra, err = ethClient.GetBlockSignerByHash(context.Background(), block.Hash())
			assert.NoError(t, err)
			assert.NotNil(t, extra)
			assert.NotNil(t, extra.BlockProposer)
			assert.NotEqual(t, common.Address{}, extra.BlockProposer)
			log.Printf("proposer is %s", extra.BlockProposer.String())
			for i, signer := range extra.CommitSigners {
				log.Printf("index %d signer %s", i, signer.String())
			}
			break
		}
		time.Sleep(2 * time.Second)
		trie--
		log.Printf("failed to get block, attempt left: %d, error: %s", trie, err)
	}

}
