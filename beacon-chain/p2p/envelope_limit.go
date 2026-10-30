package p2p

import (
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
)

// maxEnvelopesPerPeerPerSlot bounds the execution payload envelopes a peer can send into validation per slot.
const maxEnvelopesPerPeerPerSlot = 4

var envelopeTopicSuffix = "/" + GossipExecutionPayloadEnvelopeMessage + "/" + encoder.ProtocolSuffixSSZSnappy

// envelopeLimiter counts the execution payload envelopes received from each peer in the current slot.
type envelopeLimiter struct {
	mu      sync.Mutex
	genesis time.Time
	slot    primitives.Slot
	counts  map[peer.ID]int
}

func (l *envelopeLimiter) setGenesis(genesis time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.genesis = genesis
}

// admit charges up to n envelopes from pid against its allowance for the current slot and returns how many fit.
func (l *envelopeLimiter) admit(pid peer.ID, n int) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.genesis.IsZero() {
		return n
	}
	if slot := slots.CurrentSlot(l.genesis); l.counts == nil || slot != l.slot {
		l.slot = slot
		l.counts = make(map[peer.ID]int)
	}
	n = min(n, maxEnvelopesPerPeerPerSlot-l.counts[pid])
	l.counts[pid] += n
	return n
}

// inspectRPC drops envelopes beyond the sender's allowance. It runs before gossipsub marks messages
// as seen, so another peer's copy of a dropped envelope is still validated.
func (s *Service) inspectRPC(from peer.ID, rpc *pubsub.RPC) error {
	n := 0
	for _, msg := range rpc.Publish {
		if strings.HasSuffix(msg.GetTopic(), envelopeTopicSuffix) {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	admitted := s.envelopeLimiter.admit(from, n)
	if admitted == n {
		return nil
	}
	kept := make([]*pubsubpb.Message, 0, len(rpc.Publish)-(n-admitted))
	for _, msg := range rpc.Publish {
		if strings.HasSuffix(msg.GetTopic(), envelopeTopicSuffix) {
			if admitted == 0 {
				continue
			}
			admitted--
		}
		kept = append(kept, msg)
	}
	envelopesOverPeerLimit.Add(float64(len(rpc.Publish) - len(kept)))
	rpc.Publish = kept
	return nil
}
