package sync

import (
	"bytes"
	"context"
	"math"

	envcoverage "github.com/OffchainLabs/prysm/v7/beacon-chain/coverage"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// EnvelopeCoverageProvider is the narrow coverage runtime dependency of the
// envelopes-by-range serving gate: a generation-coherent snapshot/index read,
// the in-process serve epoch for the pre-stream invalidation check, and a
// reconciliation wake-up for serving anomalies.
type EnvelopeCoverageProvider interface {
	CoherentServeRead(ctx context.Context, begin, end primitives.Slot, quota uint64) (*envcoverage.ServeRead, error)
	ServeEpoch() uint64
	WakeReconcile()
}

// envelopeRangeItem is one fully validated response item awaiting payload
// reconstruction.
type envelopeRangeItem struct {
	env       *pb.SignedBlindedExecutionPayloadEnvelope
	blockHash [32]byte
}

// executionPayloadEnvelopesByRangeRPCHandler serves execution payload
// envelopes for the requester's original slot range intersected with the
// envelope domain [max(1, gloasStart), current], gated on proven envelope
// coverage. It collects first from the proven region through the
// canonical-revealed slot index (O(quota), never O(window)), applies the
// live-frontier present-head rule at the coverage anchor, and refuses real
// gaps with ResourceUnavailable before the first chunk: the complete response
// is collected, validated, and reconstructed before streaming.
func (s *Service) executionPayloadEnvelopesByRangeRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.ExecutionPayloadEnvelopesByRangeHandler")
	defer span.End()
	recordResult := func(result executionPayloadEnvelopeRPCResult) {
		gloasExecutionPayloadEnvelopesRPCRequestsTotal.WithLabelValues("by_range", string(result)).Inc()
		if result == executionPayloadEnvelopeRPCResultServed {
			syncPayloadEnvelopeByRangeServedTotal.Inc()
		}
	}
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.ExecutionPayloadEnvelopesByRangeName[1:])

	r, ok := msg.(*pb.ExecutionPayloadEnvelopesByRangeRequest)
	if !ok {
		recordResult(executionPayloadEnvelopeRPCResultInvalid)
		return errors.New("message is not type *pb.ExecutionPayloadEnvelopesByRangeRequest")
	}
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		recordResult(executionPayloadEnvelopeRPCResultRateLimited)
		return err
	}

	remotePeer := stream.Conn().RemotePeer()
	log.WithFields(logrus.Fields{
		"startSlot": r.StartSlot,
		"count":     r.Count,
		"peer":      remotePeer,
	}).Debug("Serving execution payload envelopes by range request")

	invalidRequest := func(err error) error {
		recordResult(executionPayloadEnvelopeRPCResultInvalid)
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		s.downscorePeer(remotePeer, "executionPayloadEnvelopesByRangeRPCHandlerValidationError")
		tracing.AnnotateError(span, err)
		return err
	}
	if r.Count == 0 {
		return invalidRequest(errors.Wrap(p2ptypes.ErrInvalidRequest, "invalid request Count parameter"))
	}
	// The inclusive end is computed from the full count by checked addition:
	// the item quota below caps returned items, never the searched window.
	requestedEnd, err := r.StartSlot.SafeAdd(r.Count - 1)
	if err != nil {
		return invalidRequest(errors.Wrap(p2ptypes.ErrInvalidRequest, "overflow start + count - 1"))
	}
	quota := min(r.Count, params.BeaconConfig().MaxRequestPayloads)

	// Evaluate the original half-open request intersected with the envelope
	// domain [max(1, gloasStart), current]. The requester's bounds are never
	// rewritten and no slot outside them is ever streamed; an empty
	// intersection returns zero chunks and clean EOF before any coverage
	// state is consulted.
	current := s.cfg.clock.CurrentSlot()
	wBegin, wEnd, domainOK := envelopeServeWindow(r.StartSlot, requestedEnd, current)
	if !domainOK {
		recordResult(executionPayloadEnvelopeRPCResultEmptyDomain)
		envelopeRPCEmptyDomainTotal.Inc()
		closeStream(stream, log)
		return nil
	}

	unavailable := func(cause error, wake bool) error {
		recordResult(executionPayloadEnvelopeRPCResultResourceUnavailable)
		if wake && s.envelopeCoverage != nil {
			s.envelopeCoverage.WakeReconcile()
		}
		log.WithError(cause).WithFields(logrus.Fields{
			"startSlot": r.StartSlot,
			"count":     r.Count,
			"current":   current,
		}).Debug("Execution payload envelope range unavailable")
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, p2ptypes.ErrResourceUnavailable.Error(), stream)
		tracing.AnnotateError(span, cause)
		return nil
	}
	internalErr := func(cause error) error {
		recordResult(executionPayloadEnvelopeRPCResultError)
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, cause)
		return cause
	}

	if s.envelopeCoverage == nil {
		return unavailable(errors.New("envelope coverage runtime is not configured"), false)
	}
	if s.cfg.executionReconstructor == nil {
		return internalErr(errors.New("execution reconstructor is nil"))
	}

	// One bounded retry when a destructive coverage commit invalidates the
	// candidate response between the coherent read and streaming.
	for attempt := 0; ; attempt++ {
		read, err := s.envelopeCoverage.CoherentServeRead(ctx, wBegin, wEnd, quota)
		if err != nil {
			return internalErr(errors.Wrap(err, "coherent envelope coverage read"))
		}
		resp, flavor, refuseCause, err := s.collectEnvelopeRangeResponse(ctx, read, wBegin, wEnd, quota)
		if err != nil {
			return internalErr(err)
		}
		if refuseCause != nil {
			return unavailable(refuseCause, true)
		}
		full, err := s.reconstructEnvelopeItems(ctx, resp)
		if err != nil {
			return unavailable(errors.Wrap(err, "reconstruct envelope payloads"), true)
		}
		// A destructive commit after the coherent read invalidates the
		// candidate: discard and retry once, then refuse rather than mix
		// generations. Pure extension outside the old interval does not bump
		// the epoch, so progressive migration cannot starve serving.
		if s.envelopeCoverage.ServeEpoch() != read.Epoch {
			if attempt == 0 {
				envelopeRPCServeEpochTotal.WithLabelValues("retry").Inc()
				continue
			}
			envelopeRPCServeEpochTotal.WithLabelValues("refused").Inc()
			return unavailable(errors.New("coverage changed while serving"), false)
		}
		for _, env := range full {
			SetStreamWriteDeadline(stream, defaultWriteDuration)
			if chunkErr := WriteExecutionPayloadEnvelopeChunk(stream, s.cfg.p2p.Encoding(), env); chunkErr != nil {
				log.WithError(chunkErr).Debug("Could not send execution payload envelope chunk")
				s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
				tracing.AnnotateError(span, chunkErr)
				recordResult(executionPayloadEnvelopeRPCResultError)
				return chunkErr
			}
			s.rateLimiter.add(stream, 1)
		}
		switch flavor {
		case envelopeRangeFlavorQuotaTruncated:
			envelopeRPCQuotaTruncatedTotal.Inc()
		case envelopeRangeFlavorWithHead, envelopeRangeFlavorWithoutHead, envelopeRangeFlavorHeadOnly, envelopeRangeFlavorEmptyFrontier:
			envelopeRPCLiveFrontierTotal.WithLabelValues(flavor).Inc()
		}
		recordResult(executionPayloadEnvelopeRPCResultServed)
		closeStream(stream, log)
		return nil
	}
}

// Response-shape flavors recorded once per successfully streamed response.
const (
	envelopeRangeFlavorQuotaTruncated = "quota_truncated"
	envelopeRangeFlavorWithHead       = "with_head"
	envelopeRangeFlavorWithoutHead    = "without_head"
	envelopeRangeFlavorHeadOnly       = "head_only"
	envelopeRangeFlavorEmptyFrontier  = "empty"
)

// envelopeServeWindow intersects the original request [start, requestedEnd]
// with the envelope domain [max(1, gloasStart), current]. ok is false when
// the intersection is empty (Gloas unscheduled, wholly pre-Gloas, wholly at
// slot 0, or wholly in the future).
func envelopeServeWindow(start, requestedEnd, current primitives.Slot) (primitives.Slot, primitives.Slot, bool) {
	if params.BeaconConfig().GloasForkEpoch == math.MaxUint64 {
		return 0, 0, false
	}
	gloasStart, err := slots.EpochStart(params.BeaconConfig().GloasForkEpoch)
	if err != nil {
		return 0, 0, false
	}
	domainBegin := max(primitives.Slot(1), gloasStart)
	wBegin := max(start, domainBegin)
	wEnd := min(requestedEnd, current)
	if wBegin > wEnd {
		return 0, 0, false
	}
	return wBegin, wEnd, true
}

// collectEnvelopeRangeResponse assembles the complete candidate response for
// one coherent read: the proven-coverage prefix selected through the
// canonical-revealed slot index, then the live-frontier present-head item
// when applicable. It returns a refusal cause (nil response) for coverage
// gaps and invariant violations, and an error for internal failures.
func (s *Service) collectEnvelopeRangeResponse(
	ctx context.Context,
	read *envcoverage.ServeRead,
	wBegin, wEnd primitives.Slot,
	quota uint64,
) ([]envelopeRangeItem, string, error, error) {
	snap := read.Coverage
	refuse := func(cause error) ([]envelopeRangeItem, string, error, error) {
		return nil, "", cause, nil
	}
	// Base readiness: initialized, supported format, request start inside
	// proven coverage, and a still-canonical upper anchor.
	if !snap.Initialized {
		return refuse(errors.New("envelope coverage is uninitialized"))
	}
	if snap.FormatVersion != 1 {
		return refuse(errors.Errorf("unsupported envelope coverage format_version %d", snap.FormatVersion))
	}
	if wBegin < snap.Low {
		return refuse(errors.Errorf("request start %d below covered lower bound %d", wBegin, snap.Low))
	}
	canonical, err := s.cfg.chain.IsCanonical(ctx, snap.AnchorRoot)
	if err != nil {
		return nil, "", nil, errors.Wrap(err, "check anchor canonicality")
	}
	if !canonical {
		return refuse(errors.New("coverage anchor is no longer canonical"))
	}

	// Collect first from the proven region: index entries only exist for
	// revealed canonical slots, so skipped and withheld slots consume no
	// quota and cost no per-slot work.
	items := make([]envelopeRangeItem, 0, len(read.Roots))
	var prevHash []byte
	for _, sr := range read.Roots {
		env, err := s.cfg.beaconDB.ExecutionPayloadEnvelope(ctx, sr.Root)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return refuse(errors.Errorf("dangling revealed index entry at slot %d", sr.Slot))
			}
			return nil, "", nil, errors.Wrap(err, "load indexed envelope")
		}
		blk, err := s.cfg.beaconDB.Block(ctx, sr.Root)
		if err != nil || blk == nil || blk.IsNil() {
			return refuse(errors.Errorf("canonical block missing for indexed slot %d", sr.Slot))
		}
		itemCanonical, err := s.cfg.chain.IsCanonical(ctx, sr.Root)
		if err != nil {
			return nil, "", nil, errors.Wrap(err, "check indexed root canonicality")
		}
		if !itemCanonical {
			return refuse(errors.Errorf("indexed root at slot %d is not canonical", sr.Slot))
		}
		if env.Message == nil || env.Message.Slot != sr.Slot {
			return refuse(errors.Errorf("indexed envelope slot mismatch at slot %d", sr.Slot))
		}
		if verr := envcoverage.ValidateEnvelopeAgainstBlock(env, blk, sr.Root); verr != nil {
			return refuse(errors.Wrapf(verr, "indexed envelope failed validation at slot %d", sr.Slot))
		}
		// Payload-hash continuity across consecutive selected items: holds
		// across withheld and skipped gaps by construction, so a break means
		// a bracketed interior inconsistency.
		if prevHash != nil && !bytes.Equal(env.Message.ParentBlockHash, prevHash) {
			return refuse(errors.Errorf("payload hash continuity broken at slot %d", sr.Slot))
		}
		prevHash = env.Message.BlockHash
		items = append(items, envelopeRangeItem{env: env, blockHash: bytesutil.ToBytes32(env.Message.BlockHash)})
	}

	// A quota filled inside the proven region is a legal limited response
	// chosen entirely from proven coverage: clean EOF, and the internal-gap
	// and live-head branches are never consulted. No head item ever follows
	// a truncated prefix, keeping the requester's parent-hash chain intact.
	if uint64(len(items)) == quota {
		return items, envelopeRangeFlavorQuotaTruncated, nil, nil
	}

	// The proven region is exhausted under quota.
	if wEnd < snap.High {
		// The covered window is complete.
		return items, "", nil, nil
	}
	if snap.AnchorRoot != read.HeadRoot {
		// Coverage stopped at a known internal lag/gap below the canonical
		// head: refuse atomically rather than silently clamping.
		return refuse(errors.Errorf("coverage upper bound %d stopped below the canonical head", snap.High))
	}
	// Live frontier: the anchor is the canonical head. Serve a stored head
	// envelope when the head slot lies inside the requested window; genuine
	// absence yields the legal shorter response. Presence is verified
	// (verify-then-store), but never advances durable coverage: only a child
	// can classify absence or withholding at the head.
	flavor := ""
	if wBegin <= snap.High {
		env, err := s.cfg.beaconDB.ExecutionPayloadEnvelope(ctx, snap.AnchorRoot)
		switch {
		case errors.Is(err, db.ErrNotFound):
			if len(items) > 0 {
				flavor = envelopeRangeFlavorWithoutHead
			} else {
				flavor = envelopeRangeFlavorEmptyFrontier
			}
		case err != nil:
			return nil, "", nil, errors.Wrap(err, "load head envelope")
		default:
			blk, err := s.cfg.beaconDB.Block(ctx, snap.AnchorRoot)
			if err != nil || blk == nil || blk.IsNil() {
				return refuse(errors.New("canonical head block missing for stored head envelope"))
			}
			if env.Message == nil || env.Message.Slot != snap.High {
				return refuse(errors.New("stored head envelope slot mismatch"))
			}
			if verr := envcoverage.ValidateEnvelopeAgainstBlock(env, blk, snap.AnchorRoot); verr != nil {
				return refuse(errors.Wrap(verr, "stored head envelope failed validation"))
			}
			// Frontier continuity between the last prefix item and the head.
			if prevHash != nil && !bytes.Equal(env.Message.ParentBlockHash, prevHash) {
				return refuse(errors.New("payload hash continuity broken at the live frontier"))
			}
			if len(items) > 0 {
				flavor = envelopeRangeFlavorWithHead
			} else {
				flavor = envelopeRangeFlavorHeadOnly
			}
			items = append(items, envelopeRangeItem{env: env, blockHash: bytesutil.ToBytes32(env.Message.BlockHash)})
		}
	} else {
		// The head is outside the requested window (W.begin > high).
		flavor = envelopeRangeFlavorEmptyFrontier
	}
	return items, flavor, nil, nil
}

// reconstructEnvelopeItems batch-reconstructs full payloads for every
// candidate item before any chunk is written. Any missing payload fails the
// whole response.
func (s *Service) reconstructEnvelopeItems(ctx context.Context, items []envelopeRangeItem) ([]*pb.SignedExecutionPayloadEnvelope, error) {
	if len(items) == 0 {
		return nil, nil
	}
	hashes := make([][32]byte, 0, len(items))
	for _, it := range items {
		hashes = append(hashes, it.blockHash)
	}
	payloadByHash, err := s.cfg.executionReconstructor.ReconstructFullGloasExecutionPayloadsByHash(ctx, hashes)
	if err != nil {
		return nil, errors.Wrap(err, "batch reconstruct execution payloads")
	}
	full := make([]*pb.SignedExecutionPayloadEnvelope, 0, len(items))
	for _, it := range items {
		payload := payloadByHash[it.blockHash]
		if payload == nil {
			return nil, errors.Errorf("missing reconstructed payload for block hash %#x", it.blockHash)
		}
		full = append(full, &pb.SignedExecutionPayloadEnvelope{
			Message: &pb.ExecutionPayloadEnvelope{
				Payload:               payload,
				ExecutionRequests:     it.env.Message.ExecutionRequests,
				BuilderIndex:          it.env.Message.BuilderIndex,
				BeaconBlockRoot:       it.env.Message.BeaconBlockRoot,
				ParentBeaconBlockRoot: it.env.Message.ParentBeaconBlockRoot,
			},
			Signature: it.env.Signature,
		})
	}
	return full, nil
}
