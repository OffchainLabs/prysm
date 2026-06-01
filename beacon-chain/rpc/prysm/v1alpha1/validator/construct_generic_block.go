package validator

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// constructGenericBeaconBlock constructs a `GenericBeaconBlock` based on the block version and other parameters.
func (vs *Server) constructGenericBeaconBlock(
	sBlk interfaces.SignedBeaconBlock,
	blobsBundler enginev1.BlobsBundler,
	winningBid primitives.Wei,
	eagerPayloadStateRoot bool,
	local *blocks.GetPayloadResponse,
) (*ethpb.GenericBeaconBlock, error) {
	if sBlk == nil || sBlk.Block() == nil {
		return nil, errors.New("block cannot be nil")
	}

	blockProto, err := sBlk.Block().Proto()
	if err != nil {
		return nil, err
	}

	isBlinded := sBlk.IsBlinded()
	bidStr := primitives.WeiToBigInt(winningBid).String()

	switch sBlk.Version() {
	case version.Phase0:
		return vs.constructPhase0Block(blockProto), nil
	case version.Altair:
		return vs.constructAltairBlock(blockProto), nil
	case version.Bellatrix:
		return vs.constructBellatrixBlock(blockProto, isBlinded, bidStr), nil
	case version.Capella:
		return vs.constructCapellaBlock(blockProto, isBlinded, bidStr), nil
	case version.Deneb:
		bundle, ok := blobsBundler.(*enginev1.BlobsBundle)
		if blobsBundler != nil && !ok {
			return nil, fmt.Errorf("expected *BlobsBundler, got %T", blobsBundler)
		}
		return vs.constructDenebBlock(blockProto, isBlinded, bidStr, bundle), nil
	case version.Electra:
		bundle, ok := blobsBundler.(*enginev1.BlobsBundle)
		if blobsBundler != nil && !ok {
			return nil, fmt.Errorf("expected *BlobsBundler, got %T", blobsBundler)
		}
		return vs.constructElectraBlock(blockProto, isBlinded, bidStr, bundle), nil
	case version.Fulu:
		bundle, ok := blobsBundler.(*enginev1.BlobsBundleV2)
		if blobsBundler != nil && !ok {
			return nil, fmt.Errorf("expected *BlobsBundleV2, got %T", blobsBundler)
		}
		return vs.constructFuluBlock(blockProto, isBlinded, bidStr, bundle), nil
	case version.Gloas:
		// Gloas blocks do not carry a separate payload value — the bid is part of the block body.
		gloasBlock := blockProto.(*ethpb.BeaconBlockGloas)
		// Stateless (eager) self-build: bundle the cached envelope + blobs/proofs so the validator
		// client has everything to publish Contents without a second round-trip.
		if eagerPayloadStateRoot {
			contents := vs.gloasBlockContents(gloasBlock, local)
			if contents != nil {
				return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_GloasContents{GloasContents: contents}}, nil
			}
		}
		return &ethpb.GenericBeaconBlock{
			Block: &ethpb.GenericBeaconBlock_Gloas{Gloas: gloasBlock},
		}, nil
	default:
		return nil, fmt.Errorf("unknown block version: %d", sBlk.Version())
	}
}

// gloasBlockContents bundles the cached self-build envelope with the raw blobs and KZG proofs
// from the local payload. Returns nil when there is no cached self-build envelope (e.g. the block
// used an external builder bid), in which case the caller returns the block-only form.
func (vs *Server) gloasBlockContents(block *ethpb.BeaconBlockGloas, local *blocks.GetPayloadResponse) *ethpb.BeaconBlockContentsGloas {
	cached, ok := vs.ExecutionPayloadEnvelopeCache.Contents()
	if !ok || cached.Envelope == nil {
		return nil
	}
	var blobs, kzgProofs [][]byte
	if local != nil && local.BlobsBundler != nil {
		blobs = local.BlobsBundler.GetBlobs()
		kzgProofs = local.BlobsBundler.GetProofs()
	}
	return &ethpb.BeaconBlockContentsGloas{
		Block:                    block,
		ExecutionPayloadEnvelope: cached.Envelope,
		KzgProofs:                kzgProofs,
		Blobs:                    blobs,
	}
}

// Helper functions for constructing blocks for each version
func (vs *Server) constructPhase0Block(pb proto.Message) *ethpb.GenericBeaconBlock {
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Phase0{Phase0: pb.(*ethpb.BeaconBlock)}}
}

func (vs *Server) constructAltairBlock(pb proto.Message) *ethpb.GenericBeaconBlock {
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Altair{Altair: pb.(*ethpb.BeaconBlockAltair)}}
}

func (vs *Server) constructBellatrixBlock(pb proto.Message, isBlinded bool, payloadValue string) *ethpb.GenericBeaconBlock {
	if isBlinded {
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: pb.(*ethpb.BlindedBeaconBlockBellatrix)}, IsBlinded: true, PayloadValue: payloadValue}
	}
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Bellatrix{Bellatrix: pb.(*ethpb.BeaconBlockBellatrix)}, IsBlinded: false, PayloadValue: payloadValue}
}

func (vs *Server) constructCapellaBlock(pb proto.Message, isBlinded bool, payloadValue string) *ethpb.GenericBeaconBlock {
	if isBlinded {
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedCapella{BlindedCapella: pb.(*ethpb.BlindedBeaconBlockCapella)}, IsBlinded: true, PayloadValue: payloadValue}
	}
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Capella{Capella: pb.(*ethpb.BeaconBlockCapella)}, IsBlinded: false, PayloadValue: payloadValue}
}

func (vs *Server) constructDenebBlock(blockProto proto.Message, isBlinded bool, payloadValue string, bundle *enginev1.BlobsBundle) *ethpb.GenericBeaconBlock {
	if isBlinded {
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedDeneb{BlindedDeneb: blockProto.(*ethpb.BlindedBeaconBlockDeneb)}, IsBlinded: true, PayloadValue: payloadValue}
	}
	denebContents := &ethpb.BeaconBlockContentsDeneb{Block: blockProto.(*ethpb.BeaconBlockDeneb)}
	if bundle != nil {
		denebContents.KzgProofs = bundle.Proofs
		denebContents.Blobs = bundle.Blobs
	}
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Deneb{Deneb: denebContents}, IsBlinded: false, PayloadValue: payloadValue}
}

func (vs *Server) constructElectraBlock(blockProto proto.Message, isBlinded bool, payloadValue string, bundle *enginev1.BlobsBundle) *ethpb.GenericBeaconBlock {
	if isBlinded {
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedElectra{BlindedElectra: blockProto.(*ethpb.BlindedBeaconBlockElectra)}, IsBlinded: true, PayloadValue: payloadValue}
	}
	electraContents := &ethpb.BeaconBlockContentsElectra{Block: blockProto.(*ethpb.BeaconBlockElectra)}
	if bundle != nil {
		electraContents.KzgProofs = bundle.Proofs
		electraContents.Blobs = bundle.Blobs
	}
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Electra{Electra: electraContents}, IsBlinded: false, PayloadValue: payloadValue}
}

func (vs *Server) constructFuluBlock(blockProto proto.Message, isBlinded bool, payloadValue string, bundle *enginev1.BlobsBundleV2) *ethpb.GenericBeaconBlock {
	if isBlinded {
		return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_BlindedFulu{BlindedFulu: blockProto.(*ethpb.BlindedBeaconBlockFulu)}, IsBlinded: true, PayloadValue: payloadValue}
	}
	fuluContents := &ethpb.BeaconBlockContentsFulu{Block: blockProto.(*ethpb.BeaconBlockElectra)}
	if bundle != nil {
		fuluContents.KzgProofs = bundle.Proofs
		fuluContents.Blobs = bundle.Blobs
	}
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Fulu{Fulu: fuluContents}, IsBlinded: false, PayloadValue: payloadValue}
}
