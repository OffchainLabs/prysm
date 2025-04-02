package validator

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SubmitAggregateSelectionProof is called by a validator when its assigned to be an aggregator.
// The aggregator submits the selection proof to obtain the aggregated attestation
// object to sign over.
func (vs *Server) SubmitAggregateSelectionProof(ctx context.Context, req *ethpb.AggregateSelectionRequest) (*ethpb.AggregateSelectionResponse, error) {
	ctx, span := trace.StartSpan(ctx, "AggregatorServer.SubmitAggregateSelectionProof")
	defer span.End()
	span.SetAttributes(trace.Int64Attribute("slot", int64(req.Slot)))

	indexInCommittee, validatorIndex, err := vs.processAggregateSelection(ctx, req)
	if err != nil {
		return nil, err
	}

	var atts []*ethpb.Attestation

	if features.Get().EnableExperimentalAttestationPool {
		atts = cache.GetBySlotAndCommitteeIndex[*ethpb.Attestation](vs.AttestationCache, req.Slot, req.CommitteeIndex)
	} else {
		atts = vs.AttPool.AggregatedAttestationsBySlotIndex(ctx, req.Slot, req.CommitteeIndex)
		if len(atts) == 0 {
			atts = vs.AttPool.UnaggregatedAttestationsBySlotIndex(ctx, req.Slot, req.CommitteeIndex)
		}
	}
	if len(atts) == 0 {
		return nil, status.Errorf(codes.NotFound, "Could not find attestation for slot and committee in pool")
	}

	best := bestAggregate(atts, req.CommitteeIndex, indexInCommittee)
	attAndProof := &ethpb.AggregateAttestationAndProof{
		Aggregate:       best,
		SelectionProof:  req.SlotSignature,
		AggregatorIndex: validatorIndex,
	}
	return &ethpb.AggregateSelectionResponse{AggregateAndProof: attAndProof}, nil
}

// SubmitAggregateSelectionProofElectra is called by a validator when its assigned to be an aggregator.
// The aggregator submits the selection proof to obtain the aggregated attestation
// object to sign over.
func (vs *Server) SubmitAggregateSelectionProofElectra(
	ctx context.Context,
	req *ethpb.AggregateSelectionRequest,
) (*ethpb.AggregateSelectionElectraResponse, error) {
	ctx, span := trace.StartSpan(ctx, "AggregatorServer.SubmitAggregateSelectionProofElectra")
	defer span.End()
	span.SetAttributes(trace.Int64Attribute("slot", int64(req.Slot)))

	indexInCommittee, validatorIndex, err := vs.processAggregateSelection(ctx, req)
	if err != nil {
		return nil, err
	}

	var atts []*ethpb.AttestationElectra

	if features.Get().EnableExperimentalAttestationPool {
		atts = cache.GetBySlotAndCommitteeIndex[*ethpb.AttestationElectra](vs.AttestationCache, req.Slot, req.CommitteeIndex)
	} else {
		atts = vs.AttPool.AggregatedAttestationsBySlotIndexElectra(ctx, req.Slot, req.CommitteeIndex)
		if len(atts) == 0 {
			atts = vs.AttPool.UnaggregatedAttestationsBySlotIndexElectra(ctx, req.Slot, req.CommitteeIndex)
		}
	}
	if len(atts) == 0 {
		return nil, status.Errorf(codes.NotFound, "Could not find attestation for slot and committee in pool")
	}

	best := bestAggregate(atts, req.CommitteeIndex, indexInCommittee)
	attAndProof := &ethpb.AggregateAttestationAndProofElectra{
		Aggregate:       best,
		SelectionProof:  req.SlotSignature,
		AggregatorIndex: validatorIndex,
	}
	return &ethpb.AggregateSelectionElectraResponse{AggregateAndProof: attAndProof}, nil
}

func (vs *Server) processAggregateSelection(ctx context.Context, req *ethpb.AggregateSelectionRequest) (uint64, primitives.ValidatorIndex, error) {
	if vs.SyncChecker.Syncing() {
		return 0, 0, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	// An optimistic validator MUST NOT participate in attestation
	// (i.e., sign across the DOMAIN_BEACON_ATTESTER, DOMAIN_SELECTION_PROOF or DOMAIN_AGGREGATE_AND_PROOF domains).
	if err := vs.optimisticStatus(ctx); err != nil {
		return 0, 0, err
	}

	st, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return 0, 0, status.Errorf(codes.Internal, "Could not determine head state: %v", err)
	}

	validatorIndex, exists := st.ValidatorIndexByPubkey(bytesutil.ToBytes48(req.PublicKey))
	if !exists {
		return 0, 0, status.Error(codes.Internal, "Could not locate validator index in DB")
	}

	epoch := slots.ToEpoch(req.Slot)
	activeValidatorIndices, err := helpers.ActiveValidatorIndices(ctx, st, epoch)
	if err != nil {
		return 0, 0, status.Errorf(codes.Internal, "Could not get validators: %v", err)
	}
	seed, err := helpers.Seed(st, epoch, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		return 0, 0, status.Errorf(codes.Internal, "Could not get seed: %v", err)
	}
	committee, err := helpers.BeaconCommittee(ctx, activeValidatorIndices, seed, req.Slot, req.CommitteeIndex)
	if err != nil {
		return 0, 0, err
	}

	// Check if the validator is an aggregator
	isAggregator, err := helpers.IsAggregator(uint64(len(committee)), req.SlotSignature)
	if err != nil {
		return 0, 0, status.Errorf(codes.Internal, "Could not get aggregator status: %v", err)
	}
	if !isAggregator {
		return 0, 0, status.Errorf(codes.InvalidArgument, "Validator is not an aggregator")
	}

	var indexInCommittee uint64
	for i, idx := range committee {
		if idx == validatorIndex {
			indexInCommittee = uint64(i)
		}
	}
	return indexInCommittee, validatorIndex, nil
}

// SubmitSignedAggregateSelectionProof is called by a validator to broadcast a signed
// aggregated and proof object.
func (vs *Server) SubmitSignedAggregateSelectionProof(
	ctx context.Context,
	req *ethpb.SignedAggregateSubmitRequest,
) (*ethpb.SignedAggregateSubmitResponse, error) {
	if err := vs.CoreService.SubmitSignedAggregateSelectionProof(ctx, req.SignedAggregateAndProof); err != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(err.Reason), "Could not submit aggregate: %v", err.Err)
	}
	return &ethpb.SignedAggregateSubmitResponse{}, nil
}

// SubmitSignedAggregateSelectionProofElectra is called by a validator to broadcast a signed
// aggregated and proof object.
func (vs *Server) SubmitSignedAggregateSelectionProofElectra(
	ctx context.Context,
	req *ethpb.SignedAggregateSubmitElectraRequest,
) (*ethpb.SignedAggregateSubmitResponse, error) {
	if err := vs.CoreService.SubmitSignedAggregateSelectionProof(ctx, req.SignedAggregateAndProof); err != nil {
		return nil, status.Errorf(core.ErrorReasonToGRPC(err.Reason), "Could not submit aggregate: %v", err.Err)
	}
	return &ethpb.SignedAggregateSubmitResponse{}, nil
}

func bestAggregate[T ethpb.Att](atts []T, committeeIndex primitives.CommitteeIndex, indexInCommittee uint64) T {
	best := atts[0]
	for _, a := range atts[1:] {
		// The aggregator should prefer an attestation that they have signed. We check this by
		// looking at the attestation's committee index against the validator's committee index
		// and check the aggregate bits to ensure the validator's index is set.
		if a.CommitteeBitsVal().BitAt(uint64(committeeIndex)) &&
			a.GetAggregationBits().BitAt(indexInCommittee) &&
			(!best.GetAggregationBits().BitAt(indexInCommittee) ||
				a.GetAggregationBits().Count() > best.GetAggregationBits().Count()) {
			best = a
		}

		// If the "best" still doesn't contain the validator's index, check the aggregation bits to
		// choose the attestation with the most bits set.
		if !best.GetAggregationBits().BitAt(indexInCommittee) &&
			a.GetAggregationBits().Count() > best.GetAggregationBits().Count() {
			best = a
		}
	}
	return best
}
