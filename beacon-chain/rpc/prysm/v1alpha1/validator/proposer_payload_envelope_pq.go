package validator

import (
	"bytes"
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type pqEnvelopeCache struct {
	mu        sync.Mutex
	envelope  *ethpb.ExecutionPayloadEnvelopePQ
	blobs     [][]byte
	kzgProofs [][]byte
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
	vs.pqEnvelopes.blobs, vs.pqEnvelopes.kzgProofs = nil, nil
	if bundle := local.BlobsBundler; bundle != nil {
		vs.pqEnvelopes.blobs, vs.pqEnvelopes.kzgProofs = bundle.GetBlobs(), bundle.GetProofs()
	}
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
	envelope, _, _, err := vs.ExecutionPayloadContentsPQ(req.Slot)
	return envelope, err
}

func (vs *Server) ExecutionPayloadContentsPQ(slot primitives.Slot) (*ethpb.ExecutionPayloadEnvelopePQ, [][]byte, [][]byte, error) {
	vs.pqEnvelopes.mu.Lock()
	defer vs.pqEnvelopes.mu.Unlock()
	envelope := vs.pqEnvelopes.envelope
	if envelope == nil || envelope.Payload.SlotNumber != slot {
		return nil, nil, nil, status.Errorf(codes.NotFound, "execution payload envelope not found for slot %d", slot)
	}
	return envelope, vs.pqEnvelopes.kzgProofs, vs.pqEnvelopes.blobs, nil
}

func (vs *Server) PublishExecutionPayloadEnvelopePQ(ctx context.Context, req *ethpb.SignedExecutionPayloadEnvelopePQ) (*emptypb.Empty, error) {
	if req == nil || req.Message == nil || req.Message.Payload == nil {
		return nil, status.Error(codes.InvalidArgument, "signed envelope or payload cannot be nil")
	}
	var kzgProofs, blobs [][]byte
	if cached, proofs, b, err := vs.ExecutionPayloadContentsPQ(req.Message.Payload.SlotNumber); err == nil && bytes.Equal(cached.BeaconBlockRoot, req.Message.BeaconBlockRoot) {
		kzgProofs, blobs = proofs, b
	}
	if err := vs.PublishExecutionPayloadEnvelopeContentsPQ(ctx, req, kzgProofs, blobs); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (vs *Server) PublishExecutionPayloadEnvelopeContentsPQ(ctx context.Context, req *ethpb.SignedExecutionPayloadEnvelopePQ, kzgProofs, blobs [][]byte) error {
	if req == nil || req.Message == nil || req.Message.Payload == nil {
		return status.Error(codes.InvalidArgument, "signed envelope or payload cannot be nil")
	}
	signed, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelopePQ(req)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "Invalid envelope: %v", err)
	}
	var verified []consensusblocks.VerifiedRODataColumn
	if len(blobs) > 0 {
		sidecars, partials, err := vs.sidecarsFromContents(blobs, kzgProofs, req.Message.Payload.SlotNumber, bytesutil.ToBytes32(req.Message.BeaconBlockRoot))
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "Invalid envelope contents: %v", err)
		}
		verified = make([]consensusblocks.VerifiedRODataColumn, 0, len(sidecars))
		for _, sidecar := range sidecars {
			verified = append(verified, consensusblocks.NewVerifiedRODataColumn(sidecar))
		}
		if err := vs.P2P.BroadcastDataColumnSidecars(ctx, verified, partials); err != nil {
			log.WithError(err).Error("Failed to broadcast PQ data column sidecars")
		}
	}
	if err := vs.P2P.Broadcast(ctx, req); err != nil {
		return status.Errorf(codes.Internal, "Could not broadcast envelope: %v", err)
	}
	// The DA check on import needs the columns first.
	if len(verified) > 0 {
		if err := vs.DataColumnReceiver.ReceiveDataColumns(verified); err != nil {
			return status.Errorf(codes.Internal, "Could not import data columns: %v", err)
		}
	}
	if err := vs.ExecutionPayloadEnvelopeReceiver.ReceiveExecutionPayloadEnvelope(ctx, signed); err != nil {
		return status.Errorf(codes.Internal, "Could not import envelope: %v", err)
	}
	return nil
}
