package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations/pqpool"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (vs *Server) GetAggregateAndProofPQ(ctx context.Context, req *ethpb.AggregateAndProofPQRequest) (*ethpb.AggregateAttestationAndProofPQ, error) {
	ctx, span := trace.StartSpan(ctx, "AggregatorServer.GetAggregateAndProofPQ")
	defer span.End()

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}
	st, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	ok, err := pq.IsAggregator(st, req.Slot, req.CommitteeIndex, req.AggregatorIndex)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Could not get aggregation committee: %v", err)
	}
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "Validator is not an aggregator")
	}
	aggregate, err := pqpool.BuildAggregate(ctx, st, vs.PQAttPool.All(), req.Slot, req.CommitteeIndex)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not build aggregate: %v", err)
	}
	if aggregate == nil {
		return nil, status.Error(codes.NotFound, "No attestations to aggregate")
	}
	return &ethpb.AggregateAttestationAndProofPQ{
		AggregatorIndex: req.AggregatorIndex,
		Aggregate:       aggregate.Att,
		Proof:           aggregate.Proof,
	}, nil
}

func (vs *Server) SubmitSignedAggregateAndProofPQ(ctx context.Context, signed *ethpb.SignedAggregateAttestationAndProofPQ) (*ethpb.SignedAggregateSubmitResponse, error) {
	ctx, span := trace.StartSpan(ctx, "AggregatorServer.SubmitSignedAggregateAndProofPQ")
	defer span.End()

	if signed == nil || signed.Message == nil || signed.Message.Aggregate == nil || signed.Message.Aggregate.Data == nil ||
		signed.Message.Aggregate.Data.Source == nil || signed.Message.Aggregate.Data.Target == nil {
		return nil, status.Error(codes.InvalidArgument, "Incomplete aggregate")
	}
	aggregate := signed.Message.Aggregate
	data := aggregate.Data
	if slots.ToEpoch(data.Slot) != data.Target.Epoch {
		return nil, status.Error(codes.InvalidArgument, "Aggregate slot does not match the target epoch")
	}
	if aggregate.CommitteeBits.Count() != 1 {
		return nil, status.Error(codes.InvalidArgument, "Aggregate must cover one committee")
	}
	targetState, err := vs.AttestationStateFetcher.AttestationTargetState(ctx, data.Target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get target state: %v", err)
	}
	if err := pq.VerifyAggregateAndProof(ctx, targetState, signed); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid aggregate: %v", err)
	}
	root, err := data.HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get attestation root: %v", err)
	}
	if err := vs.P2P.Broadcast(ctx, signed); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast aggregate: %v", err)
	}
	if _, err := vs.PQAttPool.SaveAggregate(aggregate, signed.Message.Proof); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not save aggregate: %v", err)
	}
	return &ethpb.SignedAggregateSubmitResponse{AttestationDataRoot: root[:]}, nil
}

// The REST aggregate endpoint names the data root, not the aggregator.
func (vs *Server) AggregateAttestationPQ(ctx context.Context, slot primitives.Slot, index primitives.CommitteeIndex, dataRoot [32]byte) (*ethpb.AttestationPQ, []byte, error) {
	st, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	aggregate, err := pqpool.BuildAggregate(ctx, st, vs.PQAttPool.All(), slot, index)
	if err != nil {
		return nil, nil, status.Errorf(codes.Internal, "Could not build aggregate: %v", err)
	}
	if aggregate == nil {
		return nil, nil, status.Error(codes.NotFound, "No attestations to aggregate")
	}
	root, err := aggregate.Att.Data.HashTreeRoot()
	if err != nil {
		return nil, nil, status.Errorf(codes.Internal, "Could not get attestation root: %v", err)
	}
	if root != dataRoot {
		return nil, nil, status.Error(codes.NotFound, "No matching aggregate found")
	}
	return aggregate.Att, aggregate.Proof, nil
}
