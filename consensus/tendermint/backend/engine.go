package backend

import (
	"bytes"
	"errors"
	"math/big"
	"reflect"
	"time"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/utils"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint/validator"
	"github.com/Evrynetlabs/evrynet-node/core/state"
	"github.com/Evrynetlabs/evrynet-node/core/state/staking"
	"github.com/Evrynetlabs/evrynet-node/core/types"
	"github.com/Evrynetlabs/evrynet-node/core/vm"
	"github.com/Evrynetlabs/evrynet-node/log"
	"github.com/Evrynetlabs/evrynet-node/rlp"
	"github.com/Evrynetlabs/evrynet-node/rpc"
)

var (
	validatorRewardPercentage int64 = 50
	voterRewardPercentage     int64 = 50

	defaultDifficulty = big.NewInt(1)
	now               = time.Now
)

const (
	// Number of blocks after which to save the vote snapshot to the database
	checkpointInterval = 1024
)

var (
	peerWaitDuration = 2 * time.Second
)

func (sb *Backend) addProposalSeal(h *types.Header) error {
	seal, err := sb.Sign(utils.SigHash(h).Bytes())
	if err != nil {
		return err
	}
	return utils.WriteSeal(h, seal)
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top.
func (sb *Backend) Seal(chain consensus.ChainReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) (err error) {
	// update the block header timestamp and signature and propose the block to core engine
	header := block.Header()
	blockNumber := header.Number.Uint64()
	if blockNumber == 0 {
		return errors.New("cannot Seal block 0")
	}
	// validate address of the validator
	valSet, err := sb.valSetInfo.GetValSet(chain, big.NewInt(int64(blockNumber)))
	if err != nil {
		return err
	}

	if err = sb.addProposalSeal(header); err != nil {
		return err
	}
	block = block.WithSeal(header)
	// checks the address must stored in snapshot
	if _, v := valSet.GetByAddress(sb.address); v == nil {
		return tendermint.ErrUnauthorized
	}

	// update seal to header of the block
	parent := chain.GetHeader(header.ParentHash, blockNumber-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	// wait for the timestamp of header, make sure this block does not come from the future
	headerTime := int64(block.Header().Time)
	delay := time.Unix(headerTime, 0).Sub(now())
	//TODO: revise delay
	select {
	case <-time.After(delay):
	case <-stop:
		results <- nil
		return
	}

	ch := sb.commitChs.createCommitChannelAndCloseIfExist(block.Number().String())
	// post block into tendermint engine
	go func(block *types.Block) {
		//nolint:tendermint.Errheck
		sb.EventMux().Post(tendermint.NewBlockEvent{
			Block: block,
		})
	}(block)
	// miner won't be able to interrupt a sealing task
	// a sealing task can only exist when core consensus agreed upon a block
	go func(ch <-chan *types.Block) {
		select {
		case bl, ok := <-ch:
			if ok {
				//this step is to stop other go routine wait for a block
				sb.commitChs.closeAndRemoveCommitChannel(bl.Number().String())
				log.Info("committing... returned block to miner", "block_hash", bl.Hash(), "number", bl.Number())
				results <- bl
				return
			}
		case <-sb.closingBackgroundThreadsCh:
			log.Trace("interrupt commit channel")
			return
		}
	}(ch)
	return nil
}

//tryStartCore will attempt to start core
//it return true if core is already start/ started successfully
//false in case core's still waiting for enough peer to start
func (sb *Backend) tryStartCore() bool {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()
	log.Info("attempt to start tendermint core")
	if sb.coreStarted {
		log.Warn("core is already started", "error", tendermint.ErrStartedEngine)
		return true
	}

	// Check enough 2f+1 peers
	valSet := sb.Validators(sb.currentBlock().Number())
	if len(sb.FindExistingPeers(valSet)) < valSet.MinPeers() {
		log.Warn("not enough 2f+1 peers to start backend")
		return false
	}

	if err := sb.core.Start(); err != nil {
		log.Error("failed to start tendermint core", "error", err)
		return false
	}
	sb.coreStarted = true
	// trigger dequeue msg loop
	go func() {
		select {
		case sb.dequeueMsgTriggering <- struct{}{}:
		case <-sb.closingBackgroundThreadsCh:
			log.Trace("interrupt trigger dequeue loop when starting core")
			return
		}
	}()
	return true
}

// Start implements consensus.Tendermint.Start
func (sb *Backend) Start(chain consensus.FullChainReader, currentBlock func() *types.Block, verifyAndSubmitBlock func(*types.Block) error) error {
	sb.mutex.Lock()
	if sb.coreStarted {
		sb.mutex.Unlock()
		return tendermint.ErrStartedEngine
	}

	//set chain reader
	sb.chain = chain
	sb.currentBlock = currentBlock
	sb.verifyAndSubmitBlock = verifyAndSubmitBlock
	if sb.commitChs != nil {
		sb.commitChs.closeAndRemoveAllChannels()
	}
	sb.mutex.Unlock()

	//clear Previous start loop
	select {
	case sb.controlChan <- struct{}{}:
	default:
	}

	ticker := time.NewTicker(peerWaitDuration)

	for {
		select {
		case <-sb.controlChan:
			log.Info("interrupt mining start loop")
			return errors.New("start miner interrupted")
		case <-ticker.C:
			if sb.tryStartCore() {
				return nil
			}
		}
	}
}

// Stop implements consensus.Tendermint.Stop
func (sb *Backend) Stop() error {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()
	if !sb.coreStarted {
		// send to sb.controlChan if backend in tryStartCore loop
		select {
		case sb.controlChan <- struct{}{}:
		default:
		}
		return nil
	}
	if err := sb.core.Stop(); err != nil {
		return err
	}
	sb.coreStarted = false
	sb.EventMux().Post(tendermint.StopCoreEvent{})
	return nil
}

// Author retrieves the Evrynet address of the account that minted the given
// block, which may be different from the header's coinbase if a consensus
// engine is based on signatures.
func (sb *Backend) Author(header *types.Header) (common.Address, error) {
	return sb.blockProposer(header)
}

// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
func (sb *Backend) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return sb.verifyHeader(chain, header, nil)
}

// VerifyProposalHeader will call be.verifyHeader for checking
func (sb *Backend) VerifyProposalHeader(header *types.Header) error {
	if sb.chain == nil {
		return errors.New("no chain reader ")
	}
	// verify valSet in header is match with valSet from stateDB
	if header.Number.Uint64()%sb.config.Epoch == 0 {
		parent := sb.chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
		if parent == nil {
			return tendermint.ErrUnknownParent
		}
		validators, err := sb.getNextValidatorSet(sb.chain, parent)
		if err != nil {
			return err
		}
		// get validators's address from the extra-data
		valSetInHeader, err := utils.GetValSetAddresses(header)
		if err != nil {
			log.Info("No validators in the extra-data", err)
			return err
		}
		if !reflect.DeepEqual(validators, valSetInHeader) {
			return tendermint.ErrMismatchValSet
		}
	}
	return sb.verifyHeader(sb.chain, header, nil)
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (sb *Backend) verifyHeader(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	if header.Number == nil {
		return tendermint.ErrUnknownBlock
	}

	// Don't waste time checking blocks from the future
	if header.Time > big.NewInt(now().Unix()).Uint64() {
		return consensus.ErrFutureBlock
	}

	// Ensure that the extra data format is satisfied
	if _, err := types.ExtractTendermintExtra(header); err != nil {
		return tendermint.ErrInvalidExtraDataFormat
	}

	// Ensure that the mix digest is zero as we don't have fork protection currently
	if header.MixDigest != types.TendermintDigest {
		return tendermint.ErrInvalidMixDigest
	}
	// Ensure that the block's difficulty is meaningful (may not be correct at this point)
	if header.Difficulty == nil || header.Difficulty.Cmp(defaultDifficulty) != 0 {
		return tendermint.ErrInvalidDifficulty
	}

	return sb.verifyCascadingFields(chain, header, parents)
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (sb *Backend) verifyCascadingFields(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	// get block number from header of block
	blockNumber := header.Number.Uint64()
	if blockNumber == 0 {
		return nil
	}
	// Ensure that the block's timestamp isn't too close to it's parent
	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, blockNumber-1)
	}
	if parent == nil || parent.Number.Uint64() != blockNumber-1 || parent.Hash() != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}
	if parent.Time+sb.config.BlockPeriod > header.Time {
		//TODO: find out if tendermint is subject to error when Block Period is too fast
		//	return errInvalidTimestamp
		log.Warn("block time difference is too small", "different in ms", header.Time-sb.config.BlockPeriod)
	}
	// get val-sets to prepare for the verify proposal and committed seal
	valSet, err := sb.getValSetFromChain(chain, header, parents)
	if err != nil {
		return err
	}
	if err := sb.verifyProposalSeal(header, valSet); err != nil {
		return err
	}

	return sb.verifyCommittedSeals(header, valSet)
}

// getValSetFromChain returns the valset deprived from ChainReader and parents Headers
func (sb *Backend) getValSetFromChain(chain consensus.ChainReader, header *types.Header, parents []*types.Header) (tendermint.ValidatorSet, error) {
	var (
		blockNumber   = header.Number.Uint64()
		checkpoint    = utils.GetCheckpointNumber(sb.config.Epoch, header.Number.Uint64())
		hash          = header.ParentHash
		number        = header.Number.Uint64() - 1
		currentHeader = header
	)
	// if type of validator set is fixed, then use valsetInfo to get it
	if len(sb.config.FixedValidators) > 0 {
		return sb.valSetInfo.GetValSet(chain, big.NewInt(int64(blockNumber)))
	}
	// check if chain contains transition block, get it from historical data
	if chain.CurrentHeader().Number.Uint64() >= checkpoint {
		return sb.valSetInfo.GetValSet(chain, big.NewInt(int64(blockNumber)))
	}
	// check if parent headers contains transition block
	for {
		if len(parents) != 0 {
			currentHeader = parents[len(parents)-1]
			if currentHeader.Hash() != hash || currentHeader.Number.Uint64() != number {
				return nil, consensus.ErrUnknownAncestor
			}
			parents = parents[:len(parents)-1]
		} else {
			currentHeader = chain.GetHeader(hash, number)
			if currentHeader == nil {
				return nil, consensus.ErrUnknownAncestor
			}
		}
		if number%sb.config.Epoch == 0 {
			validators, err := utils.GetValSetAddresses(currentHeader)
			if err != nil {
				return nil, err
			}
			return validator.NewSet(validators, sb.config.ProposerPolicy, int64(blockNumber)), nil
		}
		number, hash = number-1, currentHeader.ParentHash
	}
	// if the parents does not contain the transition block, return an error
	return nil, tendermint.ErrUnknownBlock
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (sb *Backend) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	errorHeaders := make(chan error, len(headers))
	go func() {
		for i, header := range headers {
			err := sb.verifyHeader(chain, header, headers[:i])

			select {
			case <-abort:
				return
			case errorHeaders <- err:
			}
		}
	}()
	return abort, errorHeaders
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of a given engine.
func (sb *Backend) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return tendermint.ErrInvalidUncleHash
	}
	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (sb *Backend) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	// get parent header and ensure the signer is in parent's validator set
	blockNumber := header.Number.Uint64()
	if blockNumber == 0 {
		return tendermint.ErrUnknownBlock
	}

	// get valsets to prepare for the verify proposal
	valset, err := sb.valSetInfo.GetValSet(chain, big.NewInt(int64(blockNumber)))
	if err != nil {
		return err
	}

	return sb.verifyProposalSeal(header, valset)
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (sb *Backend) Prepare(chain consensus.FullChainReader, header *types.Header) error {
	// set coinbase with the proposer's address
	header.Coinbase = sb.Address()
	// use the same difficulty and mixDigest for all blocks
	header.MixDigest = types.TendermintDigest
	// use the same difficulty for all blocks
	// TODO: thight might reflect 2F+1 value since our block have nothing included to indicate it
	header.Difficulty = sb.CalcDifficulty(chain, header.Time, nil)

	// get parent
	var (
		blockNumber = header.Number.Uint64()
		parent      = chain.GetHeader(header.ParentHash, blockNumber-1)
	)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	// prepare extra data without validators
	header.Extra = sb.prepareExtra(header)

	// set header's timestamp from parent's timestamp and blockperiod
	var (
		parentTime  = new(big.Int).SetUint64(parent.Time)
		blockPeriod = new(big.Int).SetUint64(sb.config.BlockPeriod)
		headerTime  = new(big.Int).Add(parentTime, blockPeriod)
	)

	if headerTime.Int64() < time.Now().Unix() {
		header.Time = uint64(time.Now().Unix())
	} else {
		header.Time = headerTime.Uint64()
	}

	if err := sb.addValSetToHeader(chain, header, parent); err != nil {
		log.Error("failed to add val set to header", "err", err)
	}

	return nil
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
//
// Note, the block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (sb *Backend) Finalize(chain consensus.FullChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header) error {
	// Accumulate any block rewards and commit the final state root
	if err := sb.accumulateRewards(chain, state, header); err != nil {
		log.Error("failed to accumulateRewards", "err", err)
		return err
	}

	// Since there is a change in stateDB, its trie must be update
	// In case block reached EIP158 hash, the state will attempt to delete empty object as EIP158 sepcification
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	return nil
}

// FinalizeAndAssemble runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
//
// Note, the block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (sb *Backend) FinalizeAndAssemble(chain consensus.FullChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	// Accumulate any block rewards and commit the final state root
	if err := sb.accumulateRewards(chain, state, header); err != nil {
		log.Error("failed to accumulateRewards", "err", err)
		return nil, err
	}

	// No block rewards, so the state remains as is and uncles are dropped
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = types.CalcUncleHash(nil)

	// Assemble and return the final block for sealing
	return types.NewBlock(header, txs, nil, receipts), nil
}

// SealHash returns the hash of a block prior to it being sealed.
func (sb *Backend) SealHash(header *types.Header) (hash common.Hash) {
	return utils.SigHash(header)
}

// CalcDifficulty tempo return default difficulty
func (sb *Backend) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	return defaultDifficulty
}

// APIs will expose some RPC API methods
func (sb *Backend) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{{
		Namespace: "tendermint",
		Version:   "1.0",
		Service:   &TendermintAPI{chain: chain, be: sb},
		Public:    true,
	}}
}

// Close terminates any background threads maintained by the consensus engine.
func (sb *Backend) Close() error {
	close(sb.closingBackgroundThreadsCh)

	return nil
}

// verifyProposalSeal checks proposal seal is signed by validator
func (sb *Backend) verifyProposalSeal(header *types.Header, valSet tendermint.ValidatorSet) error {
	// resolve the authorization key and check against signers
	signer, err := sb.blockProposer(header)
	if err != nil {
		log.Error("proposal seal is invalid", "error", err)
		return err
	}
	// compare with coin base that contain the address of proposer.
	if signer != header.Coinbase {
		return tendermint.ErrCoinBaseInvalid
	}

	// Signer should be in the validator set of previous block's extraData.
	if _, v := valSet.GetByAddress(signer); v == nil {
		return tendermint.ErrUnauthorized
	}
	return nil
}

// verifyCommittedSeals checks whether every committed seal is signed by one of the parent's validators
func (sb *Backend) verifyCommittedSeals(header *types.Header, valSet tendermint.ValidatorSet) error {
	extra, err := types.ExtractTendermintExtra(header)
	if err != nil {
		return err
	}
	// The length of Committed seals should be larger than 0
	if len(extra.CommittedSeal) == 0 {
		return tendermint.ErrEmptyCommittedSeals
	}

	vals := valSet.Copy()
	// Check whether the committed seals are generated by parent's validators
	validSeal := 0
	proposalSeal := utils.PrepareCommittedSeal(header.Hash())
	// 1. Get committed seals from current header
	for _, seal := range extra.CommittedSeal {
		// 2. Get the original address by seal and parent block hash
		addr, err := utils.GetSignatureAddress(proposalSeal, seal)
		if err != nil {
			log.Error("not a valid address", "err", err)
			return tendermint.ErrInvalidSignature
		}
		// Every validator can have only one seal. If more than one seals are signed by a
		// validator, the validator cannot be found and errInvalidCommittedSeals is returned.
		if vals.RemoveValidator(addr) {
			validSeal++
		} else {
			return tendermint.ErrInvalidCommittedSeals
		}
	}

	// The length of validSeal should be larger or equal than min majority (num validator - maximum faulty)
	if validSeal < valSet.MinMajority() {
		return tendermint.ErrInvalidCommittedSeals
	}

	return nil
}

// blockProposer extracts the Evrynet account address from a signed header.
func (sb *Backend) blockProposer(header *types.Header) (common.Address, error) {
	if value, known := sb.blockProposerCache.Get(header.Number.Uint64()); known {
		if proposer, ok := value.(common.Address); ok {
			return proposer, nil
		}
	}

	// Retrieve the signature from the header extra-data
	extra, err := types.ExtractTendermintExtra(header)
	if err != nil {
		return common.Address{}, err
	}
	addr, err := utils.GetSignatureAddress(utils.SigHash(header).Bytes(), extra.Seal)
	if err != nil {
		return addr, err
	}

	sb.blockProposerCache.Add(header.Number.Uint64(), addr)
	return addr, nil
}

// addValSetToHeader Add validator set back to the tendermint extra.
func (sb *Backend) addValSetToHeader(chainReader consensus.FullChainReader, header *types.Header, parent *types.Header) error {
	var (
		blockNumber = header.Number.Uint64()
		epoch       = sb.config.Epoch
	)

	if blockNumber%epoch != 0 {
		// ignore if this block is not the end of epoch
		return nil
	}

	validators, err := sb.getNextValidatorSet(chainReader, parent)
	if err != nil {
		return err
	}

	log.Info("sets the val-set back to extra-data", "number", blockNumber)
	return utils.WriteValSet(header, validators)
}

func (sb *Backend) getNextValidatorSet(chainReader consensus.FullChainReader, header *types.Header) ([]common.Address, error) {
	if validators, known := sb.computedValSetCache.Get(header.Number.Uint64()); known {
		if addresses, ok := validators.([]common.Address); ok {
			return addresses, nil
		}
	}
	start := time.Now()
	stateDB, err := chainReader.StateAt(header.Root)
	if err != nil {
		return nil, err
	}

	stakingCaller := sb.getStakingCaller(chainReader, stateDB, header)
	validators, err := stakingCaller.GetValidators(sb.stakingContractAddr)
	if err != nil {
		return nil, err
	}
	sb.computedValSetCache.Add(header.Number.Uint64(), validators)
	log.Info("found new val set", "number", header.Number.Uint64(), "elapsed", common.PrettyDuration(time.Since(start)),
		"valset", common.PrettyAddresses(validators))
	return validators, nil
}

func (sb *Backend) getStakingCaller(chainReader consensus.FullChainReader, stateDB *state.StateDB, header *types.Header) staking.StakingCaller {
	if sb.config.UseEVMCaller {
		log.Info("using the EVM caller to get validators", "number", header.Number.Uint64())
		return staking.NewEVMStakingCaller(stateDB,
			staking.NewChainContextWrapper(sb, chainReader.GetHeader),
			header,
			chainReader.Config(),
			vm.Config{})
	} else {
		log.Info("using the StateDB caller to get validators", "number", header.Number.Uint64())
		return staking.NewStateDbStakingCaller(stateDB, sb.config.IndexStateVariables)
	}
}

func (sb *Backend) prepareExtra(header *types.Header) []byte {
	var (
		tdm     *types.TendermintExtra
		payload []byte
		buf     bytes.Buffer
		err     error
	)

	if len(header.Extra) < types.TendermintExtraVanity {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, types.TendermintExtraVanity-len(header.Extra))...)
	}
	buf.Write(header.Extra[:types.TendermintExtraVanity])

	tdm = &types.TendermintExtra{}
	payload, err = rlp.EncodeToBytes(&tdm)

	if err != nil {
		log.Error("failed to encode payload to Tendermint extra", "error", err)
	}
	return append(buf.Bytes(), payload...)
}
