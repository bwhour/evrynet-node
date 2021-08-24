// Copyright 2017 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/core"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/log"
	"github.com/Evrynetlabs/evrynet-node/params"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

// makeGenesis creates a new genesis struct based on some user input.
func (w *wizard) makeGenesis() {
	// Construct a default genesis block
	genesis := &core.Genesis{
		Timestamp:  uint64(time.Now().Unix()),
		GasLimit:   4700000,
		Difficulty: big.NewInt(524288),
		Alloc:      make(core.GenesisAlloc),
		Config: &params.ChainConfig{
			HomesteadBlock:      big.NewInt(0),
			EIP150Block:         big.NewInt(0),
			EIP155Block:         big.NewInt(0),
			EIP158Block:         big.NewInt(0),
			ByzantiumBlock:      big.NewInt(0),
			ConstantinopleBlock: big.NewInt(0),
		},
	}
	// Figure out which consensus engine to choose
	fmt.Println()
	fmt.Println("Which consensus engine to use? (default = clique)")
	fmt.Println(" 1. Ethash - proof-of-work")
	fmt.Println(" 2. Clique - proof-of-authority")
	fmt.Println(" 3. Tendermint - practical-byzantine-fault-tolerance")

	choice := w.read()
	switch {
	case choice == "1":
		// In case of ethash, we're pretty much done
		genesis.Config.Ethash = new(params.EthashConfig)
		genesis.ExtraData = make([]byte, 32)

	case choice == "2":
		// In the case of clique, configure the consensus parameters
		genesis.Difficulty = big.NewInt(1)
		genesis.Config.Clique = &params.CliqueConfig{
			Period: 15,
			Epoch:  30000,
		}
		fmt.Println()
		fmt.Println("How many seconds should blocks take? (default = 15)")
		genesis.Config.Clique.Period = uint64(w.readDefaultInt(15))

		// We also need the initial list of signers
		fmt.Println()
		fmt.Println("Which accounts are allowed to seal? (mandatory at least one)")

		var signers []common.Address
		for {
			if address := w.readAddress(); address != nil {
				signers = append(signers, *address)
				continue
			}
			if len(signers) > 0 {
				break
			}
		}
		// Sort the signers and embed into the extra-data section
		for i := 0; i < len(signers); i++ {
			for j := i + 1; j < len(signers); j++ {
				if bytes.Compare(signers[i][:], signers[j][:]) > 0 {
					signers[i], signers[j] = signers[j], signers[i]
				}
			}
		}
		genesis.ExtraData = make([]byte, 32+len(signers)*common.AddressLength+65)
		for i, signer := range signers {
			copy(genesis.ExtraData[32+i*common.AddressLength:], signer[:])
		}
	case choice == "" || choice == "3":
		fmt.Println("What is policy to select proposer (default 0 - roundrobin)")
		policy := uint64(w.readDefaultInt(0))
		genesis.Config.Tendermint = &params.TendermintConfig{
			ProposerPolicy: policy,
		}

		// Query the tendermint block reward
		fmt.Println()
		fmt.Println("Specify your tendermint block reward if you want an explicit one (default = 5e+18)")
		genesis.Config.Tendermint.BlockReward = new(big.Int).Set(w.readDefaultBigInt(big.NewInt(5e+18)))

		// In the case of Tender-mint, configure the consensus parameters
		genesis.Difficulty = big.NewInt(1)

		// We also need the initial list of validators
		fmt.Println()
		fmt.Println("Which accounts are validators? (mandatory at least one)")

		var validators []common.Address
		for {
			if address := w.readAddress(); address != nil {
				validators = append(validators, *address)
				continue
			}
			if len(validators) > 0 {
				break
			}
		}
		tendermintExtra := types.TendermintExtra{}

		fmt.Println()
		fmt.Println("Do you want to use fixed validators? (default = no)")
		if w.readDefaultYesNo(false) {
			genesis.Config.Tendermint.FixedValidators = validators
		} else if err := w.configStakingSC(genesis, validators); err != nil {
			log.Error("Failed to config staking SC", "error", err)
			return
		}

		// RLP encode validator's address to bytes
		valSetData, err := rlp.EncodeToBytes(validators)
		if err != nil {
			log.Error("rlp encode got error", "error", err)
			return
		}
		tendermintExtra.ValidatorAdds = valSetData
		extraData, err := rlp.EncodeToBytes(&tendermintExtra)
		if err != nil {
			log.Error("rlp encode got error", "error", err)
			return
		}
		tendermintExtraVanity := bytes.Repeat([]byte{0x00}, types.TendermintExtraVanity)
		genesis.ExtraData = append(tendermintExtraVanity, extraData...)
	default:
		log.Crit("Invalid consensus engine choice", "choice", choice)
	}

	//Create flood accounts
	fmt.Println()
	fmt.Println("Do you want to create accounts for tx flood? (default = no)")
	if w.readDefaultYesNo(false) {
		fmt.Println()
		fmt.Println("How many accounts do you want? (default = 1000)")
		numberAcc := w.readDefaultInt(1000)

		fmt.Println()
		fmt.Println("What is the seed? (default = testnet)")
		seed := w.readDefaultString("testnet")
		accs, err := generateAccounts(numberAcc, seed)
		if err != nil {
			log.Error("fail to generate new account", "Error:", err)
			return
		}

		for _, acc := range accs {
			if _, ok := genesis.Alloc[acc.Address]; ok {
				fmt.Printf("- Address %s already existed => Ignore\n",
					common.AddressToEvryAddressString(acc.Address))
				continue
			}
			genesis.Alloc[acc.Address] = core.GenesisAccount{
				Balance: new(big.Int).Lsh(big.NewInt(1), 256-7), // 2^256 / 128 (allow many pre-funds without balance overflows)
			}
		}
	} else {
		// Consensus all set, just ask for initial funds and go
		fmt.Println()
		fmt.Println("Which accounts should be pre-funded? (advisable at least one)")
		for {
			// Read the address of the account to fund
			if address := w.readAddress(); address != nil {
				if _, ok := genesis.Alloc[*address]; ok {
					fmt.Printf("- Address %s already existed. Please input another one.\n", address.String())
					continue
				}
				genesis.Alloc[*address] = core.GenesisAccount{
					Balance: new(big.Int).Lsh(big.NewInt(1), 256-7), // 2^256 / 128 (allow many pre-funds without balance overflows)
				}
				continue
			}
			break
		}
		fmt.Println()
		fmt.Println("Should the  precompile-addresses (0x1 .. 0xff) be pre-funded with 1 wei? (advisable no)")
		if w.readDefaultYesNo(false) {
			// Add a batch of precompile balances to avoid them getting deleted
			for i := int64(0); i < 256; i++ {
				addr := common.BigToAddress(big.NewInt(i))
				if _, ok := genesis.Alloc[addr]; ok {
					fmt.Printf("- Account %d (address: %s) already existed => Ignore\n", i, addr.String())
					continue
				}
				genesis.Alloc[addr] = core.GenesisAccount{Balance: big.NewInt(1)}
			}
		}
	}

	// Query the user for some custom extras
	fmt.Println()
	fmt.Println("Specify your chain/network ID if you want an explicit one (default = random)")
	genesis.Config.ChainID = new(big.Int).SetUint64(uint64(w.readDefaultInt(rand.Intn(65536))))

	// Query the gas price
	fmt.Println()
	fmt.Println("Specify your network gas price if you want an explicit one (default = 1 Gwei)")
	genesis.Config.GasPrice = new(big.Int).SetUint64(uint64(w.readDefaultInt(params.GasPriceConfig)))

	// All done, store the genesis and flush to disk
	log.Info("Configured new genesis block")

	w.conf.Genesis = genesis
	w.conf.flush()
}

// importGenesis imports a Geth genesis spec into puppeth.
func (w *wizard) importGenesis() {
	// Request the genesis JSON spec URL from the user
	fmt.Println()
	fmt.Println("Where's the genesis file? (local file or http/https url)")
	url := w.readURL()

	// Convert the various allowed URLs to a reader stream
	var reader io.Reader

	switch url.Scheme {
	case "http", "https":
		// Remote web URL, retrieve it via an HTTP client
		res, err := http.Get(url.String())
		if err != nil {
			log.Error("Failed to retrieve remote genesis", "err", err)
			return
		}
		defer res.Body.Close()
		reader = res.Body

	case "":
		// Schemaless URL, interpret as a local file
		file, err := os.Open(url.String())
		if err != nil {
			log.Error("Failed to open local genesis", "err", err)
			return
		}
		defer file.Close()
		reader = file

	default:
		log.Error("Unsupported genesis URL scheme", "scheme", url.Scheme)
		return
	}
	// Parse the genesis file and inject it successful
	var genesis core.Genesis
	if err := json.NewDecoder(reader).Decode(&genesis); err != nil {
		log.Error("Invalid genesis spec: %v", err)
		return
	}
	log.Info("Imported genesis block")

	w.conf.Genesis = &genesis
	w.conf.flush()
}

// manageGenesis permits the modification of chain configuration parameters in
// a genesis config and the export of the entire genesis spec.
func (w *wizard) manageGenesis() {
	// Figure out whether to modify or export the genesis
	fmt.Println()
	fmt.Println(" 1. Modify existing fork rules")
	fmt.Println(" 2. Export genesis configurations")
	fmt.Println(" 3. Remove genesis configuration")

	choice := w.read()
	switch choice {
	case "1":
		// Fork rule updating requested, iterate over each fork
		fmt.Println()
		fmt.Printf("Which block should Homestead come into effect? (default = %v)\n", w.conf.Genesis.Config.HomesteadBlock)
		w.conf.Genesis.Config.HomesteadBlock = w.readDefaultBigInt(w.conf.Genesis.Config.HomesteadBlock)

		fmt.Println()
		fmt.Printf("Which block should EIP150 (Tangerine Whistle) come into effect? (default = %v)\n", w.conf.Genesis.Config.EIP150Block)
		w.conf.Genesis.Config.EIP150Block = w.readDefaultBigInt(w.conf.Genesis.Config.EIP150Block)

		fmt.Println()
		fmt.Printf("Which block should EIP155 (Spurious Dragon) come into effect? (default = %v)\n", w.conf.Genesis.Config.EIP155Block)
		w.conf.Genesis.Config.EIP155Block = w.readDefaultBigInt(w.conf.Genesis.Config.EIP155Block)

		fmt.Println()
		fmt.Printf("Which block should EIP158/161 (also Spurious Dragon) come into effect? (default = %v)\n", w.conf.Genesis.Config.EIP158Block)
		w.conf.Genesis.Config.EIP158Block = w.readDefaultBigInt(w.conf.Genesis.Config.EIP158Block)

		fmt.Println()
		fmt.Printf("Which block should Byzantium come into effect? (default = %v)\n", w.conf.Genesis.Config.ByzantiumBlock)
		w.conf.Genesis.Config.ByzantiumBlock = w.readDefaultBigInt(w.conf.Genesis.Config.ByzantiumBlock)

		fmt.Println()
		fmt.Printf("Which block should Constantinople come into effect? (default = %v)\n", w.conf.Genesis.Config.ConstantinopleBlock)
		w.conf.Genesis.Config.ConstantinopleBlock = w.readDefaultBigInt(w.conf.Genesis.Config.ConstantinopleBlock)
		if w.conf.Genesis.Config.PetersburgBlock == nil {
			w.conf.Genesis.Config.PetersburgBlock = w.conf.Genesis.Config.ConstantinopleBlock
		}
		fmt.Println()
		fmt.Printf("Which block should Constantinople-Fix (remove EIP-1283) come into effect? (default = %v)\n", w.conf.Genesis.Config.PetersburgBlock)
		w.conf.Genesis.Config.PetersburgBlock = w.readDefaultBigInt(w.conf.Genesis.Config.PetersburgBlock)

		out, _ := json.MarshalIndent(w.conf.Genesis.Config, "", "  ")
		fmt.Printf("Chain configuration updated:\n\n%s\n", out)

		w.conf.flush()

	case "2":
		// Save whatever genesis configuration we currently have
		fmt.Println()
		fmt.Printf("Which folder to save the genesis specs into? (default = current)\n")
		fmt.Printf("  Will create %s.json, %s-aleth.json, %s-harmony.json, %s-parity.json\n", w.network, w.network, w.network, w.network)

		folder := w.readDefaultString(".")
		if err := os.MkdirAll(folder, 0755); err != nil {
			log.Error("Failed to create spec folder", "folder", folder, "err", err)
			return
		}
		out, _ := json.MarshalIndent(w.conf.Genesis, "", "  ")

		// Export the native genesis spec used by puppeth and Geth
		gethJson := filepath.Join(folder, fmt.Sprintf("%s.json", w.network))
		if err := ioutil.WriteFile((gethJson), out, 0644); err != nil {
			log.Error("Failed to save genesis file", "err", err)
			return
		}
		log.Info("Saved native genesis chain spec", "path", gethJson)

		// Export the genesis spec used by Aleth (formerly C++ Evrynet)
		if spec, err := newAlethGenesisSpec(w.network, w.conf.Genesis); err != nil {
			log.Error("Failed to create Aleth chain spec", "err", err)
		} else {
			saveGenesis(folder, w.network, "aleth", spec)
		}
		// Export the genesis spec used by Parity
		if spec, err := newParityChainSpec(w.network, w.conf.Genesis, []string{}); err != nil {
			log.Error("Failed to create Parity chain spec", "err", err)
		} else {
			saveGenesis(folder, w.network, "parity", spec)
		}
		// Export the genesis spec used by Harmony (formerly EvrynetJ
		saveGenesis(folder, w.network, "harmony", w.conf.Genesis)

	case "3":
		// Make sure we don't have any services running
		if len(w.conf.servers()) > 0 {
			log.Error("Genesis reset requires all services and servers torn down")
			return
		}
		log.Info("Genesis block destroyed")

		w.conf.Genesis = nil
		w.conf.flush()
	default:
		log.Error("That's not something I can do")
		return
	}
}

// saveGenesis JSON encodes an arbitrary genesis spec into a pre-defined file.
func saveGenesis(folder, network, client string, spec interface{}) {
	path := filepath.Join(folder, fmt.Sprintf("%s-%s.json", network, client))

	out, _ := json.Marshal(spec)
	if err := ioutil.WriteFile(path, out, 0644); err != nil {
		log.Error("Failed to save genesis file", "client", client, "err", err)
		return
	}
	log.Info("Saved genesis chain spec", "client", client, "path", path)
}

type account struct {
	PriKey  *ecdsa.PrivateKey
	PubKey  *ecdsa.PublicKey
	Address common.Address
}

func (a *account) PrivateKeyStr() string {
	return hex.EncodeToString(crypto.FromECDSA(a.PriKey))
}

func (a *account) PublicKeyStr() string {
	return hex.EncodeToString(crypto.FromECDSAPub(a.PubKey))
}

// generateAccounts creates a list of accounts from seed
func generateAccounts(num int, seed string) ([]*account, error) {
	var accs []*account
	for i := 0; i < num; i++ {
		seedBytes := []byte(seed + strconv.Itoa(i))
		seedBytes = append(seedBytes, bytes.Repeat([]byte{0x00}, ed25519.SeedSize-len(seedBytes))...)

		key := ed25519.NewKeyFromSeed(seedBytes)[32:]
		privateKey, err := crypto.ToECDSA(key[:])
		if err != nil {
			return nil, err
		}

		publicKeyECDSA, ok := privateKey.Public().(*ecdsa.PublicKey)
		if !ok {
			return nil, errors.New("cannot assert type: publicKey is not of type *ecdsa.PublicKey")
		}
		accs = append(accs,
			&account{
				PriKey:  privateKey,
				PubKey:  publicKeyECDSA,
				Address: crypto.PubkeyToAddress(privateKey.PublicKey),
			})
	}
	return accs, nil
}
