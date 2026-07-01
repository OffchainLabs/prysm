package validator

import (
	"context"
	"fmt"

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

	// Precompute partial columns too, when enabled, so partial-column peers can fill in
	// cells. Gloas sidecars carry no inline commitments, so seed them from the bid before
	// building the partials.
	var partialColumns []consensusblocks.PartialDataColumn
	if len(roSidecars) > 0 && vs.ExecutionEngineCaller.PartialColumnsSupported() {
		commitments, err := sBlk.Block().Body().BlobKzgCommitments()
		if err != nil {
			return nil, errors.Wrap(err, "blob kzg commitments")
		}
		partialColumns, err = partialColumnsFromSidecars(roSidecars, commitments)
		if err != nil {
			return nil, err
		}
	}

	vs.ExecutionPayloadEnvelopeCache.Set(&cache.ExecutionPayloadContents{
		Envelope:       envelope,
		DataColumns:    roSidecars,
		PartialColumns: partialColumns,
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

// GetExecutionPayloadEnvelope implements the gRPC endpoint:
// /eth/v1alpha1/validator/execution_payload_envelope/{slot}/{builder_index}
// It returns the stored execution payload envelope for a slot/builder and, for
// self-build envelopes, computes the post-payload state root on demand.
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

	// Return the blinded wire form (payload_root); the signer validates over its HTR, which equals
	// the full envelope's HTR, and the BN reconstructs the full payload from this cache on publish.
	blinded, err := contents.Envelope.WireBlinded()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not build blinded envelope: %v", err)
	}
	return &ethpb.ExecutionPayloadEnvelopeResponse{
		Blinded: blinded,
	}, nil
}

// PublishExecutionPayloadEnvelope validates and broadcasts a signed execution payload envelope.
// This is called by validators after signing the envelope retrieved from GetExecutionPayloadEnvelope.
//
// gRPC endpoint: POST /eth/v1alpha1/validator/execution_payload_envelope
func (vs *Server) PublishExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.GenericSignedExecutionPayloadEnvelope,
) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.PublishExecutionPayloadEnvelope")
	defer span.End()

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
		"builderIndex":    signed.Message.BuilderIndex,
		"beaconBlockRoot": fmt.Sprintf("%#x", beaconBlockRoot[:8]),
	})
	log.Info("Publishing signed execution payload envelope")

	// Broadcast sidecars BEFORE receiving the envelope so the DA check sees them. Stateless publishes
	// carry blobs+proofs (this node may not have them cached); stateful publishes rely on the cache.
	var sidecars []consensusblocks.RODataColumn
	var partialColumns []consensusblocks.PartialDataColumn
	if len(blobs) > 0 {
		sidecars, partialColumns, err = vs.sidecarsFromContents(blobs, kzgProofs, envSlot, beaconBlockRoot)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid execution payload envelope contents: %v", err)
		}
	} else if cached, ok := vs.ExecutionPayloadEnvelopeCache.Contents(); ok && cached.Envelope.Payload.SlotNumber == envSlot {
		sidecars = cached.DataColumns
		partialColumns = cached.PartialColumns
	}
	if len(sidecars) > 0 {
		log.WithFields(logrus.Fields{
			"columns":  len(sidecars),
			"partials": len(partialColumns),
		}).Debug("Broadcasting Gloas data column sidecars")
		if err := vs.broadcastAndReceiveDataColumns(ctx, sidecars, partialColumns); err != nil {
			log.WithError(err).Error("Failed to broadcast Gloas data column sidecars")
		}
	}

	if err := vs.P2P.Broadcast(ctx, signed); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to broadcast execution payload envelope: %v", err)
	}

	roSigned, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(signed)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not wrap signed envelope: %v", err)
	}
	if err := vs.ExecutionPayloadEnvelopeReceiver.ReceiveExecutionPayloadEnvelope(ctx, roSigned); err != nil {
		// Broadcast already succeeded; import failed. REST maps Aborted -> 202 (beacon-APIs #580).
		return nil, status.Errorf(codes.Aborted, "failed to receive execution payload envelope: %v", err)
	}

	log.Info("Successfully published execution payload envelope")

	return &emptypb.Empty{}, nil
}

// resolveEnvelopeToPublish turns the generic publish request into the full signed envelope plus any
// caller-supplied blobs. The blinded (stateful) arm reconstructs the full envelope from the cache by
// matching beacon_block_root; the contents (stateless) arm carries everything in the request.
func (vs *Server) resolveEnvelopeToPublish(req *ethpb.GenericSignedExecutionPayloadEnvelope) (*ethpb.SignedExecutionPayloadEnvelope, [][]byte, [][]byte, error) {
	switch {
	case req.GetContents() != nil:
		c := req.GetContents()
		if c.SignedExecutionPayloadEnvelope == nil || c.SignedExecutionPayloadEnvelope.Message == nil ||
			c.SignedExecutionPayloadEnvelope.Message.Payload == nil {
			return nil, nil, nil, status.Error(codes.InvalidArgument, "signed envelope or payload cannot be nil")
		}
		return c.SignedExecutionPayloadEnvelope, c.Blobs, c.KzgProofs, nil
	case req.GetBlinded() != nil:
		b := req.GetBlinded()
		if b.Message == nil {
			return nil, nil, nil, status.Error(codes.InvalidArgument, "blinded envelope message cannot be nil")
		}
		cached, ok := vs.ExecutionPayloadEnvelopeCache.Contents()
		if !ok || cached.Envelope == nil {
			return nil, nil, nil, status.Error(codes.FailedPrecondition, "no cached execution payload envelope to reconstruct from")
		}
		cachedBlinded, err := cached.Envelope.WireBlinded()
		if err != nil {
			return nil, nil, nil, status.Errorf(codes.Internal, "could not derive blinded envelope from cache: %v", err)
		}
		cachedRoot, err := cachedBlinded.HashTreeRoot()
		if err != nil {
			return nil, nil, nil, status.Errorf(codes.Internal, "could not hash cached blinded envelope: %v", err)
		}
		blindedRoot, err := b.Message.HashTreeRoot()
		if err != nil {
			return nil, nil, nil, status.Errorf(codes.Internal, "could not hash blinded envelope: %v", err)
		}
		if cachedRoot != blindedRoot {
			return nil, nil, nil, status.Error(codes.InvalidArgument, "cached envelope does not match blinded envelope")
		}
		return &ethpb.SignedExecutionPayloadEnvelope{Message: cached.Envelope, Signature: b.Signature}, nil, nil, nil
	default:
		return nil, nil, nil, status.Error(codes.InvalidArgument, "generic signed execution payload envelope must set contents or blinded")
	}
}

// sidecarsFromContents verifies caller-supplied blobs+KZG proofs (stateless publish) and builds the
// data column sidecars for the slot, plus partial columns when partial-column support is enabled so
// partial-column peers can fill in cells. Verification matters because broadcastAndReceiveDataColumns
// upgrades the sidecars to "verified" without re-checking.
func (vs *Server) sidecarsFromContents(blobs, kzgProofs [][]byte, slot primitives.Slot, blockRoot [32]byte) ([]consensusblocks.RODataColumn, []consensusblocks.PartialDataColumn, error) {
	commitments, err := verifyCellProofs(blobs, kzgProofs)
	if err != nil {
		return nil, nil, errors.Wrap(err, "kzg verification failed")
	}
	cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(blobs, kzgProofs)
	if err != nil {
		return nil, nil, errors.Wrap(err, "compute cells and proofs")
	}
	sidecars, err := peerdas.DataColumnSidecarsGloas(cellsPerBlob, proofsPerBlob, slot, blockRoot)
	if err != nil {
		return nil, nil, err
	}

	// Gloas sidecars carry no inline commitments; seed them from the commitments derived from the
	// supplied blobs so partial-column peers can request cells against them.
	var partialColumns []consensusblocks.PartialDataColumn
	if vs.ExecutionEngineCaller.PartialColumnsSupported() {
		partialColumns, err = partialColumnsFromSidecars(sidecars, commitments)
		if err != nil {
			return nil, nil, err
		}
	}
	return sidecars, partialColumns, nil
}

// verifyCellProofs derives the KZG commitment for each blob and batch-verifies the cell proofs
// against them, returning the commitments so callers can seed Gloas sidecars (which carry none inline).
func verifyCellProofs(blobs, flatProofs [][]byte) ([][]byte, error) {
	commitments := make([][]byte, len(blobs))
	for i, blob := range blobs {
		if len(blob) != kzg.BytesPerBlob {
			return nil, errors.Errorf("blob %d has wrong size %d", i, len(blob))
		}
		var b kzg.Blob
		copy(b[:], blob)
		c, err := kzg.BlobToKZGCommitment(&b)
		if err != nil {
			return nil, errors.Wrapf(err, "compute kzg commitment for blob %d", i)
		}
		commitments[i] = c[:]
	}
	if err := kzg.VerifyCellKZGProofBatchFromBlobData(blobs, commitments, flatProofs, fieldparams.NumberOfColumns); err != nil {
		return nil, err
	}
	return commitments, nil
}

// partialColumnsFromSidecars seeds each sidecar with the bid commitments and wraps it into a
// fully-included partial column so partial-column peers can request individual cells. It is the
// single construction path shared by the self-build and stateless publish flows so both compute
// identical (deterministic) group ids.
func partialColumnsFromSidecars(sidecars []consensusblocks.RODataColumn, commitments [][]byte) ([]consensusblocks.PartialDataColumn, error) {
	partialColumns := make([]consensusblocks.PartialDataColumn, 0, len(sidecars))
	for i := range sidecars {
		sidecars[i].SetBidCommitments(commitments)
		pc, err := consensusblocks.NewPartialDataColumnFromVerifiedRODataColumn(consensusblocks.NewVerifiedRODataColumn(sidecars[i]))
		if err != nil {
			return nil, errors.Wrap(err, "partial column from verified ro data column")
		}
		partialColumns = append(partialColumns, pc)
	}
	return partialColumns, nil
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
