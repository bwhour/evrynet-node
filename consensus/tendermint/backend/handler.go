package backend

import (
	"errors"
	"fmt"
	"math/big"

	queue "github.com/enriquebris/goconcurrentqueue"
	"golang.org/x/crypto/sha3"

	"github.com/Evrynetlabs/evrynet-node/common"
	"github.com/Evrynetlabs/evrynet-node/consensus"
	"github.com/Evrynetlabs/evrynet-node/consensus/tendermint"
	"github.com/Evrynetlabs/evrynet-node/log"
	"github.com/Evrynetlabs/evrynet-node/p2p"
	"github.com/Evrynetlabs/evrynet-node/rlp"
)

var (
	// errDecodeFailed is returned when decode message fails
	errDecodeFailed = errors.New("fail to decode istanbul message")
)

func rLPHash(v interface{}) (h common.Hash) {
	hw := sha3.New256()
	_ = rlp.Encode(hw, v)
	hw.Sum(h[:0])
	return h
}

func (sb *Backend) decode(msg p2p.Msg) ([]byte, common.Hash, error) {
	var data []byte
	if err := msg.Decode(&data); err != nil {
		return nil, common.Hash{}, errDecodeFailed
	}
	return data, rLPHash(data), nil
}

func (sb *Backend) sendDataToCore(data []byte) error {
	return sb.checkAndSendMsg(data)
}

func (sb *Backend) replayTendermintMsg() (done bool, err error) {
	sb.mutex.RLock()
	defer sb.mutex.RUnlock()
	if !sb.coreStarted {
		log.Info("core stopped. Exit replaying tendermint msg to core.")
		return true, nil
	}
	if sb.storingMsgs.GetLen() == 0 {
		return true, nil
	}

	stored, err := sb.storingMsgs.Get(0)
	if err != nil {
		if queueErr, ok := err.(*queue.QueueError); ok {
			if queueErr.Code() == queue.QueueErrorCodeEmptyQueue { // avoid get error when queue.length == 0
				return true, nil
			}
		}
		log.Error("failed to get data from queue", "error", err)
		return false, err
	}
	if err := sb.sendDataToCore(stored.([]byte)); err != nil {
		log.Error("failed to Post msg to core", "error", err)
		return false, err
	}
	_, _ = sb.storingMsgs.Dequeue()
	return false, nil
}

func (sb *Backend) dequeueMsgLoop() {
	for {
		select {
		case <-sb.dequeueMsgTriggering: // w8 signal to trigger dequeue msg
			log.Trace("replay msg started")
		replayLoop:
			for {
				// replay message one by one to core until there is no more message
				done, err := sb.replayTendermintMsg()
				if err != nil {
					log.Error("failed to replayTendermintMsg", "err", err)
					break replayLoop
				}
				if done {
					break replayLoop
				}
			}
		case <-sb.closingBackgroundThreadsCh:
			log.Trace("interrupt dequeue message loop")
			return
		}
	}
}

// HandleMsg implements consensus.Handler.HandleMsg
// return false if the message cannot be handle by Tendermint Backend
func (sb *Backend) HandleMsg(addr common.Address, msg p2p.Msg) (bool, error) {
	switch msg.Code {
	case consensus.TendermintMsg:
		decodedMsg, _, err := sb.decode(msg)
		if err != nil {
			log.Error("failed to decode message from p2p.Msg", "err", err)
			return true, err
		}

		//Dequeue if storingMsg reached max
		if sb.storingMsgs.GetLen() >= maxNumberMessages {
			for n := sb.storingMsgs.GetLen() - maxNumberMessages; n > 0; n-- {
				//Free a slot for new message
				_, err := sb.storingMsgs.Dequeue()
				if err != nil {
					log.Error("failed to free a message from queue", "err", err)
					return true, err
				}
			}
		}

		if err := sb.storingMsgs.Enqueue(decodedMsg); err != nil {
			log.Error("failed to store message to queue", "err", err)
			return true, err
		}

		//log.Debug("Received Message from peer", "address", addr.Hex(), "code", msg.Code, "hash", hash.String())
		//TODO: mark peer's message and self known message with the hash get from message

		// Trigger dequeue loop
		go func() {
			select {
			case sb.dequeueMsgTriggering <- struct{}{}:
			case <-sb.closingBackgroundThreadsCh:
				log.Trace("interrupt trigger dequeue loop when handling message")
				return
			}
		}()
		return true, nil
	default:
		return false, fmt.Errorf("unknown message code %d for Tendermint's protocol", msg.Code)
		//TODO:Handler other cases
		//Case 1: NewBlock when this node is the propose.
		//More cases to be added...
	}
}

// HandleNewChainHead implements consensus.Handler.HandleNewChainHead
func (sb *Backend) HandleNewChainHead(blockNumber *big.Int) error {
	sb.mutex.RLock()
	defer sb.mutex.RUnlock()
	if !sb.coreStarted {
		return tendermint.ErrStoppedEngine
	}
	sb.commitChs.closeAndRemoveCommitChannel(blockNumber.String())
	go func() {
		if err := sb.tendermintEventMux.Post(tendermint.FinalCommittedEvent{
			BlockNumber: blockNumber}); err != nil {
			log.Error("failed to post FinalCommittedEvent to core", "error", err)
		}
	}()
	return nil
}
