package sync

import (
	"context"
	stderrors "errors"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

// pendingDataColumnExpSlots is the number of slots after which a queued data column
// sidecar is considered stale and evicted.
const pendingDataColumnExpSlots primitives.Slot = 4

// pendingGloasDataColumnEntry holds a Gloas data column sidecar that arrived before
// its block was seen, queued for deferred re-validation once the block arrives.
type pendingGloasDataColumnEntry struct {
	roDataColumn blocks.RODataColumn
	// topic is preserved from the original gossip message for subnet re-validation.
	topic string
	// forwardingPeer is the peer that sent us this sidecar. Stored so we can
	// downgrade it retroactively if re-validation later rejects the sidecar.
	forwardingPeer peer.ID
	arrivalSlot    primitives.Slot
}

// processPendingDataColumnsForRoot is called from beaconBlockSubscriber immediately
// after a block is received and added to the DB. It does two things in one pass:
//
//  1. Evicts stale entries across all pending roots (sidecars whose block never
//     arrived within pendingDataColumnExpSlots slots).
//  2. Re-validates and processes all queued sidecars for the newly arrived root.
//
// On re-validation failure (REJECT): the forwarding peer is downscored.
// On success: the sidecar is saved to the chain and re-broadcast to the network,
// because the original gossip message was returned as ValidationIgnore.
func (s *Service) processPendingDataColumnsForRoot(ctx context.Context, root [32]byte) {
	const dataColumnSidecarSubTopic = "/data_column_sidecar_%d/"

	currentSlot := s.cfg.clock.CurrentSlot()

	// Evict stale entries and extract the entries for this root in a single lock window.
	s.pendingDataColumnsLock.Lock()
	for r, entries := range s.pendingDataColumnsByRoot {
		var live []pendingGloasDataColumnEntry
		for _, e := range entries {
			if currentSlot > e.arrivalSlot+pendingDataColumnExpSlots {
				delete(s.pendingDataColumnKeys, computeRootIndexCacheKey(e.roDataColumn.BlockRoot(), e.roDataColumn.Index()))
				continue
			}
			live = append(live, e)
		}
		if len(live) == 0 {
			delete(s.pendingDataColumnsByRoot, r)
		} else {
			s.pendingDataColumnsByRoot[r] = live
		}
	}
	entries, ok := s.pendingDataColumnsByRoot[root]
	if ok {
		delete(s.pendingDataColumnsByRoot, root)
	}
	s.pendingDataColumnsLock.Unlock()

	if !ok {
		return
	}

	for _, entry := range entries {
		key := computeRootIndexCacheKey(entry.roDataColumn.BlockRoot(), entry.roDataColumn.Index())

		// Re-run full Gloas validation now that the block is in the DB.
		// We reconstruct a minimal pubsub.Message with only the topic field set;
		// validateDataColumnGloas uses msg.Topic only for subnet validation.
		topic := entry.topic
		syntheticMsg := &pubsub.Message{Message: &pubsubpb.Message{Topic: &topic}}

		verifiedRODataColumn, err := s.validateDataColumnGloas(ctx, syntheticMsg, entry.roDataColumn, dataColumnSidecarSubTopic)

		s.pendingDataColumnsLock.Lock()
		delete(s.pendingDataColumnKeys, key)
		s.pendingDataColumnsLock.Unlock()

		if err != nil {
			// Distinguish REJECT from IGNORE.
			// REJECT: sidecar is genuinely invalid — downgrade the forwarding peer.
			// IGNORE: benign (already seen, etc.) — no penalty.
			var vErr validationError
			if stderrors.As(err, &vErr) && vErr.result == pubsub.ValidationReject {
				if entry.forwardingPeer != "" {
					newScore := s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(entry.forwardingPeer)
					log.WithFields(logrus.Fields{
						"peer":     entry.forwardingPeer,
						"slot":     entry.roDataColumn.Slot(),
						"index":    entry.roDataColumn.Index,
						"newScore": newScore,
					}).Debug("Downscored peer for deferred invalid Gloas data column sidecar")
				}
			}
			continue
		}

		// Forward to the chain — same path as a sidecar arriving after its block normally.
		if err := s.receiveDataColumnSidecar(ctx, verifiedRODataColumn); err != nil {
			log.WithError(err).
				WithField("slot", verifiedRODataColumn.Slot()).
				Debug("Failed to receive deferred Gloas data column sidecar")
			continue
		}

		// Re-broadcast: the original message was returned as ValidationIgnore so the
		// network never propagated it. Now that it is confirmed valid, push it back out.
		if err := s.cfg.p2p.BroadcastDataColumnSidecars(ctx, []blocks.VerifiedRODataColumn{verifiedRODataColumn}); err != nil {
			log.WithError(err).
				WithField("slot", verifiedRODataColumn.Slot()).
				WithField("index", verifiedRODataColumn.Index).
				Debug("Failed to re-broadcast deferred Gloas data column sidecar")
		}
	}
}

// addDataColumnToPendingQueue enqueues a Gloas data column sidecar for deferred
// re-validation once its block becomes available. forwardingPeer is the peer that
// sent us the sidecar; it will be downscored if re-validation rejects it.
// Duplicate (blockRoot, columnIndex) pairs are silently ignored.
func (s *Service) addDataColumnToPendingQueue(roDataColumn blocks.RODataColumn, topic string, forwardingPeer peer.ID) {
	key := computeRootIndexCacheKey(roDataColumn.BlockRoot(), roDataColumn.Index())

	s.pendingDataColumnsLock.Lock()
	defer s.pendingDataColumnsLock.Unlock()

	if s.pendingDataColumnsByRoot == nil || s.pendingDataColumnKeys == nil {
		return
	}
	if s.pendingDataColumnKeys[key] {
		return
	}

	blockRoot := roDataColumn.BlockRoot()
	s.pendingDataColumnKeys[key] = true
	s.pendingDataColumnsByRoot[blockRoot] = append(
		s.pendingDataColumnsByRoot[blockRoot],
		pendingGloasDataColumnEntry{
			roDataColumn:   roDataColumn,
			topic:          topic,
			forwardingPeer: forwardingPeer,
			arrivalSlot:    roDataColumn.Slot(),
		},
	)
}
