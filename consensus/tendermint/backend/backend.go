package backend

import (
	"crypto/ecdsa"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	queue "github.com/enriquebris/goconcurrentqueue"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/backend/fixed_valset_info"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/backend/staking"
	tendermintCore "github.com/Evrynetlabs/evrynet-node/consensus/tendermint/core"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/crypto"
	"github.com/Evrynetlabs/evrynet-node/event"
	"github.com/Evrynetlabs/evrynet-node/evrdb"
	"github.com/Evrynetlabs/evrynet-node/log"
)

const (
	fetcherID         = "tendermint"
	maxNumberMessages = 64 * 128 * 6 // 64 node * 128 round * 6 messages per round. These number are made higher than expected for safety.
	maxTrigger        = 1000         // maximum of trigger signal that dequeuing op will store.

	maxBroadcastSleepTime        = time.Minute * 5
	initialBroadcastSleepTime    = time.Millisecond * 100
	broadcastSleepTimeIncreament = time.Millisecond * 100
	inMemoryValset               = 10
	inMemoryProposer             = 100
)

var (
	//ErrNoBroadcaster is return when trying to access backend.Broadcaster without SetBroadcaster first
	ErrNoBroadcaster = errors.New("no broadcaster is set")
)

//Option return an optional function for backend's initial behaviour
type Option func(b *Backend) error

// New creates an backend for Istanbul core engine.
// The p2p communication, i.e, broadcaster is set separately by calling backend.SetBroadcaster
func New(config *tendermint.Config, privateKey *ecdsa.PrivateKey, opts ...Option) consensus.Tendermint {
	valSetCache, _ := lru.NewARC(inMemoryValset)
	proposerCache, _ := lru.NewARC(inMemoryProposer)
	be := &Backend{
		config:                     config,
		tendermintEventMux:         new(event.TypeMux),
		privateKey:                 privateKey,
		address:                    crypto.PubkeyToAddress(privateKey.PublicKey),
		commitChs:                  newCommitChannels(),
		mutex:                      &sync.RWMutex{},
		storingMsgs:                queue.NewFIFO(),
		dequeueMsgTriggering:       make(chan struct{}, maxTrigger),
		closingBackgroundThreadsCh: make(chan struct{}),
		controlChan:                make(chan struct{}),
		computedValSetCache:        valSetCache,
		blockProposerCache:         proposerCache,
	}

	if config.FixedValidators != nil && len(config.FixedValidators) > 0 {
		be.valSetInfo = fixed_valset_info.NewFixedValidatorSetInfo(config.FixedValidators)
	} else {
		be.valSetInfo = staking.NewStakingValidatorInfo(config.Epoch, config.ProposerPolicy)
		if config.StakingSCAddress == nil {
			panic("nil staking address")
		}
		be.stakingContractAddr = *config.StakingSCAddress
	}
	be.core = tendermintCore.New(be, config)

	for _, opt := range opts {
		if err := opt(be); err != nil {
			log.Error("error at initialization of backend", err)
		}
	}

	go be.dequeueMsgLoop()
	return be
}

// SetBroadcaster implements consensus.Handler.SetBroadcaster
func (sb *Backend) SetBroadcaster(broadcaster consensus.Broadcaster) {
	sb.broadcaster = broadcaster
}

// ----------------------------------------------------------------------------
type Backend struct {
	config             *tendermint.Config
	tendermintEventMux *event.TypeMux
	privateKey         *ecdsa.PrivateKey
	core               tendermintCore.Engine
	db                 evrdb.Database
	broadcaster        consensus.Broadcaster
	address            common.Address

	//once voting finish, the block will be send for commit here
	//it is a map of blocknumber- channels with mutex
	commitChs *commitChannels

	coreStarted                bool
	mutex                      *sync.RWMutex
	chain                      consensus.FullChainReader
	controlChan                chan struct{}
	closingBackgroundThreadsCh chan struct{}

	//storingMsgs is used to store msg to handler when core stopped
	storingMsgs          *queue.FIFO
	dequeueMsgTriggering chan struct{}

	currentBlock func() *types.Block
	//verifyAndSubmitBlock to send the proposal block to miner
	verifyAndSubmitBlock func(*types.Block) error

	valSetInfo          ValidatorSetInfo
	stakingContractAddr common.Address // stakingContractAddr stores the address of staking smart-contract
	computedValSetCache *lru.ARCCache  // computedValSetCache stores the valset is computed from stateDB

	blockProposerCache *lru.ARCCache // blockProposerCache stores the address of proposal block
}

// EventMux implements tendermint.Backend.EventMux
func (sb *Backend) EventMux() *event.TypeMux {
	return sb.tendermintEventMux
}

// Sign implements tendermint.Backend.Sign
func (sb *Backend) Sign(data []byte) ([]byte, error) {
	hashData := crypto.Keccak256(data)
	return crypto.Sign(hashData, sb.privateKey)
}

// Address implements tendermint.Backend.Address
func (sb *Backend) Address() common.Address {
	return sb.address
}

// Broadcast implements tendermint.Backend.Broadcast
// It sends message to its validator by calling gossiping, and send message to itself by eventMux
func (sb *Backend) Broadcast(valSet tendermint.ValidatorSet, blockNumber *big.Int, round int64, msgType uint64, payload []byte) error {
	// send to others
	if err := sb.Gossip(valSet, blockNumber, round, msgType, payload); err != nil {
		return err
	}
	// send to self
	go func() {
		if err := sb.checkAndSendMsg(payload); err != nil {
			log.Error("failed to post event to self", "error", err)
		}
	}()
	return nil
}

type broadcastTask struct {
	Payload     []byte
	MinPeers    int
	TotalPeers  int
	Targets     map[common.Address]bool
	BlockNumber *big.Int
	Round       int64
	MsgType     uint64
}

// Gossip implements tendermint.Backend.Gossip
// It sends message to its validators only, not itself.
// The validators must be able to connected through Peer.
// It will return backend.ErrNoBroadcaster if no broadcaster is set for backend
func (sb *Backend) Gossip(valSet tendermint.ValidatorSet, blockNumber *big.Int, round int64, msgType uint64, payload []byte) error {
	targets := make(map[common.Address]bool)

	for _, val := range valSet.List() {
		if val.Address() != sb.address {
			targets[val.Address()] = true
		}
	}
	if sb.broadcaster == nil {
		return ErrNoBroadcaster
	}
	if len(targets) > 0 {
		task := broadcastTask{
			Payload:     payload,
			MinPeers:    valSet.MinPeers(),
			Targets:     targets,
			TotalPeers:  len(targets),
			BlockNumber: blockNumber,
			Round:       round,
			MsgType:     msgType,
		}
		go sb.gossip(task)
	}
	return nil
}

func (sb *Backend) gossip(task broadcastTask) {
	var (
		timeSleep   = initialBroadcastSleepTime
		successSent = 0
		mu          sync.Mutex

		finalEvtSub = sb.EventMux().Subscribe(tendermint.FinalCommittedEvent{})
		stopEvtSub  = sb.EventMux().Subscribe(tendermint.StopCoreEvent{})
		abort       = make(chan struct{})
	)
	defer func() {
		finalEvtSub.Unsubscribe()
		stopEvtSub.Unsubscribe()
	}()
	// close abort go routine if new block or core.abort()
	go func() {
		for {
			select {
			case finalEvt, ok := <-finalEvtSub.Chan():
				if !ok {
					return
				}
				finalizedBlock := finalEvt.Data.(tendermint.FinalCommittedEvent).BlockNumber
				if finalizedBlock.Cmp(task.BlockNumber) >= 0 {
					log.Info("cancel broadcast task because of final event", "task_block", task.BlockNumber, "finalized_block", finalizedBlock)
					close(abort)
					return
				}
			case _ = <-stopEvtSub.Chan():
				log.Info("cancel broadcast task because core is stopped")
				close(abort)
				return
			}
		}
	}()
	for {
		ps := sb.broadcaster.FindPeers(task.Targets)
		log.Info("find peers", "found_peers", len(ps),
			"block", task.BlockNumber, "round", task.Round, "msg_type", task.MsgType)
		done := make(chan struct{})
		var wg sync.WaitGroup
		for addr, p := range ps {
			wg.Add(1)
			go func(p consensus.Peer, addr common.Address) {
				defer wg.Done()
				if err := p.Send(consensus.TendermintMsg, task.Payload); err != nil {
					log.Error("failed to send message to peer", "error", err, "addr", addr,
						"block", task.BlockNumber, "round", task.Round, "msg_type", task.MsgType)
					return
				}
				mu.Lock()
				delete(task.Targets, addr)
				successSent += 1
				mu.Unlock()
			}(p, addr)
		}
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			log.Info("gossip to peers", "found_peers", len(ps), "min", task.MinPeers, "success", successSent,
				"block", task.BlockNumber, "round", task.Round, "msg_type", task.MsgType)
			if successSent >= task.MinPeers {
				return
			}
		case _ = <-abort:
			return
		}
		// sleep and retries until success or core abort or new block event
		log.Info("failed to sent to peer, sleeping", "time_sleep", timeSleep)
		select {
		case <-time.After(timeSleep):
		case _ = <-abort:
			return
		}
		// increase timeSleep 100ms after each epoch until timeSleep >= maxBroadcastSleepTime
		if timeSleep < maxBroadcastSleepTime {
			timeSleep += broadcastSleepTimeIncreament
		}
	}
}

// Multicast implements tendermint.Backend.Multicast
// Send msgs to peers in a set of address
// return err if not found peer with address or sending failed
func (sb *Backend) Multicast(targets map[common.Address]bool, payload []byte) error {
	if sb.broadcaster == nil {
		return ErrNoBroadcaster
	}
	if len(targets) == 0 {
		return nil
	}
	var (
		failed   int64 = 0
		ps             = sb.broadcaster.FindPeers(targets)
		notFound       = len(targets) - len(ps)
	)
	log.Trace("multicast", "targets", len(targets), "found", len(ps))
	var wg sync.WaitGroup
	for a, p := range ps {
		wg.Add(1)
		go func(addr common.Address, peer consensus.Peer) {
			defer wg.Done()
			if err := peer.Send(consensus.TendermintMsg, payload); err != nil {
				atomic.AddInt64(&failed, 1)
				log.Debug("failed to send when multicast", "err", err, "addr", addr)
			}
		}(a, p)
	}
	wg.Wait()
	if failed != 0 || notFound != 0 {
		return errors.Errorf("failed to multicast: failed to send %d address, not found %d address", failed, notFound)
	}
	return nil
}

// Validators return validator set for a block number
func (sb *Backend) Validators(blockNumber *big.Int) tendermint.ValidatorSet {
	valSet, err := sb.valSetInfo.GetValSet(sb.chain, blockNumber)
	if err != nil {
		log.Error("failed to get validator set", "error", err, "block", blockNumber.Int64())
	}
	return valSet
}

// FindExistingPeers check validator peers exist or not by address
func (sb *Backend) FindExistingPeers(valSet tendermint.ValidatorSet) map[common.Address]consensus.Peer {
	targets := make(map[common.Address]bool)
	for _, val := range valSet.List() {
		if val.Address() != sb.Address() {
			targets[val.Address()] = true
		}
	}
	return sb.broadcaster.FindPeers(targets)
}

//Commit implement tendermint.Backend.Commit()
func (sb *Backend) Commit(block *types.Block) {
	isSent := sb.commitChs.sendBlock(block)
	// if don't have committed channel to sent, then enqueue for downloading
	if !isSent {
		sb.broadcaster.Enqueue(fetcherID, block)
	}
}

func (sb *Backend) Cancel(block *types.Block) {
	sb.commitChs.closeAndRemoveCommitChannel(block.Number().String())
}

func (sb *Backend) CurrentHeadBlock() *types.Block {
	return sb.currentBlock()
}

// ValidatorsByChainReader returns val-set from snapshot
func (sb *Backend) ValidatorsByChainReader(blockNumber *big.Int, chain consensus.ChainReader) tendermint.ValidatorSet {
	valSet, err := sb.valSetInfo.GetValSet(chain, blockNumber)
	if err != nil {
		log.Error("failed to get validator set", "error", err, "block", blockNumber.Int64())
	}
	return valSet
}

// VerifyProposalBlock verify post-processor state of proposal block (txs, Root, receipt).
// If success, the result will be send to the pending tasks of miner
func (sb *Backend) VerifyProposalBlock(block *types.Block) error {
	//if block from this node, there is no need to verify state
	if block.Coinbase() == sb.Address() {
		return nil
	}
	//verify txs, stateRoot and receipt
	if sb.verifyAndSubmitBlock == nil {
		return errors.New("no verify block hook")
	}
	if err := sb.verifyAndSubmitBlock(block); err != nil {
		return err
	}
	return nil
}
