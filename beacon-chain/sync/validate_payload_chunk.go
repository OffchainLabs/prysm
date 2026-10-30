package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// maxPendingChunkRoots bounds the buffer of chunks whose bid commitment is not
// yet known. Each root holds at most TOTAL_EXECUTION_PAYLOAD_CHUNKS proof-valid
// chunks, so the worst case is maxPendingChunkRoots * MAX_PAYLOAD_SIZE bytes.
const maxPendingChunkRoots = 8

// pendingChunkEntry buffers early-arriving chunks for one chunk commitment.
type pendingChunkEntry struct {
	slot   primitives.Slot
	chunks [fieldparams.TotalExecutionPayloadChunks]*ethpb.PayloadChunk
}

// validatePayloadChunk implements the EIP-8411 execution_payload_chunks gossip
// validations:
//  1. [IGNORE] chunk.slot matches the current slot within MAXIMUM_GOSSIP_CLOCK_DISPARITY.
//  2. [REJECT] chunk.index < TOTAL_EXECUTION_PAYLOAD_CHUNKS.
//  3. [IGNORE] (chunk.slot, chunk.payload_chunks_root) is a known bid commitment;
//     unknown-root chunks are briefly buffered and revalidated on bid arrival.
//  4. [REJECT] the inclusion proof verifies.
//  5. [REJECT] if a valid block's bid commits to this root, chunk.beacon_block_root
//     must equal that block's root.
//  6. [IGNORE] first-seen per (payload_chunks_root, index); marked seen on accept.
func (s *Service) validatePayloadChunk(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}
	receivedTime := prysmTime.Now()

	_, span := trace.StartSpan(ctx, "sync.validatePayloadChunk")
	defer span.End()

	if msg.Topic == nil {
		return pubsub.ValidationReject, p2p.ErrInvalidTopic
	}

	m, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}
	c, ok := m.(*ethpb.PayloadChunk)
	if !ok {
		return pubsub.ValidationReject, errWrongMessage
	}
	if len(c.PayloadChunksRoot) != fieldparams.RootLength || len(c.BeaconBlockRoot) != fieldparams.RootLength {
		return pubsub.ValidationReject, errors.New("payload chunk roots have wrong length")
	}

	s.prunePayloadChunkBuffers()

	// [IGNORE] chunk.slot matches the current slot within MAXIMUM_GOSSIP_CLOCK_DISPARITY.
	if err := s.validateChunkSlotWindow(c.Slot); err != nil {
		return pubsub.ValidationIgnore, err
	}

	// [REJECT] chunk.index is below TOTAL_EXECUTION_PAYLOAD_CHUNKS. The bound is
	// security critical: the Merkle walk reads only the low proof-depth bits.
	if c.Index >= uint64(fieldparams.TotalExecutionPayloadChunks) {
		return pubsub.ValidationReject, errors.New("payload chunk index out of range")
	}

	// [REJECT] the inclusion proof is valid. Checked before the commitment rule
	// so the unknown-root buffer only ever holds internally consistent chunks.
	if err := heze.VerifyPayloadChunkInclusionProof(c); err != nil {
		return pubsub.ValidationReject, err
	}

	chunksRoot := bytesutil.ToBytes32(c.PayloadChunksRoot)

	// [IGNORE] a known bid committed to (chunk.slot, chunk.payload_chunks_root).
	blockRoot, known := s.payloadChunkCommitmentBlockRoot(c.Slot, chunksRoot)
	if !known {
		s.queuePendingPayloadChunk(c)
		return pubsub.ValidationIgnore, errors.New("unknown payload chunk commitment")
	}

	// [REJECT] the routing hint must match the block containing the bid, once known.
	if blockRoot != ([32]byte{}) && bytesutil.ToBytes32(c.BeaconBlockRoot) != blockRoot {
		return pubsub.ValidationReject, errors.New("payload chunk beacon_block_root does not match the block committing to it")
	}

	// [IGNORE] first-seen per (payload_chunks_root, index).
	if s.hasSeenPayloadChunk(chunksRoot, c.Index) {
		return pubsub.ValidationIgnore, nil
	}
	s.setSeenPayloadChunk(c.Slot, chunksRoot, c.Index)

	msg.ValidatorData = c
	if startTime, err := slots.StartTime(s.cfg.clock.GenesisTime(), c.Slot); err == nil {
		payloadChunkArrivalDelaySeconds.Observe(receivedTime.Sub(startTime).Seconds())
	}
	return pubsub.ValidationAccept, nil
}

// validateChunkSlotWindow checks that the wall clock sits inside chunk.slot
// extended by MAXIMUM_GOSSIP_CLOCK_DISPARITY on both sides.
func (s *Service) validateChunkSlotWindow(slot primitives.Slot) error {
	start, err := slots.StartTime(s.cfg.clock.GenesisTime(), slot)
	if err != nil {
		return err
	}
	disparity := params.BeaconConfig().MaximumGossipClockDisparityDuration()
	now := prysmTime.Now()
	if now.Before(start.Add(-disparity)) {
		return errors.New("payload chunk is from a future slot")
	}
	if now.After(start.Add(params.BeaconConfig().SlotDuration()).Add(disparity)) {
		return errors.New("payload chunk is from a past slot")
	}
	return nil
}

// payloadChunkSubscriber feeds validated chunks into the reconstruction pool.
func (s *Service) payloadChunkSubscriber(ctx context.Context, msg proto.Message) error {
	c, ok := msg.(*ethpb.PayloadChunk)
	if !ok {
		return errWrongMessage
	}
	s.acceptVerifiedPayloadChunk(ctx, c)
	return nil
}

// queuePendingPayloadChunk buffers a proof-valid chunk whose commitment is not
// yet known. Without this, a chunk that beats its bid or block would be lost
// for the pubsub seen-cache TTL since IGNOREd messages are not redelivered.
func (s *Service) queuePendingPayloadChunk(c *ethpb.PayloadChunk) {
	if c.Slot != s.cfg.clock.CurrentSlot() {
		return
	}
	chunksRoot := bytesutil.ToBytes32(c.PayloadChunksRoot)

	s.pendingPayloadChunksLock.Lock()
	defer s.pendingPayloadChunksLock.Unlock()
	entry, ok := s.pendingPayloadChunks[chunksRoot]
	if !ok {
		if len(s.pendingPayloadChunks) >= maxPendingChunkRoots {
			log.Debug("Too many pending payload chunk roots, dropping chunk with unknown commitment")
			return
		}
		entry = &pendingChunkEntry{slot: c.Slot}
		s.pendingPayloadChunks[chunksRoot] = entry
	}
	if entry.chunks[c.Index] == nil {
		entry.chunks[c.Index] = c
	}
}

// drainPendingPayloadChunks revalidates buffered chunks for a commitment that
// just became known and feeds the survivors into the reconstruction pool.
func (s *Service) drainPendingPayloadChunks(slot primitives.Slot, chunksRoot [32]byte) {
	s.pendingPayloadChunksLock.Lock()
	entry, ok := s.pendingPayloadChunks[chunksRoot]
	if ok {
		delete(s.pendingPayloadChunks, chunksRoot)
	}
	s.pendingPayloadChunksLock.Unlock()

	blockRoot, known := s.payloadChunkCommitmentBlockRoot(slot, chunksRoot)
	if !known {
		return
	}
	if ok && entry.slot == slot {
		for _, c := range entry.chunks {
			if c == nil {
				continue
			}
			// Rule 5 against the now-known block binding.
			if blockRoot != ([32]byte{}) && bytesutil.ToBytes32(c.BeaconBlockRoot) != blockRoot {
				continue
			}
			if s.hasSeenPayloadChunk(chunksRoot, c.Index) {
				continue
			}
			s.setSeenPayloadChunk(c.Slot, chunksRoot, c.Index)
			s.acceptVerifiedPayloadChunk(s.ctx, c)
			// EIP-8411 requires a chunk that passes validation to be forwarded
			// immediately. A buffered chunk was returned as ValidationIgnore,
			// so libp2p never relayed it; now that the commitment is known and
			// the chunk has passed, this node is the only path onward for its
			// peers. Without this the set reconstructs locally while downstream
			// receivers starve.
			if err := s.cfg.p2p.Broadcast(s.ctx, c); err != nil {
				log.WithError(err).Debug("Could not forward drained payload chunk")
			}
		}
	}

	// A set that completed before its block binding was known was either held
	// back (zero hint) or parked under an unauthenticated hint that may be
	// forged. Re-import it with the authoritative binding; duplicate imports
	// dedupe downstream.
	if blockRoot != ([32]byte{}) {
		if chunks, done := s.payloadChunkPool.completedChunks(chunksRoot, slot); done {
			s.importReconstructedEnvelope(s.ctx, slot, chunksRoot, [32]byte{}, chunks)
		}
	}
}

// prunePayloadChunkBuffers drops stale pending chunks and pool entries once the
// wall clock moves to a new slot.
func (s *Service) prunePayloadChunkBuffers() {
	current := s.cfg.clock.CurrentSlot()

	// One slot of grace, matching the validator's clock-disparity allowance and
	// the pool's own pruning. Dropping at the exact boundary discards buffers
	// whose remaining chunks are still admissible.
	s.pendingPayloadChunksLock.Lock()
	for root, entry := range s.pendingPayloadChunks {
		if current > 0 && entry.slot+1 < current {
			delete(s.pendingPayloadChunks, root)
		}
	}
	s.pendingPayloadChunksLock.Unlock()

	s.payloadChunkPool.pruneBefore(current)
}
