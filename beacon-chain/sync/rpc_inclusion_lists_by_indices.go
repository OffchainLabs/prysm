package sync

import (
	"context"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type inclusionListRPCResult string

const (
	inclusionListRPCResultServed              inclusionListRPCResult = "served"
	inclusionListRPCResultInvalid             inclusionListRPCResult = "invalid"
	inclusionListRPCResultRateLimited         inclusionListRPCResult = "rate_limited"
	inclusionListRPCResultResourceUnavailable inclusionListRPCResult = "resource_unavailable"
	inclusionListRPCResultError               inclusionListRPCResult = "error"
)

// inclusionListsByIndicesRPCHandler handles the /eth2/beacon_chain/req/inclusion_lists_by_indices/1/ RPC request.
// spec: https://github.com/ethereum/consensus-specs/blob/master/specs/heze/p2p-interface.md#inclusionlistsbyindices-v1
func (s *Service) inclusionListsByIndicesRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.inclusionListsByIndicesRPCHandler")
	defer span.End()
	recordResult := func(result inclusionListRPCResult) {
		hezeInclusionListsByIndicesRPCRequestsTotal.WithLabelValues(string(result)).Inc()
	}
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.InclusionListsByIndicesName[1:]) // slice the leading slash off the name var

	req, ok := msg.(*types.InclusionListsByIndicesReq)
	if !ok {
		recordResult(inclusionListRPCResultInvalid)
		return errors.New("message is not type *types.InclusionListsByIndicesReq")
	}
	bits := bitfield.Bitvector16(req.Indices[:])
	requested := bits.Count()

	if err := s.rateLimiter.validateRequest(stream, requested); err != nil {
		recordResult(inclusionListRPCResultRateLimited)
		return errors.Wrap(err, "rate limiter validate request")
	}
	s.rateLimiter.add(stream, int64(max(requested, 1))) // lint:ignore uintcast -- A Bitvector16 has at most 16 set bits.
	defer closeStream(stream, log)

	if req.Slot < minimumInclusionListRequestSlot(s.cfg.clock.CurrentSlot()) {
		recordResult(inclusionListRPCResultResourceUnavailable)
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, types.ErrResourceUnavailable.Error(), stream)
		return nil
	}

	store := s.cfg.chain.InclusionListStore()
	// No valid stored list for the key means no state work for the request.
	if requested == 0 || len(store.Transactions(req.Slot, req.DependentRoot, false)) == 0 {
		recordResult(inclusionListRPCResultServed)
		return nil
	}

	st, err := s.cfg.chain.LookaheadState(ctx, req.DependentRoot, slots.ShufflingLookaheadEpoch(slots.ToEpoch(req.Slot)))
	if errors.Is(err, blockchain.ErrInvalidDependentRoot) || errors.Is(err, blockchain.ErrDependentStateUnavailable) {
		// Without the committee the requested positions do not map to validators, so the response has no lists.
		log.WithError(err).WithFields(logrus.Fields{
			"slot":          req.Slot,
			"dependentRoot": req.DependentRoot,
		}).Debug("Could not get the lookahead state for the inclusion list committee")
		recordResult(inclusionListRPCResultServed)
		return nil
	}
	var committee []primitives.ValidatorIndex
	if err == nil {
		committee, err = helpers.InclusionListCommittee(ctx, st, req.Slot)
	}
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"slot":          req.Slot,
			"dependentRoot": req.DependentRoot,
		}).Debug("Could not compute the inclusion list committee")
		recordResult(inclusionListRPCResultError)
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		return err
	}

	sent := make(map[primitives.ValidatorIndex]struct{}, requested)
	for i, validatorIndex := range committee {
		if !bits.BitAt(uint64(i)) {
			continue
		}
		// A validator at two requested positions gets one chunk.
		if _, ok := sent[validatorIndex]; ok {
			continue
		}
		sent[validatorIndex] = struct{}{}
		// The store keeps the first list of an equivocator, so the equivocator check must come first.
		if store.IsEquivocator(req.Slot, req.DependentRoot, validatorIndex) {
			continue
		}
		signed, _, ok := store.Get(req.Slot, req.DependentRoot, validatorIndex)
		if !ok {
			continue
		}
		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if err := WriteInclusionListChunk(stream, s.cfg.p2p.Encoding(), signed); err != nil {
			log.WithError(err).Debug("Could not send a chunked response")
			recordResult(inclusionListRPCResultError)
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			tracing.AnnotateError(span, err)
			return err
		}
	}

	recordResult(inclusionListRPCResultServed)
	return nil
}

// minimumInclusionListRequestSlot is max(current_slot - MIN_SLOTS_FOR_INCLUSION_LISTS_REQUESTS, compute_start_slot_at_epoch(HEZE_FORK_EPOCH)).
func minimumInclusionListRequestSlot(current primitives.Slot) primitives.Slot {
	hezeStart, err := slots.EpochStart(params.BeaconConfig().HezeForkEpoch)
	if err != nil {
		// The Heze fork epoch is not scheduled, so no slot can be served.
		return params.BeaconConfig().FarFutureSlot
	}
	window := primitives.Slot(params.BeaconConfig().MinSlotsForInclusionListsRequests)
	return max(current.FlooredSubSlot(window), hezeStart)
}

// WriteInclusionListChunk writes a signed inclusion list chunk with the context digest of the list's slot.
// response_chunk  ::= <result> | <context-bytes> | <encoding-dependent-header> | <encoded-payload>
func WriteInclusionListChunk(stream libp2pcore.Stream, encoding encoder.NetworkEncoding, signed *ethpb.SignedInclusionList) error {
	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
		return errors.Wrap(err, "stream write")
	}
	ctxBytes := params.ForkDigest(slots.ToEpoch(signed.Message.Slot))
	if err := writeContextToStream(ctxBytes[:], stream); err != nil {
		return errors.Wrap(err, "write context to stream")
	}
	if _, err := encoding.EncodeWithMaxLength(stream, signed); err != nil {
		return errors.Wrap(err, "encode with max length")
	}
	return nil
}
