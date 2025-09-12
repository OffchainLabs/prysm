package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
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

func (c *beaconApiValidatorClient) proposeBeaconBlock(ctx context.Context, in *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
	var res *blockProcessingResult
	var err error
	switch blockType := in.Block.(type) {
	case *ethpb.GenericSignedBeaconBlock_Phase0:
		res, err = handlePhase0Block(blockType)
	case *ethpb.GenericSignedBeaconBlock_Altair:
		res, err = handleAltairBlock(blockType)
	case *ethpb.GenericSignedBeaconBlock_Bellatrix:
		res, err = handleBellatrixBlock(blockType)
	case *ethpb.GenericSignedBeaconBlock_BlindedBellatrix:
		res, err = handleBlindedBellatrixBlock(blockType)
	case *ethpb.GenericSignedBeaconBlock_Capella:
		res, err = handleCapellaBlock(blockType)
	case *ethpb.GenericSignedBeaconBlock_BlindedCapella:
		res, err = handleBlindedCapellaBlock(blockType)
	case *ethpb.GenericSignedBeaconBlock_Deneb:
		res, err = handleDenebBlockContents(blockType)
	case *ethpb.GenericSignedBeaconBlock_BlindedDeneb:
		res, err = handleBlindedDenebBlock(blockType)
	case *ethpb.GenericSignedBeaconBlock_Electra:
		res, err = handleElectraBlockContents(blockType)
	case *ethpb.GenericSignedBeaconBlock_BlindedElectra:
		res, err = handleBlindedElectraBlock(blockType)
	case *ethpb.GenericSignedBeaconBlock_Fulu:
		res, err = handleFuluBlockContents(blockType)
	case *ethpb.GenericSignedBeaconBlock_BlindedFulu:
		res, err = handleBlindedFuluBlock(blockType)
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
		_, _, err = c.jsonRestHandler.PostSSZ(ctx, endpoint, headers, bytes.NewBuffer(res.marshalledSSZ))
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
				err = c.jsonRestHandler.Post(ctx, endpoint, headers, bytes.NewBuffer(jsonData), nil)
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
		err = c.jsonRestHandler.Post(ctx, endpoint, headers, bytes.NewBuffer(jsonData), nil)
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

func handlePhase0Block(block *ethpb.GenericSignedBeaconBlock_Phase0) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "phase0"
	res.blinded = false

	beaconBlockRoot, err := block.Phase0.Block.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for phase0 beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.Phase0.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize block for phase0 beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock := structs.SignedBeaconBlockPhase0FromConsensus(block.Phase0)
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleAltairBlock(block *ethpb.GenericSignedBeaconBlock_Altair) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "altair"
	res.blinded = false

	beaconBlockRoot, err := block.Altair.Block.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for altair beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.Altair.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize block for altair beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock := structs.SignedBeaconBlockAltairFromConsensus(block.Altair)
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleBellatrixBlock(block *ethpb.GenericSignedBeaconBlock_Bellatrix) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "bellatrix"
	res.blinded = false

	beaconBlockRoot, err := block.Bellatrix.Block.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for bellatrix beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.Bellatrix.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize block for bellatrix beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock, err := structs.SignedBeaconBlockBellatrixFromConsensus(block.Bellatrix)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert bellatrix beacon block")
		}
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleBlindedBellatrixBlock(block *ethpb.GenericSignedBeaconBlock_BlindedBellatrix) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "bellatrix"
	res.blinded = true

	beaconBlockRoot, err := block.BlindedBellatrix.Block.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for bellatrix beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.BlindedBellatrix.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize block for bellatrix beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock, err := structs.SignedBlindedBeaconBlockBellatrixFromConsensus(block.BlindedBellatrix)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert blinded bellatrix beacon block")
		}
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleCapellaBlock(block *ethpb.GenericSignedBeaconBlock_Capella) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "capella"
	res.blinded = false

	beaconBlockRoot, err := block.Capella.Block.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for capella beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.Capella.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize capella beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock, err := structs.SignedBeaconBlockCapellaFromConsensus(block.Capella)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert capella beacon block")
		}
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleBlindedCapellaBlock(block *ethpb.GenericSignedBeaconBlock_BlindedCapella) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "capella"
	res.blinded = true

	beaconBlockRoot, err := block.BlindedCapella.Block.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for blinded capella beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.BlindedCapella.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize blinded capella beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock, err := structs.SignedBlindedBeaconBlockCapellaFromConsensus(block.BlindedCapella)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert blinded capella beacon block")
		}
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleDenebBlockContents(block *ethpb.GenericSignedBeaconBlock_Deneb) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "deneb"
	res.blinded = false

	beaconBlockRoot, err := block.Deneb.Block.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for deneb beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.Deneb.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize deneb beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock, err := structs.SignedBeaconBlockContentsDenebFromConsensus(block.Deneb)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert deneb beacon block contents")
		}
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleBlindedDenebBlock(block *ethpb.GenericSignedBeaconBlock_BlindedDeneb) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "deneb"
	res.blinded = true

	beaconBlockRoot, err := block.BlindedDeneb.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for deneb blinded beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.BlindedDeneb.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize blinded deneb  beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock, err := structs.SignedBlindedBeaconBlockDenebFromConsensus(block.BlindedDeneb)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert deneb blinded beacon block")
		}
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleElectraBlockContents(block *ethpb.GenericSignedBeaconBlock_Electra) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "electra"
	res.blinded = false

	beaconBlockRoot, err := block.Electra.Block.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for electra beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.Electra.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize electra beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock, err := structs.SignedBeaconBlockContentsElectraFromConsensus(block.Electra)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert electra beacon block contents")
		}
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleBlindedElectraBlock(block *ethpb.GenericSignedBeaconBlock_BlindedElectra) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "electra"
	res.blinded = true

	beaconBlockRoot, err := block.BlindedElectra.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for electra blinded beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.BlindedElectra.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize blinded electra beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock, err := structs.SignedBlindedBeaconBlockElectraFromConsensus(block.BlindedElectra)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert electra blinded beacon block")
		}
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleFuluBlockContents(block *ethpb.GenericSignedBeaconBlock_Fulu) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "fulu"
	res.blinded = false

	beaconBlockRoot, err := block.Fulu.Block.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for fulu beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.Fulu.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize fulu beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock, err := structs.SignedBeaconBlockContentsFuluFromConsensus(block.Fulu)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert fulu beacon block contents")
		}
		return json.Marshal(signedBlock)
	}

	return &res, nil
}

func handleBlindedFuluBlock(block *ethpb.GenericSignedBeaconBlock_BlindedFulu) (*blockProcessingResult, error) {
	var res blockProcessingResult
	res.consensusVersion = "fulu"
	res.blinded = true

	beaconBlockRoot, err := block.BlindedFulu.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute block root for fulu blinded beacon block")
	}
	res.beaconBlockRoot = beaconBlockRoot

	// Marshal SSZ
	ssz, err := block.BlindedFulu.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize blinded fulu beacon block")
	}
	res.marshalledSSZ = ssz

	// Set up JSON marshalling function for fallback
	res.marshalJSON = func() ([]byte, error) {
		signedBlock, err := structs.SignedBlindedBeaconBlockFuluFromConsensus(block.BlindedFulu)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert fulu blinded beacon block")
		}
		return json.Marshal(signedBlock)
	}

	return &res, nil
}
