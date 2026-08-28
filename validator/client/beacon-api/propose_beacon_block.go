package beacon_api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

type blockProcessingResult struct {
	consensusVersion string
	beaconBlockRoot  [32]byte
	blinded          bool
	// Functions to marshal on demand
	marshalSSZ  func() ([]byte, error)
	marshalJSON func() ([]byte, error)
}

type sszMarshaler interface {
	MarshalSSZ() ([]byte, error)
}

func buildBlockResult(
	versionName string,
	blinded bool,
	sszObj sszMarshaler,
	rootObj ssz.Hashable,
	jsonFn func() ([]byte, error),
) (*blockProcessingResult, error) {
	beaconBlockRoot, err := rootObj.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compute block root for %s beacon block", versionName)
	}

	return &blockProcessingResult{
		consensusVersion: versionName,
		blinded:          blinded,
		beaconBlockRoot:  beaconBlockRoot,
		marshalSSZ: func() ([]byte, error) {
			b, err := sszObj.MarshalSSZ()
			if err != nil {
				return nil, fmt.Errorf("failed to serialize %s beacon block: %w", versionName, err)
			}
			return b, nil
		},
		marshalJSON: jsonFn,
	}, nil
}

func (c *beaconApiValidatorClient) proposeBeaconBlock(ctx context.Context, in *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
	var res *blockProcessingResult
	var err error
	switch blockType := in.Block.(type) {
	case *ethpb.GenericSignedBeaconBlock_Phase0:
		res, err = buildBlockResult("phase0", false, blockType.Phase0, blockType.Phase0.Block, func() ([]byte, error) {
			return json.Marshal(structs.SignedBeaconBlockPhase0FromConsensus(blockType.Phase0))
		})
	case *ethpb.GenericSignedBeaconBlock_Altair:
		res, err = buildBlockResult("altair", false, blockType.Altair, blockType.Altair.Block, func() ([]byte, error) {
			return json.Marshal(structs.SignedBeaconBlockAltairFromConsensus(blockType.Altair))
		})
	case *ethpb.GenericSignedBeaconBlock_Bellatrix:
		res, err = buildBlockResult("bellatrix", false, blockType.Bellatrix, blockType.Bellatrix.Block, func() ([]byte, error) {
			signedBlock, err := structs.SignedBeaconBlockBellatrixFromConsensus(blockType.Bellatrix)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert bellatrix beacon block")
			}
			return json.Marshal(signedBlock)
		})
	case *ethpb.GenericSignedBeaconBlock_BlindedBellatrix:
		res, err = buildBlockResult("bellatrix", true, blockType.BlindedBellatrix, blockType.BlindedBellatrix.Block, func() ([]byte, error) {
			signedBlock, err := structs.SignedBlindedBeaconBlockBellatrixFromConsensus(blockType.BlindedBellatrix)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert blinded bellatrix beacon block")
			}
			return json.Marshal(signedBlock)
		})
	case *ethpb.GenericSignedBeaconBlock_Capella:
		res, err = buildBlockResult("capella", false, blockType.Capella, blockType.Capella.Block, func() ([]byte, error) {
			signedBlock, err := structs.SignedBeaconBlockCapellaFromConsensus(blockType.Capella)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert capella beacon block")
			}
			return json.Marshal(signedBlock)
		})
	case *ethpb.GenericSignedBeaconBlock_BlindedCapella:
		res, err = buildBlockResult("capella", true, blockType.BlindedCapella, blockType.BlindedCapella.Block, func() ([]byte, error) {
			signedBlock, err := structs.SignedBlindedBeaconBlockCapellaFromConsensus(blockType.BlindedCapella)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert blinded capella beacon block")
			}
			return json.Marshal(signedBlock)
		})
	case *ethpb.GenericSignedBeaconBlock_Deneb:
		res, err = buildBlockResult("deneb", false, blockType.Deneb, blockType.Deneb.Block, func() ([]byte, error) {
			signedBlock, err := structs.SignedBeaconBlockContentsDenebFromConsensus(blockType.Deneb)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert deneb beacon block contents")
			}
			return json.Marshal(signedBlock)
		})
	case *ethpb.GenericSignedBeaconBlock_BlindedDeneb:
		res, err = buildBlockResult("deneb", true, blockType.BlindedDeneb, blockType.BlindedDeneb, func() ([]byte, error) {
			signedBlock, err := structs.SignedBlindedBeaconBlockDenebFromConsensus(blockType.BlindedDeneb)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert deneb blinded beacon block")
			}
			return json.Marshal(signedBlock)
		})
	case *ethpb.GenericSignedBeaconBlock_Electra:
		res, err = buildBlockResult("electra", false, blockType.Electra, blockType.Electra.Block, func() ([]byte, error) {
			signedBlock, err := structs.SignedBeaconBlockContentsElectraFromConsensus(blockType.Electra)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert electra beacon block contents")
			}
			return json.Marshal(signedBlock)
		})
	case *ethpb.GenericSignedBeaconBlock_BlindedElectra:
		res, err = buildBlockResult("electra", true, blockType.BlindedElectra, blockType.BlindedElectra, func() ([]byte, error) {
			signedBlock, err := structs.SignedBlindedBeaconBlockElectraFromConsensus(blockType.BlindedElectra)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert electra blinded beacon block")
			}
			return json.Marshal(signedBlock)
		})
	case *ethpb.GenericSignedBeaconBlock_Fulu:
		res, err = buildBlockResult("fulu", false, blockType.Fulu, blockType.Fulu.Block, func() ([]byte, error) {
			signedBlock, err := structs.SignedBeaconBlockContentsFuluFromConsensus(blockType.Fulu)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert fulu beacon block contents")
			}
			return json.Marshal(signedBlock)
		})
	case *ethpb.GenericSignedBeaconBlock_BlindedFulu:
		res, err = buildBlockResult("fulu", true, blockType.BlindedFulu, blockType.BlindedFulu, func() ([]byte, error) {
			signedBlock, err := structs.SignedBlindedBeaconBlockFuluFromConsensus(blockType.BlindedFulu)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert fulu blinded beacon block")
			}
			return json.Marshal(signedBlock)
		})
	case *ethpb.GenericSignedBeaconBlock_Gloas:
		res, err = buildBlockResult("gloas", false, blockType.Gloas, blockType.Gloas.Block, func() ([]byte, error) {
			signedBlock, err := structs.SignedBeaconBlockGloasFromConsensus(blockType.Gloas)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert gloas beacon block")
			}
			return json.Marshal(signedBlock)
		})
	default:
		return nil, errors.Errorf("unsupported block type %T", in.Block)
	}

	if err != nil {
		return nil, err
	}

	endpoint := "/eth/v2/beacon/blocks"

	if res.blinded {
		endpoint = "/eth/v2/beacon/blinded_blocks"
	}

	headers := map[string]string{"Eth-Consensus-Version": res.consensusVersion}

	if err := c.handler.PostSSZWithFallback(ctx, endpoint, headers, res.marshalSSZ, res.marshalJSON); err != nil {
		return nil, fmt.Errorf("failed to submit block: %w", err)
	}

	return &ethpb.ProposeResponse{BlockRoot: res.beaconBlockRoot[:]}, nil
}
