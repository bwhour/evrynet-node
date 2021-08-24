// Copyright 2018 The go-ethereum Authors
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

// This file contains a miner stress test based on the Clique consensus engine.
package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"time"

	"github.com/pkg/errors"

	"github.com/Evrynetlabs/evrynet-node"
	"github.com/Evrynetlabs/evrynet-node/accounts/keystore"
	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/common/fdlimit"
	"github.com/Evrynetlabs/evrynet-node/common/hexutil"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/core"
	"github.com/Evrynetlabs/evrynet-node/core/state/staking"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/evr"
	"github.com/Evrynetlabs/evrynet-node/evr/downloader"
	"github.com/Evrynetlabs/evrynet-node/evrclient"
	"github.com/Evrynetlabs/evrynet-node/log"
	"github.com/Evrynetlabs/evrynet-node/miner"
	"github.com/Evrynetlabs/evrynet-node/node"
	"github.com/Evrynetlabs/evrynet-node/p2p"
	"github.com/Evrynetlabs/evrynet-node/p2p/enode"
	"github.com/Evrynetlabs/evrynet-node/params"
)

type TxMode int

const (
	NormalTxMode TxMode = iota
	SmartContractMode
	defaultGenesisFile = "./genesis_testnet.json"
	defaultConfigFile  = "./stress_config.json"
	dataDir            = "test_data"
	txsBatchSize       = 1024
	maxTxPoolSize      = 20480
)

func main() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
	_, _ = fdlimit.Raise(2048)
	var (
		err         error
		genesisFile = defaultGenesisFile
		configFile  = defaultConfigFile
	)
	if len(os.Args) == 3 {
		log.Info("overwrite default config")
		genesisFile = os.Args[1]
		configFile = os.Args[2]
	}

	cfg, enodes, faucets := parseTestConfig(configFile)
	genesis, err := parseGenesis(genesisFile)
	if err != nil {
		panic(err)
	}
	testNode, err := makeNode(genesis, enodes)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := testNode.Close(); err != nil {
			panic(err)
		}
	}()

	var (
		ethereum     *evr.Evrynet
		contractAddr *common.Address
	)
	if err := testNode.Service(&ethereum); err != nil {
		panic(err)
	}
	// wait until testNode is synced
	gasPrice := testNode.Server().ChainReader.Config().GasPrice
	nonces := waitForSyncingAndStableNonces(ethereum, faucets, ethereum.BlockChain().CurrentHeader().Number.Uint64())
	if TxMode(cfg.TxMode) == SmartContractMode {
		if contractAddr, err = prepareNewContract(cfg.RPCEndpoint, faucets[0], nonces[0], gasPrice); err != nil {
			panic(err)
		}
		nonces[0]++
	}

	go reportLoop(ethereum.BlockChain(), cfg.TxMode)
	// Start injecting transactions from the faucet like crazy
	for {
		var txs types.Transactions
		// Create a batch of transaction and inject into the pool
		// Note: if we add a single transaction one by one, the queue for broadcast txs might be full
		for i := 0; i < txsBatchSize; i++ {
			index := rand.Intn(len(faucets))
			tx, err := createTx(cfg.TxMode, gasPrice, faucets[index], nonces[index], contractAddr)
			if err != nil {
				panic(err)
			}
			nonces[index]++
			txs = append(txs, tx)
		}
		errs := ethereum.TxPool().AddLocals(txs)
		for _, err := range errs {
			if err != nil {
				panic(err)
			}
		}

		// Wait if we're too saturated
		rebroadcast := false
	waitLoop:
		for epoch := 0; ; epoch++ {
			pend, _ := ethereum.TxPool().Stats()
			switch {
			case pend < maxTxPoolSize:
				break waitLoop
			default:
				if !rebroadcast {
					forceBroadcastPendingTxs(ethereum)
					rebroadcast = true
				}
				log.Info("tx pool is full, sleeping", "pending", pend)
				time.Sleep(time.Second)
			}
		}
	}
}

//forceBroadcastPendingTxs get pending from
func forceBroadcastPendingTxs(ethereum *evr.Evrynet) {
	// force rebroadcast
	var txs types.Transactions
	pendings, err := ethereum.TxPool().Pending()
	if err != nil {
		panic(err)
	}
	for _, pendingTxs := range pendings {
		ethereum.TxPool().State()
		if len(pendingTxs) > txsBatchSize {
			txs = append(txs, pendingTxs[:txsBatchSize]...)
		} else {
			txs = append(txs, pendingTxs...)
		}
	}
	go func() {
		ethereum.GetPm().ForceBroadcastTxs(txs)
	}()
}

type stressConfig struct {
	EnodeStrings  []string `json:"enodes"`
	FaucetStrings []string `json:"faucets"`
	TxMode        TxMode   `json:"tx_mode"`
	RPCEndpoint   string   `json:"rpc_endpoint"`
}

func parseTestConfig(fileName string) (*stressConfig, []*enode.Node, []*ecdsa.PrivateKey) {
	var cfg stressConfig
	f, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}
	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		panic(err)
	}

	var (
		enodes  []*enode.Node
		faucets []*ecdsa.PrivateKey
	)
	for _, enodeS := range cfg.EnodeStrings {
		enodes = append(enodes, enode.MustParse(enodeS))
	}

	for _, faucetS := range cfg.FaucetStrings {
		faucetPriKey, err := crypto.HexToECDSA(faucetS)
		if err != nil {
			panic(err)
		}
		faucets = append(faucets, faucetPriKey)
	}
	return &cfg, enodes, faucets
}

// parseGenesis creates a genesis block from config file
func parseGenesis(fileName string) (*core.Genesis, error) {
	// Create a Clique network based off of the Rinkeby config
	// Read file genesis generated from pupeth
	genesisFile, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	config := &core.Genesis{}
	err = json.Unmarshal(genesisFile, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// makeNode creates a node from genesis config
func makeNode(genesis *core.Genesis, enodes []*enode.Node) (*node.Node, error) {
	// Define the basic configurations for the Evrynet node
	config := &node.Config{
		Name:    "geth",
		Version: params.Version,
		DataDir: dataDir,

		P2P: p2p.Config{
			ListenAddr:  "0.0.0.0:0",
			NoDiscovery: true,
			MaxPeers:    25,
		},
		NoUSB:    true,
		HTTPHost: "127.0.0.1", //add an rpc for debug
		HTTPPort: 22005,
		HTTPModules: []string{"admin", "db", "eth", "debug", "miner", "net", "shh", "txpool",
			"personal", "web3", "tendermint"},
	}
	// Start the node and configure a full Evrynet node on it
	stack, err := node.New(config)
	if err != nil {
		return nil, err
	}
	if err := stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		cfg := &evr.Config{
			Genesis:         genesis,
			NetworkId:       genesis.Config.ChainID.Uint64(),
			SyncMode:        downloader.FullSync,
			DatabaseCache:   256,
			DatabaseHandles: 256,
			TxPool: core.TxPoolConfig{
				Journal:      "transactions.rlp",
				Rejournal:    time.Hour,
				PriceLimit:   1,
				PriceBump:    10,
				AccountSlots: 16,
				GlobalSlots:  40960,
				AccountQueue: 64,
				GlobalQueue:  10240,
				Lifetime:     3 * time.Hour,
			},
			GPO: evr.DefaultConfig.GPO,
			Miner: miner.Config{
				GasFloor: genesis.GasLimit * 9 / 10,
				GasCeil:  genesis.GasLimit * 11 / 10,
				Recommit: time.Second,
			},
			Tendermint: tendermint.Config{
				IndexStateVariables: staking.DefaultConfig,
			},
		}

		fullNode, err := evr.New(ctx, cfg)
		// Init Tendermint ChainReader for p2p server to read validators set
		if fullNode != nil && fullNode.BlockChain() != nil && fullNode.BlockChain().Config().Tendermint != nil && stack.P2PServer.ChainReader == nil {
			stack.P2PServer.ChainReader = fullNode.BlockChain()
		}
		stack.P2PServerInitDone <- struct{}{}
		return fullNode, err
	}); err != nil {
		return nil, err
	}
	// Start the node and return if successful
	if err = stack.Start(); err != nil {
		return nil, err
	}

	for stack.Server().NodeInfo().Ports.Listener == 0 {
		time.Sleep(250 * time.Millisecond)
	}
	// Connect the testNode to dev chain
	for _, n := range enodes {
		stack.Server().AddPeer(n)
	}

	//Inject the signer key and start sealing with it
	store := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	nodePriKey, _ := crypto.GenerateKey()
	signer, err := store.ImportECDSA(nodePriKey, "")
	if err != nil {
		return nil, err
	}
	if err := store.Unlock(signer, ""); err != nil {
		return nil, err
	}
	return stack, nil
}

// waitForSyncingAndStableNonces wait util the node is syncing and the nonces of given addresses are not change, also returns stable nonces
func waitForSyncingAndStableNonces(ethereum *evr.Evrynet, faucets []*ecdsa.PrivateKey, initBlkNumber uint64) []uint64 {
	bc := ethereum.BlockChain()
	for !ethereum.Synced() || ethereum.BlockChain().CurrentHeader().Number.Uint64() == initBlkNumber {
		log.Warn("testNode is not synced, sleeping", "current_block", bc.CurrentHeader().Number)
		time.Sleep(3 * time.Second)
	}

	nonces := make([]uint64, len(faucets))
	// wait for nonce is not change
	for {
		for i, faucet := range faucets {
			addr := crypto.PubkeyToAddress(*(faucet.Public().(*ecdsa.PublicKey)))
			log.Info("faucet addr", "addr", addr)
			nonces[i] = ethereum.TxPool().State().GetNonce(addr)
		}
		time.Sleep(time.Second * 10)
		var diff = false
		for i, faucet := range faucets {
			addr := crypto.PubkeyToAddress(*(faucet.Public().(*ecdsa.PublicKey)))
			tmp := ethereum.TxPool().State().GetNonce(addr)
			if tmp != nonces[i] {
				diff = true
			}
		}
		if !diff {
			break
		}
	}
	return nonces
}

func prepareNewContract(rpcEndpoint string, acc *ecdsa.PrivateKey, nonce uint64, gasPrice *big.Int) (*common.Address, error) {
	log.Info("Creating Smart Contract ...")

	evrClient, err := evrclient.Dial(rpcEndpoint)
	if err != nil {
		return nil, err
	}

	// payload to create a smart contract
	payload := "0x608060405260d0806100126000396000f30060806040526004361060525763ffffffff7c01000000000000000000000000000000000000000000000000000000006000350416633fb5c1cb811460545780638381f58a14605d578063f2c9ecd8146081575b005b60526004356093565b348015606857600080fd5b50606f6098565b60408051918252519081900360200190f35b348015608c57600080fd5b50606f609e565b600055565b60005481565b600054905600a165627a7a723058209573e4f95d10c1e123e905d720655593ca5220830db660f0641f3175c1cdb86e0029"
	payLoadBytes, err := hexutil.Decode(payload)
	if err != nil {
		return nil, err
	}

	accAddr := crypto.PubkeyToAddress(acc.PublicKey)
	msg := evrynet.CallMsg{
		From:  accAddr,
		Value: common.Big0,
		Data:  payLoadBytes,
	}
	estGas, err := evrClient.EstimateGas(context.Background(), msg)
	if err != nil {
		return nil, err
	}

	tx := types.NewContractCreation(nonce, big.NewInt(0), estGas, gasPrice, payLoadBytes)
	tx, err = types.SignTx(tx, types.HomesteadSigner{}, acc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to sign Tx")
	}

	err = evrClient.SendTransaction(context.Background(), tx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create SC from %s", accAddr.String())
	}

	// Wait to get SC address
	for i := 0; i < 10; i++ {
		receipt, err := evrClient.TransactionReceipt(context.Background(), tx.Hash())
		if err == nil && receipt.Status == uint64(1) {
			log.Info("Creating Smart Contract successfully!")
			return &receipt.ContractAddress, nil
		}
		time.Sleep(1 * time.Second)
	}
	return nil, errors.New("Can not get SC address")
}

func createTx(txMode TxMode, gasPrice *big.Int, faucet *ecdsa.PrivateKey, nonces uint64, contractAddr *common.Address) (*types.Transaction, error) {
	switch txMode {
	case NormalTxMode:
		return types.SignTx(
			types.NewTransaction(nonces, crypto.PubkeyToAddress(faucet.PublicKey), new(big.Int),
				21000, gasPrice, nil),
			types.HomesteadSigner{},
			faucet)
	case SmartContractMode:
		return types.SignTx(
			types.NewTransaction(nonces, *contractAddr, new(big.Int),
				40000, gasPrice,
				[]byte("0x3fb5c1cb0000000000000000000000000000000000000000000000000000000000000002")),
			types.HomesteadSigner{},
			faucet)
	default:
		return nil, errors.Errorf("unexpected tx mode: %d", txMode)
	}
}

func reportLoop(bc *core.BlockChain, mode TxMode) {
	lastBlk := bc.CurrentHeader().Number.Uint64()
	numTxs := 0
	start := time.Now()
	preNumTxs := 0
	prevTime := time.Now()
	for {
		for currentBlk := bc.CurrentHeader().Number.Uint64(); currentBlk > lastBlk; lastBlk++ {
			numTxs += len(bc.GetBlockByNumber(lastBlk).Body().Transactions)
			log.Info("new_block", "txs", len(bc.GetBlockByNumber(lastBlk).Body().Transactions), "number", lastBlk)
		}
		log.Warn("num tx info", "tx_mode", mode, "txs", numTxs, "duration", time.Since(start),
			"avg_tps", float64(numTxs)/time.Since(start).Seconds(), "current_tps", float64(numTxs-preNumTxs)/time.Since(prevTime).Seconds(),
			"block", lastBlk)
		preNumTxs = numTxs
		prevTime = time.Now()
		time.Sleep(2 * time.Second)
	}
}
