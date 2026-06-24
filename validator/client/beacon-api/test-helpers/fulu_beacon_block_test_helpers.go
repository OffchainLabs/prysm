package test_helpers

import (
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func GenerateProtoFuluBeaconBlockContents() *ethpb.BeaconBlockContentsFulu {
	electra := GenerateProtoElectraBeaconBlockContents()
	return &ethpb.BeaconBlockContentsFulu{
		Block:     electra.Block,
		KzgProofs: electra.KzgProofs,
		Blobs:     electra.Blobs,
	}
}

func GenerateProtoBlindedFuluBeaconBlock() *ethpb.BlindedBeaconBlockFulu {
	electra := GenerateProtoBlindedElectraBeaconBlock()
	return &ethpb.BlindedBeaconBlockFulu{
		Slot:          electra.Slot,
		ProposerIndex: electra.ProposerIndex,
		ParentRoot:    electra.ParentRoot,
		StateRoot:     electra.StateRoot,
		Body:          electra.Body,
	}
}

func GenerateJsonFuluBeaconBlockContents() *structs.BeaconBlockContentsFulu {
	electra := GenerateJsonElectraBeaconBlockContents()
	return &structs.BeaconBlockContentsFulu{
		Block:     electra.Block,
		KzgProofs: electra.KzgProofs,
		Blobs:     electra.Blobs,
	}
}

func GenerateJsonBlindedFuluBeaconBlock() *structs.BlindedBeaconBlockFulu {
	electra := GenerateJsonBlindedElectraBeaconBlock()
	return &structs.BlindedBeaconBlockFulu{
		Slot:          electra.Slot,
		ProposerIndex: electra.ProposerIndex,
		ParentRoot:    electra.ParentRoot,
		StateRoot:     electra.StateRoot,
		Body:          electra.Body,
	}
}
