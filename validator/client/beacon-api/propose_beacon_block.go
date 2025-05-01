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
	marshalledJSON   []byte
	blinded          bool
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
	err = c.jsonRestHandler.Post(ctx, endpoint, headers, bytes.NewBuffer(res.marshalledJSON), nil)
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

	signedBlock := structs.SignedBeaconBlockPhase0FromConsensus(block.Phase0)
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall phase0 beacon block to json")
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

	signedBlock := structs.SignedBeaconBlockAltairFromConsensus(block.Altair)
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall altair beacon block to json")
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

	signedBlock, err := structs.SignedBeaconBlockBellatrixFromConsensus(block.Bellatrix)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall bellatrix beacon block")
	}
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall bellatrix beacon block to json")
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

	signedBlock, err := structs.SignedBlindedBeaconBlockBellatrixFromConsensus(block.BlindedBellatrix)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall blinded bellatrix beacon block")
	}
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall blinded bellatrix beacon block to json")
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

	signedBlock, err := structs.SignedBeaconBlockCapellaFromConsensus(block.Capella)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall capella beacon block")
	}
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall capella beacon block to json")
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

	signedBlock, err := structs.SignedBlindedBeaconBlockCapellaFromConsensus(block.BlindedCapella)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall blinded capella beacon block")
	}
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall blinded capella beacon block to json")
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

	signedBlock, err := structs.SignedBeaconBlockContentsDenebFromConsensus(block.Deneb)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall deneb beacon block contents")
	}
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall deneb beacon block contents to json")
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

	signedBlock, err := structs.SignedBlindedBeaconBlockDenebFromConsensus(block.BlindedDeneb)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall deneb blinded beacon block ")
	}
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall deneb blinded beacon block to json")
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

	signedBlock, err := structs.SignedBeaconBlockContentsElectraFromConsensus(block.Electra)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall electra beacon block contents")
	}
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall electra beacon block contents to json")
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

	signedBlock, err := structs.SignedBlindedBeaconBlockElectraFromConsensus(block.BlindedElectra)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall electra blinded beacon block")
	}
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall electra blinded beacon block to json")
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

	signedBlock, err := structs.SignedBeaconBlockContentsFuluFromConsensus(block.Fulu)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall fulu beacon block contents")
	}
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall fulu beacon block contents to json")
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

	signedBlock, err := structs.SignedBlindedBeaconBlockFuluFromConsensus(block.BlindedFulu)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall fulu blinded beacon block")
	}
	res.marshalledJSON, err = json.Marshal(signedBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshall fulu blinded beacon block to json")
	}

	return &res, nil
}
