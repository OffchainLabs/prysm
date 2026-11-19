package validator

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type pqEnvelopeCache struct {
	mu       sync.Mutex
	envelope *ethpb.ExecutionPayloadEnvelopePQ
}

func executionRequestsPQ(local *consensusblocks.GetPayloadResponse) *enginev1.ExecutionRequestsPQ {
	if local.ExecutionRequestsPQ == nil {
		return &enginev1.ExecutionRequestsPQ{}
	}
	return local.ExecutionRequestsPQ
}

func (vs *Server) storeExecutionPayloadEnvelopePQ(sBlk interfaces.SignedBeaconBlock, local *consensusblocks.GetPayloadResponse) error {
	blockRoot, err := sBlk.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute block hash tree root")
	}
	parentRoot := sBlk.Block().ParentRoot()
	vs.pqEnvelopes.mu.Lock()
	defer vs.pqEnvelopes.mu.Unlock()
	vs.pqEnvelopes.envelope = &ethpb.ExecutionPayloadEnvelopePQ{
		Payload:               extractExecutionPayloadGloas(local),
		ExecutionRequests:     executionRequestsPQ(local),
		BuilderIndex:          params.BeaconConfig().BuilderIndexSelfBuild,
		BeaconBlockRoot:       blockRoot[:],
		ParentBeaconBlockRoot: parentRoot[:],
	}
	return nil
}

func (vs *Server) parentExecutionRequestsPQ(ctx context.Context, parentRoot [32]byte, parentFull bool) (*enginev1.ExecutionRequestsPQ, error) {
	if !parentFull {
		return &enginev1.ExecutionRequestsPQ{}, nil
	}
	signedEnvelope, err := vs.BeaconDB.ExecutionPayloadEnvelopePQ(ctx, parentRoot)
	if err != nil {
		return nil, errors.Wrap(err, "could not get parent execution payload envelope")
	}
	return signedEnvelope.Message.ExecutionRequests, nil
}

func (vs *Server) GetExecutionPayloadEnvelopePQ(_ context.Context, req *ethpb.ExecutionPayloadEnvelopeRequest) (*ethpb.ExecutionPayloadEnvelopePQ, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	vs.pqEnvelopes.mu.Lock()
	defer vs.pqEnvelopes.mu.Unlock()
	envelope := vs.pqEnvelopes.envelope
	if envelope == nil || envelope.Payload.SlotNumber != req.Slot {
		return nil, status.Errorf(codes.NotFound, "execution payload envelope not found for slot %d", req.Slot)
	}
	return envelope, nil
}

func (vs *Server) PublishExecutionPayloadEnvelopePQ(ctx context.Context, req *ethpb.SignedExecutionPayloadEnvelopePQ) (*emptypb.Empty, error) {
	signed, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelopePQ(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid envelope: %v", err)
	}
	if err := vs.P2P.Broadcast(ctx, req); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast envelope: %v", err)
	}
	if err := vs.ExecutionPayloadEnvelopeReceiver.ReceiveExecutionPayloadEnvelope(ctx, signed); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not import envelope: %v", err)
	}
	return &emptypb.Empty{}, nil
}
