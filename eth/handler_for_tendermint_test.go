package eth

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/evrynet-official/evrynet-client/common"
	"github.com/evrynet-official/evrynet-client/consensus/tendermint"
	tendermintBackend "github.com/evrynet-official/evrynet-client/consensus/tendermint/backend"
	"github.com/evrynet-official/evrynet-client/consensus/tendermint/validator"
	"github.com/evrynet-official/evrynet-client/crypto"
	"github.com/evrynet-official/evrynet-client/p2p"
	"github.com/evrynet-official/evrynet-client/p2p/enode"
)

//TestTendermintBroadcast setup a test to broadcast a message from a node
//Broadcast included Gossip hence Gossip is not required to test separatedly
//Expectation: the MessageEvent is shown for consensus/tendermint/core.handleEvents (internal events)
//And the Message's Hash is shown for consensus/tendermint/backend.HandleMsg (external message from peers)
func TestTendermintBroadcast(t *testing.T) {
	var (
		nodePk1 = mustGeneratePrivateKey(t)
		nodePk2 = mustGeneratePrivateKey(t)
		tbe1    = tendermintBackend.New(tendermint.DefaultConfig, nodePk1)
		addrs   = []common.Address{
			crypto.PubkeyToAddress(nodePk1.PublicKey),
			crypto.PubkeyToAddress(nodePk2.PublicKey),
		}
		validatorSet = validator.NewSet(addrs, tendermint.RoundRobin)
		totalPeers   = 2
		n1           = enode.MustParseV4("enode://" + hex.EncodeToString(crypto.FromECDSAPub(&nodePk1.PublicKey)[1:]) + "@33.4.2.1:30303")
		n2           = enode.MustParseV4("enode://" + hex.EncodeToString(crypto.FromECDSAPub(&nodePk2.PublicKey)[1:]) + "@33.4.2.1:30304")
	)
	assert.NoError(t, tbe1.Start(nil, nil))
	pm1, err := NewTestProtocolManagerWithConsensus(tbe1)
	time.Sleep(2 * time.Second)
	assert.NoError(t, err)
	defer pm1.Stop()

	//Create 2 Pipe for read and write. These are full duplex
	io1, io2 := p2p.MsgPipe()

	//p1 will write to io2, p2 will receive from io1 and vice versal.
	p1 := pm1.NewPeer(63, p2p.NewPeerFromNode(n1, fmt.Sprintf("Peer %d", 0), nil), io2)
	p2 := pm1.NewPeer(63, p2p.NewPeerFromNode(n2, fmt.Sprintf("Peer %d", 1), nil), io1)
	assert.NoError(t, RegisterNewPeer(pm1, p1))
	assert.NoError(t, RegisterNewPeer(pm1, p2))

	// assert it back to tendermint Backend to call Gossip.
	bc, ok := tbe1.(tendermint.Backend)
	assert.Equal(t, true, ok)

	payload := []byte("vote message")
	assert.NoError(t, bc.Broadcast(validatorSet, payload))
	time.Sleep(2 * time.Second)

	//Making sure that the handlingMsg is done by calling pm.HandleMsg
	var (
		errCh         = make(chan error, totalPeers)
		doneCh        = make(chan struct{}, totalPeers)
		receivedCount int
		expectedCount = 1
	)
	timeout := time.After(20 * time.Second)
	for _, p := range []*Peer{p1, p2} {
		go func(p *Peer) {
			for {
				if err := pm1.HandleMsg(p); err != nil {
					errCh <- err
				} else {
					doneCh <- struct{}{}
				}
			}
		}(p)
	}
outer:
	for {
		select {
		case err = <-errCh:
			fmt.Printf("handling error %v\n", err)
			break outer
		case <-doneCh:
			receivedCount++
			if receivedCount >= expectedCount {
				fmt.Printf("handling done ")
				break outer
			}

		case <-timeout:
			fmt.Printf("timdeout")

			t.Fail()
			break outer
		}

	}
	if err != nil {
		t.Errorf("error handling msg by Peer: %v", err)
	}
}