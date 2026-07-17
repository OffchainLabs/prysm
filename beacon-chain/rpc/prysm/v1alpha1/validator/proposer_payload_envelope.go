package validator

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/logs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// storeExecutionPayloadEnvelope creates and caches the execution payload envelope
// after the block is fully built (state root set), returning the envelope for the caller to bundle.
func (vs *Server) storeExecutionPayloadEnvelope(
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
) (*ethpb.ExecutionPayloadEnvelope, error) {
	blockRoot, err := sBlk.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block hash tree root")
	}

	payload := extractExecutionPayloadGloas(local)

	parentRoot := sBlk.Block().ParentRoot()
	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload:               payload,
		ExecutionRequests:     local.ExecutionRequestsGloas,
		BuilderIndex:          params.BeaconConfig().BuilderIndexSelfBuild,
		BeaconBlockRoot:       blockRoot[:],
		ParentBeaconBlockRoot: parentRoot[:],
	}

	// Precompute sidecars here (during ProposeBeaconBlock slack) so publish stays fast.
	var roSidecars []consensusblocks.RODataColumn
	if bundle := local.BlobsBundler; bundle != nil && len(bundle.GetBlobs()) > 0 {
		cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(bundle.GetBlobs(), bundle.GetProofs())
		if err != nil {
			return nil, errors.Wrap(err, "compute cells and proofs from blobs bundle")
		}
		roSidecars, err = peerdas.DataColumnSidecarsGloas(cellsPerBlob, proofsPerBlob, sBlk.Block().Slot(), blockRoot)
		if err != nil {
			return nil, errors.Wrap(err, "build gloas data column sidecars")
		}
	}

	vs.ExecutionPayloadEnvelopeCache.Set(&cache.ExecutionPayloadContents{
		Envelope:    envelope,
		DataColumns: roSidecars,
	})
	return envelope, nil
}

func extractExecutionPayloadGloas(local *consensusblocks.GetPayloadResponse) *enginev1.ExecutionPayloadGloas {
	if local == nil || local.ExecutionData == nil || local.ExecutionData.IsNil() {
		return nil
	}
	if p, ok := local.ExecutionData.Proto().(*enginev1.ExecutionPayloadGloas); ok {
		return p
	}
	return nil
}

// GetExecutionPayloadEnvelope returns the cached execution payload envelope for the requested
// slot so the proposer can sign and publish it.
func (vs *Server) GetExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.ExecutionPayloadEnvelopeRequest,
) (*ethpb.ExecutionPayloadEnvelopeResponse, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.GetExecutionPayloadEnvelope")
	defer span.End()

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	span.SetAttributes(trace.StringAttribute("slot", fmt.Sprintf("%d", req.Slot)))

	if slots.ToEpoch(req.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload envelopes are not supported before Gloas fork (slot %d)", req.Slot)
	}

	contents, ok := vs.ExecutionPayloadEnvelopeCache.Contents()
	if !ok || contents.Envelope.Payload.SlotNumber != req.Slot {
		return nil, status.Errorf(codes.NotFound,
			"execution payload envelope not found for slot %d", req.Slot)
	}

	return &ethpb.ExecutionPayloadEnvelopeResponse{
		Envelope: contents.Envelope,
	}, nil
}

// PublishExecutionPayloadEnvelope validates and broadcasts a signed execution payload envelope,
// called by validators after signing the envelope from GetExecutionPayloadEnvelope.
func (vs *Server) PublishExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.GenericSignedExecutionPayloadEnvelope,
) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.PublishExecutionPayloadEnvelope")
	defer span.End()
	start := time.Now()

	signed, blobs, kzgProofs, err := vs.resolveEnvelopeToPublish(req)
	if err != nil {
		return nil, err
	}

	envSlot := primitives.Slot(signed.Message.Payload.SlotNumber)
	if slots.ToEpoch(envSlot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload envelopes are not supported before Gloas fork (slot %d)", envSlot)
	}

	beaconBlockRoot := bytesutil.ToBytes32(signed.Message.BeaconBlockRoot)
	span.SetAttributes(
		trace.StringAttribute("slot", fmt.Sprintf("%d", envSlot)),
		trace.StringAttribute("builderIndex", fmt.Sprintf("%d", signed.Message.BuilderIndex)),
		trace.StringAttribute("beaconBlockRoot", fmt.Sprintf("%#x", beaconBlockRoot[:8])),
	)

	log := log.WithFields(logrus.Fields{
		"slot":            envSlot,
		"builderIndex":    logs.BuilderIndexLabel(signed.Message.BuilderIndex),
		"beaconBlockRoot": fmt.Sprintf("%#x", beaconBlockRoot[:8]),
	})
	log.Debug("Publishing execution payload envelope")

	// KZG verification stays synchronous, never gossip unverified sidecars.
	var sidecars []consensusblocks.RODataColumn
	if len(blobs) > 0 {
		sidecars, err = vs.sidecarsFromContents(blobs, kzgProofs, envSlot, beaconBlockRoot)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid execution payload envelope contents: %v", err)
		}
	} else if cached, ok := vs.ExecutionPayloadEnvelopeCache.Contents(); ok && cached.Envelope.Payload.SlotNumber == envSlot {
		sidecars = cached.DataColumns
	}

	roSigned, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(signed)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not wrap signed envelope: %v", err)
	}

	// Locally built or verified above, safe to upgrade.
	verifiedSidecars := make([]consensusblocks.VerifiedRODataColumn, 0, len(sidecars))
	for _, sidecar := range sidecars {
		verifiedSidecars = append(verifiedSidecars, consensusblocks.NewVerifiedRODataColumn(sidecar))
	}
	if len(verifiedSidecars) > 0 {
		if err := vs.P2P.BroadcastDataColumnSidecars(ctx, verifiedSidecars, nil); err != nil {
			log.WithError(err).Error("Failed to broadcast Gloas data column sidecars")
		}
	}

	if err := vs.P2P.Broadcast(ctx, signed); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to broadcast execution payload envelope: %v", err)
	}

	// Import in the background so the reveal is not delayed past the PTC deadline.
	go vs.importPublishedEnvelope(log, verifiedSidecars, roSigned)

	log.WithField("duration", time.Since(start)).Info("Published execution payload envelope")

	return &emptypb.Empty{}, nil
}

// Sidecars first, the DA check needs them.
func (vs *Server) importPublishedEnvelope(log *logrus.Entry, sidecars []consensusblocks.VerifiedRODataColumn, signed interfaces.ROSignedExecutionPayloadEnvelope) {
	start := time.Now()
	if len(sidecars) > 0 {
		if err := vs.DataColumnReceiver.ReceiveDataColumns(sidecars); err != nil {
			log.WithError(err).Error("Failed to receive data columns for published envelope")
		}
	}
	if err := vs.ExecutionPayloadEnvelopeReceiver.ReceiveExecutionPayloadEnvelope(vs.Ctx, signed); err != nil {
		log.WithError(err).Error("Failed to import published execution payload envelope")
		return
	}
	log.WithField("duration", time.Since(start)).Debug("Imported published execution payload envelope")
}

// resolveEnvelopeToPublish extracts the signed envelope plus any caller-supplied blobs. The stateful
// signed_envelope arm must match the cached envelope so its precomputed data columns apply.
func (vs *Server) resolveEnvelopeToPublish(req *ethpb.GenericSignedExecutionPayloadEnvelope) (*ethpb.SignedExecutionPayloadEnvelope, [][]byte, [][]byte, error) {
	switch {
	case req.GetContents() != nil:
		c := req.GetContents()
		if c.SignedExecutionPayloadEnvelope == nil || c.SignedExecutionPayloadEnvelope.Message == nil ||
			c.SignedExecutionPayloadEnvelope.Message.Payload == nil {
			return nil, nil, nil, status.Error(codes.InvalidArgument, "signed envelope or payload cannot be nil")
		}
		return c.SignedExecutionPayloadEnvelope, c.Blobs, c.KzgProofs, nil
	case req.GetSignedEnvelope() != nil:
		signed := req.GetSignedEnvelope()
		if signed.Message == nil || signed.Message.Payload == nil {
			return nil, nil, nil, status.Error(codes.InvalidArgument, "signed envelope or payload cannot be nil")
		}
		cached, ok := vs.ExecutionPayloadEnvelopeCache.Contents()
		if !ok || cached.Envelope == nil {
			return nil, nil, nil, status.Error(codes.FailedPrecondition, "envelope without blob data was submitted but the beacon node has no cached blobs and KZG proofs")
		}
		cachedRoot, err := cached.Envelope.HashTreeRoot()
		if err != nil {
			return nil, nil, nil, status.Errorf(codes.Internal, "could not hash cached envelope: %v", err)
		}
		submittedRoot, err := signed.Message.HashTreeRoot()
		if err != nil {
			return nil, nil, nil, status.Errorf(codes.Internal, "could not hash submitted envelope: %v", err)
		}
		if cachedRoot != submittedRoot {
			return nil, nil, nil, status.Error(codes.InvalidArgument, "cached execution payload envelope does not match submitted envelope")
		}
		return signed, nil, nil, nil
	default:
		return nil, nil, nil, status.Error(codes.InvalidArgument, "generic signed execution payload envelope must set contents or signed_envelope")
	}
}

// sidecarsFromContents verifies caller-supplied blobs+KZG proofs (stateless publish) and builds the
// data column sidecars for the slot. Verification matters because broadcastAndReceiveDataColumns
// upgrades the sidecars to "verified" without re-checking.
func (vs *Server) sidecarsFromContents(blobs, kzgProofs [][]byte, slot primitives.Slot, blockRoot [32]byte) ([]consensusblocks.RODataColumn, error) {
	if err := verifyCellProofs(blobs, kzgProofs); err != nil {
		return nil, errors.Wrap(err, "kzg verification failed")
	}
	cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(blobs, kzgProofs)
	if err != nil {
		return nil, errors.Wrap(err, "compute cells and proofs")
	}
	return peerdas.DataColumnSidecarsGloas(cellsPerBlob, proofsPerBlob, slot, blockRoot)
}

// verifyCellProofs batch-verifies cell proofs against commitments derived from the blobs.
func verifyCellProofs(blobs [][]byte, flatProofs [][]byte) error {
	commitments := make([][]byte, len(blobs))
	for i, blob := range blobs {
		if len(blob) != kzg.BytesPerBlob {
			return errors.Errorf("blob %d has wrong size %d", i, len(blob))
		}
		var b kzg.Blob
		copy(b[:], blob)
		c, err := kzg.BlobToKZGCommitment(&b)
		if err != nil {
			return errors.Wrapf(err, "compute kzg commitment for blob %d", i)
		}
		commitments[i] = c[:]
	}
	return kzg.VerifyCellKZGProofBatchFromBlobData(blobs, commitments, flatProofs, fieldparams.NumberOfColumns)
}

// setParentExecutionRequests populates the parent_execution_requests field
// in the block body based on the parent's execution payload envelope.
func (vs *Server) setParentExecutionRequests(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, parentFull bool) error {
	if head.Version() < version.Gloas {
		return sBlk.SetParentExecutionRequests(&enginev1.ExecutionRequestsGloas{})
	}

	parentRoot := sBlk.Block().ParentRoot()
	parentSlot, err := vs.ForkchoiceFetcher.RecentBlockSlot(parentRoot)
	if err != nil {
		return errors.Wrap(err, "could not get parent block slot")
	}
	if slots.ToEpoch(parentSlot) < params.BeaconConfig().GloasForkEpoch || !parentFull {
		return sBlk.SetParentExecutionRequests(&enginev1.ExecutionRequestsGloas{})
	}

	// TODO: replace DB lookup with a single-entry cache (blockroot → envelope).
	signedEnvelope, err := vs.BeaconDB.ExecutionPayloadEnvelope(ctx, parentRoot)
	if err != nil {
		return errors.Wrap(err, "could not get parent execution payload envelope")
	}
	return sBlk.SetParentExecutionRequests(signedEnvelope.Message.ExecutionRequests)
}
