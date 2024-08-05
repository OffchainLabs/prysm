package types

import (
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/wrapper"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/metadata"
)

func init() {
	// Initialize data maps.
	InitializeDataMaps()
}

// This file provides a mapping of fork version to the respective data type. This is
// to allow any service to appropriately use the correct data type with a provided
// fork-version.

var (
	// BlockMap maps the fork-version to the underlying data type for that
	// particular fork period.
	BlockMap map[[4]byte]func() (interfaces.ReadOnlySignedBeaconBlock, error)
	// MetaDataMap maps the fork-version to the underlying data type for that
	// particular fork period.
	MetaDataMap map[[4]byte]func() (metadata.Metadata, error)
	// AttestationMap maps the fork-version to the underlying data type for that
	// particular fork period.
	AttestationMap map[[4]byte]func() (ethpb.Att, error)
	// AggregateAttestationMap maps the fork-version to the underlying data type for that
	// particular fork period.
	AggregateAttestationMap map[[4]byte]func() (ethpb.SignedAggregateAttAndProof, error)
)

// InitializeDataMaps initializes all the relevant object maps. This function is called to
// reset maps and reinitialize them.
func InitializeDataMaps() {
	// Reset our block map.
	BlockMap = map[[4]byte]func() (interfaces.ReadOnlySignedBeaconBlock, error){
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlock{Block: &ethpb.BeaconBlock{Body: &ethpb.BeaconBlockBody{}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockAltair{Block: &ethpb.BeaconBlockAltair{Body: &ethpb.BeaconBlockBodyAltair{}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockBellatrix{Block: &ethpb.BeaconBlockBellatrix{Body: &ethpb.BeaconBlockBodyBellatrix{ExecutionPayload: &enginev1.ExecutionPayload{}}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockCapella{Block: &ethpb.BeaconBlockCapella{Body: &ethpb.BeaconBlockBodyCapella{ExecutionPayload: &enginev1.ExecutionPayloadCapella{}}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockDeneb{Block: &ethpb.BeaconBlockDeneb{Body: &ethpb.BeaconBlockBodyDeneb{ExecutionPayload: &enginev1.ExecutionPayloadDeneb{}}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockElectra{Block: &ethpb.BeaconBlockElectra{Body: &ethpb.BeaconBlockBodyElectra{ExecutionPayload: &enginev1.ExecutionPayloadElectra{}}}},
			)
		},
	}

	// Reset our metadata map.
	MetaDataMap = map[[4]byte]func() (metadata.Metadata, error){
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV0(&ethpb.MetaDataV0{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV2(&ethpb.MetaDataV2{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV2(&ethpb.MetaDataV2{}), nil
		},
	}

	// Reset our attestation map.
	AttestationMap = map[[4]byte]func() (ethpb.Att, error){
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() (ethpb.Att, error) {
			return &ethpb.Attestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (ethpb.Att, error) {
			return &ethpb.Attestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (ethpb.Att, error) {
			return &ethpb.Attestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (ethpb.Att, error) {
			return &ethpb.Attestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (ethpb.Att, error) {
			return &ethpb.Attestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (ethpb.Att, error) {
			return &ethpb.AttestationElectra{}, nil
		},
	}

	// Reset our aggregate attestation map.
	AggregateAttestationMap = map[[4]byte]func() (ethpb.SignedAggregateAttAndProof, error){
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProof{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProof{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProof{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProof{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProof{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProofElectra{}, nil
		},
	}
}
