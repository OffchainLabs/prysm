package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

type blockProcessingResult struct {
	consensusVersion string
	beaconBlockRoot  [32]byte
	marshalledSSZ    []byte
	blinded          bool
	// Function to marshal JSON on demand
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

	marshaledSSZ, err := sszObj.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to serialize %s beacon block", versionName)
	}

	return &blockProcessingResult{
		consensusVersion: versionName,
		blinded:          blinded,
		beaconBlockRoot:  beaconBlockRoot,
		marshalledSSZ:    marshaledSSZ,
		marshalJSON:      jsonFn,
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

	// Try PostSSZ first with SSZ data
	if res.marshalledSSZ != nil {
		_, _, err = c.handler.PostSSZ(ctx, endpoint, headers, bytes.NewBuffer(res.marshalledSSZ))
		if err != nil {
			errJson := &httputil.DefaultJsonError{}
			// If PostSSZ fails with 406 (Not Acceptable), fall back to JSON
			if !errors.As(err, &errJson) {
				return nil, err
			}
			if errJson.Code == http.StatusNotAcceptable && res.marshalJSON != nil {
				log.WithError(err).Warn("Failed to submit block ssz, falling back to JSON")
				jsonData, jsonErr := res.marshalJSON()
				if jsonErr != nil {
					return nil, errors.Wrap(jsonErr, "failed to marshal JSON")
				}
				// Reset headers for JSON
				err = c.handler.Post(ctx, endpoint, headers, bytes.NewBuffer(jsonData), nil)
				// If JSON also fails, return that error
				if err != nil {
					return nil, errors.Wrap(err, "failed to submit block via JSON fallback")
				}
			} else {
				// For non-406 errors or when no JSON fallback is available, return the SSZ error
				return nil, errors.Wrap(errJson, "failed to submit block ssz")
			}
		}
	} else if res.marshalJSON == nil {
		return nil, errors.New("no marshalling functions available")
	} else {
		// No SSZ data available, marshal and use JSON
		jsonData, jsonErr := res.marshalJSON()
		if jsonErr != nil {
			return nil, errors.Wrap(jsonErr, "failed to marshal JSON")
		}
		// Reset headers for JSON
		err = c.handler.Post(ctx, endpoint, headers, bytes.NewBuffer(jsonData), nil)
		errJson := &httputil.DefaultJsonError{}
		if err != nil {
			if !errors.As(err, &errJson) {
				return nil, err
			}
			// Error 202 means that the block was successfully broadcast, but validation failed
			if errJson.Code == http.StatusAccepted {
				return nil, errors.New("block was successfully broadcast but failed validation")
			}
			return nil, errJson
		}
	}

	return &ethpb.ProposeResponse{BlockRoot: res.beaconBlockRoot[:]}, nil
}
