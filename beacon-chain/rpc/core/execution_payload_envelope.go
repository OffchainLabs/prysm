package core

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/logs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// GetExecutionPayloadEnvelope returns the cached execution payload envelope for the requested slot.
func (s *Service) GetExecutionPayloadEnvelope(ctx context.Context, req *ethpb.ExecutionPayloadEnvelopeRequest) (*ethpb.ExecutionPayloadEnvelope, *RpcError) {
	_, span := trace.StartSpan(ctx, "ProposerServer.GetExecutionPayloadEnvelope")
	defer span.End()

	if req == nil {
		return nil, &RpcError{Reason: BadRequest, Err: errors.New("request cannot be nil")}
	}
	span.SetAttributes(trace.StringAttribute("slot", fmt.Sprintf("%d", req.Slot)))

	if slots.ToEpoch(req.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, &RpcError{Reason: BadRequest, Err: errors.Errorf("execution payload envelopes are not supported before Gloas fork (slot %d)", req.Slot)}
	}

	contents, ok := s.ExecutionPayloadEnvelopeCache.Contents()
	if !ok || contents.Envelope.Payload.SlotNumber != req.Slot {
		return nil, &RpcError{Reason: NotFound, Err: errors.Errorf("execution payload envelope not found for slot %d", req.Slot)}
	}
	return contents.Envelope, nil
}

// PublishExecutionPayloadEnvelope validates, broadcasts, and imports a signed execution payload envelope.
func (s *Service) PublishExecutionPayloadEnvelope(ctx context.Context, req *ethpb.GenericSignedExecutionPayloadEnvelope) *RpcError {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.PublishExecutionPayloadEnvelope")
	defer span.End()
	start := time.Now()

	signed, blobs, kzgProofs, rpcErr := s.resolveEnvelopeToPublish(req)
	if rpcErr != nil {
		return rpcErr
	}

	envSlot := primitives.Slot(signed.Message.Payload.SlotNumber)
	if slots.ToEpoch(envSlot) < params.BeaconConfig().GloasForkEpoch {
		return &RpcError{Reason: BadRequest, Err: errors.Errorf("execution payload envelopes are not supported before Gloas fork (slot %d)", envSlot)}
	}

	beaconBlockRoot := bytesutil.ToBytes32(signed.Message.BeaconBlockRoot)
	span.SetAttributes(
		trace.StringAttribute("slot", fmt.Sprintf("%d", envSlot)),
		trace.StringAttribute("builderIndex", fmt.Sprintf("%d", signed.Message.BuilderIndex)),
		trace.StringAttribute("beaconBlockRoot", fmt.Sprintf("%#x", beaconBlockRoot[:8])),
	)

	entry := log.WithFields(logrus.Fields{
		"slot":            envSlot,
		"builderIndex":    logs.BuilderIndexLabel(signed.Message.BuilderIndex),
		"beaconBlockRoot": fmt.Sprintf("%#x", beaconBlockRoot[:8]),
	})
	entry.Debug("Publishing execution payload envelope")

	var (
		sidecars []consensusblocks.RODataColumn
		err      error
	)

	if len(blobs) > 0 {
		sidecars, err = sidecarsFromEnvelopeContents(blobs, kzgProofs, envSlot, beaconBlockRoot)
		if err != nil {
			return &RpcError{Reason: BadRequest, Err: errors.Wrap(err, "invalid execution payload envelope contents")}
		}
	} else if cached, ok := s.ExecutionPayloadEnvelopeCache.Contents(); ok && cached.Envelope.Payload.SlotNumber == envSlot {
		sidecars = cached.DataColumns
	}

	roSigned, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(signed)
	if err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not wrap signed envelope")}
	}

	verifiedSidecars := make([]consensusblocks.VerifiedRODataColumn, 0, len(sidecars))
	for _, sidecar := range sidecars {
		verifiedSidecars = append(verifiedSidecars, consensusblocks.NewVerifiedRODataColumn(sidecar))
	}
	if len(verifiedSidecars) > 0 {
		if err := s.P2P.BroadcastDataColumnSidecars(ctx, verifiedSidecars, nil); err != nil {
			entry.WithError(err).Error("Failed to broadcast Gloas data column sidecars")
		}
	}

	if err := s.P2P.Broadcast(ctx, signed); err != nil {
		return &RpcError{Reason: Internal, Err: errors.Wrap(err, "failed to broadcast execution payload envelope")}
	}

	go s.importPublishedEnvelope(entry, verifiedSidecars, roSigned)

	entry.WithField("duration", time.Since(start)).Info("Published execution payload envelope")
	return nil
}

// Sidecars first, the DA check needs them.
func (s *Service) importPublishedEnvelope(entry *logrus.Entry, sidecars []consensusblocks.VerifiedRODataColumn, signed interfaces.ROSignedExecutionPayloadEnvelope) {
	start := time.Now()
	if len(sidecars) > 0 {
		if err := s.DataColumnReceiver.ReceiveDataColumns(sidecars); err != nil {
			entry.WithError(err).Error("Failed to receive data columns for published envelope")
		}
	}
	if err := s.ExecutionPayloadEnvelopeReceiver.ReceiveExecutionPayloadEnvelope(s.Ctx, signed); err != nil {
		entry.WithError(err).Error("Failed to import published execution payload envelope")
		return
	}
	entry.WithField("duration", time.Since(start)).Debug("Imported published execution payload envelope")
}

func (s *Service) resolveEnvelopeToPublish(req *ethpb.GenericSignedExecutionPayloadEnvelope) (*ethpb.SignedExecutionPayloadEnvelope, [][]byte, [][]byte, *RpcError) {
	switch {
	case req.GetContents() != nil:
		contents := req.GetContents()
		if contents.SignedExecutionPayloadEnvelope == nil || contents.SignedExecutionPayloadEnvelope.Message == nil ||
			contents.SignedExecutionPayloadEnvelope.Message.Payload == nil {
			return nil, nil, nil, &RpcError{Reason: BadRequest, Err: errors.New("signed envelope or payload cannot be nil")}
		}
		return contents.SignedExecutionPayloadEnvelope, contents.Blobs, contents.KzgProofs, nil
	case req.GetSignedEnvelope() != nil:
		signed := req.GetSignedEnvelope()
		if signed.Message == nil || signed.Message.Payload == nil {
			return nil, nil, nil, &RpcError{Reason: BadRequest, Err: errors.New("signed envelope or payload cannot be nil")}
		}
		cached, ok := s.ExecutionPayloadEnvelopeCache.Contents()
		if !ok || cached.Envelope == nil {
			return nil, nil, nil, &RpcError{Reason: FailedPrecondition, Err: errors.New("envelope without blob data was submitted but the beacon node has no cached blobs and KZG proofs")}
		}
		cachedRoot, err := cached.Envelope.HashTreeRoot()
		if err != nil {
			return nil, nil, nil, &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not hash cached envelope")}
		}
		submittedRoot, err := signed.Message.HashTreeRoot()
		if err != nil {
			return nil, nil, nil, &RpcError{Reason: Internal, Err: errors.Wrap(err, "could not hash submitted envelope")}
		}
		if cachedRoot != submittedRoot {
			return nil, nil, nil, &RpcError{Reason: BadRequest, Err: errors.New("cached execution payload envelope does not match submitted envelope")}
		}
		return signed, nil, nil, nil
	default:
		return nil, nil, nil, &RpcError{Reason: BadRequest, Err: errors.New("generic signed execution payload envelope must set contents or signed_envelope")}
	}
}

func sidecarsFromEnvelopeContents(blobs, kzgProofs [][]byte, slot primitives.Slot, blockRoot [32]byte) ([]consensusblocks.RODataColumn, error) {
	if err := verifyEnvelopeCellProofs(blobs, kzgProofs); err != nil {
		return nil, errors.Wrap(err, "kzg verification failed")
	}
	cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(blobs, kzgProofs)
	if err != nil {
		return nil, errors.Wrap(err, "compute cells and proofs")
	}
	return peerdas.DataColumnSidecarsGloas(cellsPerBlob, proofsPerBlob, slot, blockRoot)
}

func verifyEnvelopeCellProofs(blobs [][]byte, flatProofs [][]byte) error {
	commitments := make([][]byte, len(blobs))
	for i, blob := range blobs {
		if len(blob) != kzg.BytesPerBlob {
			return errors.Errorf("blob %d has wrong size %d", i, len(blob))
		}
		var b kzg.Blob
		copy(b[:], blob)
		commitment, err := kzg.BlobToKZGCommitment(&b)
		if err != nil {
			return errors.Wrapf(err, "compute kzg commitment for blob %d", i)
		}
		commitments[i] = commitment[:]
	}
	return kzg.VerifyCellKZGProofBatchFromBlobData(blobs, commitments, flatProofs, fieldparams.NumberOfColumns)
}
