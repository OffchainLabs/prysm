package types

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	lightclientConsensusTypes "github.com/OffchainLabs/prysm/v6/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v6/consensus-types/wrapper"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/metadata"
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
	// AttesterSlashingMap maps the fork-version to the underlying data type for that particular
	// fork period.
	AttesterSlashingMap map[[4]byte]func() (ethpb.AttSlashing, error)
	// LightClientOptimisticUpdateMap maps the fork-version to the underlying data type for that
	// particular fork period.
	LightClientOptimisticUpdateMap map[[4]byte]func() (interfaces.LightClientOptimisticUpdate, error)
	// LightClientFinalityUpdateMap maps the fork-version to the underlying data type for that
	// particular fork period.
	LightClientFinalityUpdateMap map[[4]byte]func() (interfaces.LightClientFinalityUpdate, error)
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
				&ethpb.SignedBeaconBlockElectra{Block: &ethpb.BeaconBlockElectra{Body: &ethpb.BeaconBlockBodyElectra{ExecutionPayload: &enginev1.ExecutionPayloadDeneb{}}}},
			)
		},
		bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion): func() (interfaces.ReadOnlySignedBeaconBlock, error) {
			return blocks.NewSignedBeaconBlock(
				&ethpb.SignedBeaconBlockFulu{Block: &ethpb.BeaconBlockElectra{Body: &ethpb.BeaconBlockBodyElectra{ExecutionPayload: &enginev1.ExecutionPayloadDeneb{}}}},
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
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (metadata.Metadata, error) {
			return wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion): func() (metadata.Metadata, error) {
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
			return &ethpb.SingleAttestation{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion): func() (ethpb.Att, error) {
			return &ethpb.SingleAttestation{}, nil
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
		bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion): func() (ethpb.SignedAggregateAttAndProof, error) {
			return &ethpb.SignedAggregateAttestationAndProofElectra{}, nil
		},
	}

	// Reset our aggregate attestation map.
	AttesterSlashingMap = map[[4]byte]func() (ethpb.AttSlashing, error){
		bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion): func() (ethpb.AttSlashing, error) {
			return &ethpb.AttesterSlashing{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (ethpb.AttSlashing, error) {
			return &ethpb.AttesterSlashing{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (ethpb.AttSlashing, error) {
			return &ethpb.AttesterSlashing{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (ethpb.AttSlashing, error) {
			return &ethpb.AttesterSlashing{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (ethpb.AttSlashing, error) {
			return &ethpb.AttesterSlashing{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (ethpb.AttSlashing, error) {
			return &ethpb.AttesterSlashingElectra{}, nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().FuluForkVersion): func() (ethpb.AttSlashing, error) {
			return &ethpb.AttesterSlashingElectra{}, nil
		},
	}

	// Reset our light client optimistic update map.
	LightClientOptimisticUpdateMap = map[[4]byte]func() (interfaces.LightClientOptimisticUpdate, error){
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (interfaces.LightClientOptimisticUpdate, error) {
			return lightclientConsensusTypes.NewEmptyOptimisticUpdateAltair(), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (interfaces.LightClientOptimisticUpdate, error) {
			return lightclientConsensusTypes.NewEmptyOptimisticUpdateAltair(), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (interfaces.LightClientOptimisticUpdate, error) {
			return lightclientConsensusTypes.NewEmptyOptimisticUpdateCapella(), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (interfaces.LightClientOptimisticUpdate, error) {
			return lightclientConsensusTypes.NewEmptyOptimisticUpdateDeneb(), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (interfaces.LightClientOptimisticUpdate, error) {
			return lightclientConsensusTypes.NewEmptyOptimisticUpdateDeneb(), nil
		},
	}

	// Reset our light client finality update map.
	LightClientFinalityUpdateMap = map[[4]byte]func() (interfaces.LightClientFinalityUpdate, error){
		bytesutil.ToBytes4(params.BeaconConfig().AltairForkVersion): func() (interfaces.LightClientFinalityUpdate, error) {
			return lightclientConsensusTypes.NewEmptyFinalityUpdateAltair(), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().BellatrixForkVersion): func() (interfaces.LightClientFinalityUpdate, error) {
			return lightclientConsensusTypes.NewEmptyFinalityUpdateAltair(), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().CapellaForkVersion): func() (interfaces.LightClientFinalityUpdate, error) {
			return lightclientConsensusTypes.NewEmptyFinalityUpdateCapella(), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().DenebForkVersion): func() (interfaces.LightClientFinalityUpdate, error) {
			return lightclientConsensusTypes.NewEmptyFinalityUpdateDeneb(), nil
		},
		bytesutil.ToBytes4(params.BeaconConfig().ElectraForkVersion): func() (interfaces.LightClientFinalityUpdate, error) {
			return lightclientConsensusTypes.NewEmptyFinalityUpdateElectra(), nil
		},
	}
}
