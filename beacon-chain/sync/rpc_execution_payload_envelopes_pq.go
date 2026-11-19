package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/pkg/errors"
)

func isPQSlot(slot primitives.Slot) bool {
	return slots.ToEpoch(slot) >= params.BeaconConfig().PQForkEpoch
}

// The shared DB reader drops the PQ requests and signature, so the full envelope is rebuilt from the PQ record.
func (s *Service) writeExecutionPayloadEnvelopePQChunk(ctx context.Context, stream libp2pcore.Stream, root [32]byte, payload *enginev1.ExecutionPayloadGloas) error {
	blinded, err := s.cfg.beaconDB.ExecutionPayloadEnvelopePQ(ctx, root)
	if err != nil {
		return err
	}
	envelope := &ethpb.SignedExecutionPayloadEnvelopePQ{
		Message: &ethpb.ExecutionPayloadEnvelopePQ{
			Payload:               payload,
			ExecutionRequests:     blinded.Message.ExecutionRequests,
			BuilderIndex:          blinded.Message.BuilderIndex,
			BeaconBlockRoot:       blinded.Message.BeaconBlockRoot,
			ParentBeaconBlockRoot: blinded.Message.ParentBeaconBlockRoot,
		},
		Signature: blinded.Signature,
	}
	return writeEnvelopeChunk(stream, s.cfg.p2p.Encoding(), primitives.Slot(payload.SlotNumber), envelope)
}

func readChunkedROExecutionPayloadEnvelope(
	stream network.Stream,
	encoding encoder.NetworkEncoding,
	ctxMap ContextByteVersions,
) (interfaces.ROSignedExecutionPayloadEnvelope, error) {
	v, err := readEnvelopeChunkVersion(stream, encoding, ctxMap)
	if err != nil {
		return nil, err
	}
	if v >= version.PQ {
		envelope := &ethpb.SignedExecutionPayloadEnvelopePQ{}
		if err := encoding.DecodeWithMaxLength(stream, envelope); err != nil {
			return nil, errors.Wrap(err, "failed to decode execution payload envelope from RPC chunk stream")
		}
		return blocks.WrappedROSignedExecutionPayloadEnvelopePQ(envelope)
	}
	envelope := &ethpb.SignedExecutionPayloadEnvelope{}
	if err := encoding.DecodeWithMaxLength(stream, envelope); err != nil {
		return nil, errors.Wrap(err, "failed to decode execution payload envelope from RPC chunk stream")
	}
	return blocks.WrappedROSignedExecutionPayloadEnvelope(envelope)
}
