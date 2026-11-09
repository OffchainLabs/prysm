package types

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// GetBlockSignRequest maps the request for signing type BLOCK.
func GetBlockSignRequest(request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*BlockSignRequest, error) {
	beaconBlock, ok := request.Object.(*validatorpb.SignRequest_Block)
	if !ok {
		return nil, errors.New("failed to cast request object to block")
	}
	if beaconBlock == nil {
		return nil, errors.New("invalid sign request: ReadOnlyBeaconBlock is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	beaconBlockBody, err := MapBeaconBlockBody(beaconBlock.Block.Body)
	if err != nil {
		return nil, err
	}
	return &BlockSignRequest{
		Type:        "BLOCK",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		Block: &BeaconBlock{
			Slot:          fmt.Sprint(beaconBlock.Block.Slot),
			ProposerIndex: fmt.Sprint(beaconBlock.Block.ProposerIndex),
			ParentRoot:    beaconBlock.Block.ParentRoot,
			StateRoot:     beaconBlock.Block.StateRoot,
			Body:          beaconBlockBody,
		},
	}, nil
}

// GetAggregationSlotSignRequest maps the request for signing type AGGREGATION_SLOT.
func GetAggregationSlotSignRequest(request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*AggregationSlotSignRequest, error) {
	aggregationSlot, ok := request.Object.(*validatorpb.SignRequest_Slot)
	if !ok {
		return nil, errors.New("failed to cast request object to aggregation slot")
	}
	if aggregationSlot == nil {
		return nil, errors.New("invalid sign request: Slot is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	return &AggregationSlotSignRequest{
		Type:        "AGGREGATION_SLOT",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		AggregationSlot: &AggregationSlot{
			Slot: fmt.Sprint(aggregationSlot.Slot),
		},
	}, nil
}

// GetAggregateAndProofV2SignRequest maps the request for signing type AGGREGATE_AND_PROOF_V2 on Electra changes.
func GetAggregateAndProofV2SignRequest(v int, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*AggregateAndProofV2SignRequest, error) {
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}

	var aggregateAndProofJSON []byte
	if v < version.Electra {
		aggregateAttestationAndProof, ok := request.Object.(*validatorpb.SignRequest_AggregateAttestationAndProof)
		if !ok {
			return nil, errors.New("failed to cast request object to aggregate attestation and proof")
		}
		if aggregateAttestationAndProof == nil {
			return nil, errors.New("invalid sign request: AggregateAndProof is nil")
		}
		aggregateAndProof, err := MapAggregateAndProof(aggregateAttestationAndProof.AggregateAttestationAndProof)
		if err != nil {
			return nil, err
		}
		aggregateAndProofJSON, err = json.Marshal(aggregateAndProof)
		if err != nil {
			return nil, err
		}
	} else {
		aggregateAttestationAndProof, ok := request.Object.(*validatorpb.SignRequest_AggregateAttestationAndProofElectra)
		if !ok {
			return nil, errors.New("failed to cast request object to aggregate attestation and proof Electra")
		}
		if aggregateAttestationAndProof == nil {
			return nil, errors.New("invalid sign request: AggregateAndProof is nil")
		}
		aggregateAndProof, err := MapAggregateAndProofElectra(aggregateAttestationAndProof.AggregateAttestationAndProofElectra)
		if err != nil {
			return nil, err
		}
		aggregateAndProofJSON, err = json.Marshal(aggregateAndProof)
		if err != nil {
			return nil, err
		}
	}

	return &AggregateAndProofV2SignRequest{
		Type:        "AGGREGATE_AND_PROOF_V2",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		AggregateAndProof: &AggregateAndProofV2{
			Version: strings.ToUpper(version.String(v)),
			Data:    aggregateAndProofJSON,
		},
	}, nil
}

// GetAttestationSignRequest maps the request for signing type ATTESTATION.
func GetAttestationSignRequest(request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*AttestationSignRequest, error) {
	attestation, ok := request.Object.(*validatorpb.SignRequest_AttestationData)
	if !ok {
		return nil, errors.New("failed to cast request object to attestation")
	}
	if attestation == nil {
		return nil, errors.New("invalid sign request: Attestation is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	attestationData, err := MapAttestationData(attestation.AttestationData)
	if err != nil {
		return nil, err
	}
	return &AttestationSignRequest{
		Type:        "ATTESTATION",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		Attestation: attestationData,
	}, nil
}

// GetBlockAltairSignRequest maps the request for signing type BLOCK_V2.
func GetBlockAltairSignRequest(request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*BlockAltairSignRequest, error) {
	beaconBlockAltair, ok := request.Object.(*validatorpb.SignRequest_BlockAltair)
	if !ok {
		return nil, errors.New("failed to cast request object to block altair")
	}
	if beaconBlockAltair == nil {
		return nil, errors.New("invalid sign request: ReadOnlyBeaconBlock is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	blockAltair, err := MapBeaconBlockAltair(beaconBlockAltair.BlockAltair)
	if err != nil {
		return nil, err
	}
	return &BlockAltairSignRequest{
		Type:        "BLOCK_V2",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		BeaconBlock: &BeaconBlockAltairBlockV2{
			Version: "ALTAIR",
			Block:   blockAltair,
		},
	}, nil
}

// GetRandaoRevealSignRequest maps the request for signing type RANDAO_REVEAL.
func GetRandaoRevealSignRequest(request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*RandaoRevealSignRequest, error) {
	randaoReveal, ok := request.Object.(*validatorpb.SignRequest_Epoch)
	if !ok {
		return nil, errors.New("failed to cast request object to randao reveal")
	}
	if randaoReveal == nil {
		return nil, errors.New("invalid sign request: Epoch is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	return &RandaoRevealSignRequest{
		Type:        "RANDAO_REVEAL",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		RandaoReveal: &RandaoReveal{
			Epoch: fmt.Sprint(randaoReveal.Epoch),
		},
	}, nil
}

// GetVoluntaryExitSignRequest maps the request for signing type VOLUNTARY_EXIT.
func GetVoluntaryExitSignRequest(request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*VoluntaryExitSignRequest, error) {
	voluntaryExit, ok := request.Object.(*validatorpb.SignRequest_Exit)
	if !ok {
		return nil, errors.New("failed to cast request object to voluntary exit")
	}
	if voluntaryExit == nil || voluntaryExit.Exit == nil {
		return nil, errors.New("invalid sign request: Exit is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	return &VoluntaryExitSignRequest{
		Type:        "VOLUNTARY_EXIT",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		VoluntaryExit: &VoluntaryExit{
			ValidatorIndex: fmt.Sprint(voluntaryExit.Exit.ValidatorIndex),
			Epoch:          fmt.Sprint(voluntaryExit.Exit.Epoch),
		},
	}, nil
}

// GetSyncCommitteeMessageSignRequest maps the request for signing type SYNC_COMMITTEE_MESSAGE.
func GetSyncCommitteeMessageSignRequest(request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*SyncCommitteeMessageSignRequest, error) {
	if request == nil {
		return nil, errors.New("nil sign request provided")
	}
	syncCommitteeMessage, ok := request.Object.(*validatorpb.SignRequest_SyncMessageBlockRoot)
	if !ok {
		return nil, errors.New("failed to cast request object to sync committee message")
	}
	if syncCommitteeMessage == nil || syncCommitteeMessage.SyncMessageBlockRoot == nil {
		return nil, errors.New("invalid sign request: SyncCommitteeMessage is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	return &SyncCommitteeMessageSignRequest{
		Type:        "SYNC_COMMITTEE_MESSAGE",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		SyncCommitteeMessage: &SyncCommitteeMessage{
			BeaconBlockRoot: syncCommitteeMessage.SyncMessageBlockRoot,
			Slot:            fmt.Sprint(request.SigningSlot),
		},
	}, nil
}

// GetSyncCommitteeSelectionProofSignRequest maps the request for signing type SYNC_COMMITTEE_SELECTION_PROOF.
func GetSyncCommitteeSelectionProofSignRequest(request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*SyncCommitteeSelectionProofSignRequest, error) {
	if request == nil {
		return nil, errors.New("nil sign request provided")
	}
	syncCommitteeSelectionProof, ok := request.Object.(*validatorpb.SignRequest_SyncAggregatorSelectionData)
	if !ok {
		return nil, errors.New("failed to cast request object to sync committee selection proof")
	}
	if syncCommitteeSelectionProof == nil {
		return nil, errors.New("invalid sign request: SyncCommitteeSelectionProof is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	aggregatorSelectionData, err := MapSyncAggregatorSelectionData(syncCommitteeSelectionProof.SyncAggregatorSelectionData)
	if err != nil {
		return nil, err
	}
	return &SyncCommitteeSelectionProofSignRequest{
		Type:                        "SYNC_COMMITTEE_SELECTION_PROOF",
		ForkInfo:                    fork,
		SigningRoot:                 request.SigningRoot,
		SyncAggregatorSelectionData: aggregatorSelectionData,
	}, nil
}

// GetSyncCommitteeContributionAndProofSignRequest maps the request for signing type SYNC_COMMITTEE_CONTRIBUTION_AND_PROOF.
func GetSyncCommitteeContributionAndProofSignRequest(request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*SyncCommitteeContributionAndProofSignRequest, error) {
	if request == nil {
		return nil, errors.New("nil sign request provided")
	}
	syncCommitteeContributionAndProof, ok := request.Object.(*validatorpb.SignRequest_ContributionAndProof)
	if !ok {
		return nil, errors.New("failed to cast request object to sync committee contribution and proof")
	}
	if syncCommitteeContributionAndProof == nil {
		return nil, errors.New("invalid sign request: SyncCommitteeContributionAndProof is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	contribution, err := MapContributionAndProof(syncCommitteeContributionAndProof.ContributionAndProof)
	if err != nil {
		return nil, err
	}
	return &SyncCommitteeContributionAndProofSignRequest{
		Type:                 "SYNC_COMMITTEE_CONTRIBUTION_AND_PROOF",
		ForkInfo:             fork,
		SigningRoot:          request.SigningRoot,
		ContributionAndProof: contribution,
	}, nil
}

// GetBlockV2BlindedSignRequest maps the request for signing types (GetBlockV2 id defined by the remote signer interface and not the beacon APIs)
func GetBlockV2BlindedSignRequest(request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*BlockV2BlindedSignRequest, error) {
	if request == nil {
		return nil, errors.New("nil sign request provided")
	}
	var pb any
	var ver int
	switch o := request.Object.(type) {
	case *validatorpb.SignRequest_BlockBellatrix:
		if o == nil {
			return nil, errors.New("invalid sign request: bellatrix block is nil")
		}
		pb, ver = o.BlockBellatrix, version.Bellatrix
	case *validatorpb.SignRequest_BlindedBlockBellatrix:
		if o == nil {
			return nil, errors.New("invalid sign request - blinded bellatrix block is nil")
		}
		pb, ver = o.BlindedBlockBellatrix, version.Bellatrix
	case *validatorpb.SignRequest_BlockCapella:
		if o == nil {
			return nil, errors.New("invalid sign request: capella block is nil")
		}
		pb, ver = o.BlockCapella, version.Capella
	case *validatorpb.SignRequest_BlindedBlockCapella:
		if o == nil {
			return nil, errors.New("invalid sign request: blinded capella block is nil")
		}
		pb, ver = o.BlindedBlockCapella, version.Capella
	case *validatorpb.SignRequest_BlockDeneb:
		if o == nil {
			return nil, errors.New("invalid sign request: deneb block is nil")
		}
		pb, ver = o.BlockDeneb, version.Deneb
	case *validatorpb.SignRequest_BlindedBlockDeneb:
		if o == nil {
			return nil, errors.New("invalid sign request: blinded deneb block is nil")
		}
		pb, ver = o.BlindedBlockDeneb, version.Deneb
	case *validatorpb.SignRequest_BlockElectra:
		if o == nil {
			return nil, errors.New("invalid sign request: electra block is nil")
		}
		pb, ver = o.BlockElectra, version.Electra
	case *validatorpb.SignRequest_BlindedBlockElectra:
		if o == nil {
			return nil, errors.New("invalid sign request: blinded electra block is nil")
		}
		pb, ver = o.BlindedBlockElectra, version.Electra
	case *validatorpb.SignRequest_BlockFulu:
		if o == nil {
			return nil, errors.New("invalid sign request: fulu block is nil")
		}
		pb, ver = o.BlockFulu, version.Fulu
	case *validatorpb.SignRequest_BlindedBlockFulu:
		if o == nil {
			return nil, errors.New("invalid sign request: blinded fulu block is nil")
		}
		pb, ver = o.BlindedBlockFulu, version.Fulu
	case *validatorpb.SignRequest_BlockGloas:
		if o == nil {
			return nil, errors.New("invalid sign request: gloas block is nil")
		}
		pb, ver = o.BlockGloas, version.Gloas
	case *validatorpb.SignRequest_BlockHeze:
		if o == nil {
			return nil, errors.New("invalid sign request: heze block is nil")
		}
		pb, ver = o.BlockHeze, version.Heze
	default:
		return nil, errors.New("invalid sign request - invalid object type")
	}
	b, err := blocks.NewBeaconBlock(pb)
	if err != nil {
		return nil, fmt.Errorf("initialize %s block: %w", version.String(ver), err)
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, fmt.Errorf("map %s block fork info: %w", version.String(ver), err)
	}
	beaconBlockHeader, err := interfaces.BeaconBlockHeaderFromBlockInterface(b)
	if err != nil {
		return nil, fmt.Errorf("create %s block header: %w", version.String(ver), err)
	}
	return &BlockV2BlindedSignRequest{
		Type:        "BLOCK_V2",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		BeaconBlock: &BeaconBlockV2Blinded{
			Version: strings.ToUpper(version.String(ver)),
			BlockHeader: &BeaconBlockHeader{
				Slot:          fmt.Sprint(beaconBlockHeader.Slot),
				ProposerIndex: fmt.Sprint(beaconBlockHeader.ProposerIndex),
				ParentRoot:    beaconBlockHeader.ParentRoot,
				StateRoot:     beaconBlockHeader.StateRoot,
				BodyRoot:      beaconBlockHeader.BodyRoot,
			},
		},
	}, nil
}

// GetBuilderRequestAuthSignRequest maps the request for signing type BUILDER_REQUEST_AUTH (gloas builder API).
func GetBuilderRequestAuthSignRequest(v int, request *validatorpb.SignRequest) (*BuilderRequestAuthSignRequest, error) {
	if request == nil {
		return nil, errors.New("nil sign request provided")
	}
	authReq, ok := request.Object.(*validatorpb.SignRequest_BuilderRequestAuth)
	if !ok {
		return nil, errors.New("failed to cast request object to builder request auth")
	}
	if authReq == nil || authReq.BuilderRequestAuth == nil {
		return nil, errors.New("invalid sign request: BuilderRequestAuth is nil")
	}
	return &BuilderRequestAuthSignRequest{
		Type:        "BUILDER_REQUEST_AUTH",
		SigningRoot: request.SigningRoot,
		BuilderRequestAuth: &VersionedBuilderRequestAuth{
			Version: strings.ToUpper(version.String(v)),
			Data: &BuilderRequestAuth{
				Data: authReq.BuilderRequestAuth.Data,
				Slot: fmt.Sprint(authReq.BuilderRequestAuth.Slot),
			},
		},
	}, nil
}

// GetExecutionPayloadEnvelopeSignRequest maps the request for signing type EXECUTION_PAYLOAD_ENVELOPE (gloas).
func GetExecutionPayloadEnvelopeSignRequest(v int, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*ExecutionPayloadEnvelopeSignRequest, error) {
	if request == nil {
		return nil, errors.New("nil sign request provided")
	}
	envelopeReq, ok := request.Object.(*validatorpb.SignRequest_ExecutionPayloadEnvelope)
	if !ok {
		return nil, errors.New("failed to cast request object to execution payload envelope")
	}
	if envelopeReq == nil || envelopeReq.ExecutionPayloadEnvelope == nil {
		return nil, errors.New("invalid sign request: ExecutionPayloadEnvelope is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	envelope, err := MapExecutionPayloadEnvelope(envelopeReq.ExecutionPayloadEnvelope)
	if err != nil {
		return nil, err
	}
	return &ExecutionPayloadEnvelopeSignRequest{
		Type:        "EXECUTION_PAYLOAD_ENVELOPE",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		ExecutionPayloadEnvelope: &VersionedExecutionPayloadEnvelope{
			Version: strings.ToUpper(version.String(v)),
			Data:    envelope,
		},
	}, nil
}

// GetPayloadAttestationMessageSignRequest maps the request for signing type PAYLOAD_ATTESTATION_MESSAGE (gloas).
func GetPayloadAttestationMessageSignRequest(v int, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*PayloadAttestationMessageSignRequest, error) {
	if request == nil {
		return nil, errors.New("nil sign request provided")
	}
	pa, ok := request.Object.(*validatorpb.SignRequest_PayloadAttestationData)
	if !ok {
		return nil, errors.New("failed to cast request object to payload attestation data")
	}
	if pa == nil || pa.PayloadAttestationData == nil {
		return nil, errors.New("invalid sign request: PayloadAttestationData is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	return &PayloadAttestationMessageSignRequest{
		Type:        "PAYLOAD_ATTESTATION_MESSAGE",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		PayloadAttestationMessage: &VersionedPayloadAttestationData{
			Version: strings.ToUpper(version.String(v)),
			Data: &PayloadAttestationData{
				BeaconBlockRoot:   pa.PayloadAttestationData.BeaconBlockRoot,
				Slot:              fmt.Sprint(pa.PayloadAttestationData.Slot),
				PayloadPresent:    pa.PayloadAttestationData.PayloadPresent,
				BlobDataAvailable: pa.PayloadAttestationData.BlobDataAvailable,
			},
		},
	}, nil
}

// GetProposerPreferencesSignRequest maps the request for signing type PROPOSER_PREFERENCES (gloas).
func GetProposerPreferencesSignRequest(v int, request *validatorpb.SignRequest, genesisValidatorsRoot []byte) (*ProposerPreferencesSignRequest, error) {
	if request == nil {
		return nil, errors.New("nil sign request provided")
	}
	pp, ok := request.Object.(*validatorpb.SignRequest_ProposerPreference)
	if !ok {
		return nil, errors.New("failed to cast request object to proposer preferences")
	}
	if pp == nil || pp.ProposerPreference == nil {
		return nil, errors.New("invalid sign request: ProposerPreferences is nil")
	}
	fork, err := MapForkInfo(request.SigningSlot, genesisValidatorsRoot)
	if err != nil {
		return nil, err
	}
	return &ProposerPreferencesSignRequest{
		Type:        "PROPOSER_PREFERENCES",
		ForkInfo:    fork,
		SigningRoot: request.SigningRoot,
		ProposerPreferences: &VersionedProposerPreferences{
			Version: strings.ToUpper(version.String(v)),
			Data: &ProposerPreferences{
				DependentRoot:  pp.ProposerPreference.DependentRoot,
				ProposalSlot:   fmt.Sprint(pp.ProposerPreference.ProposalSlot),
				ValidatorIndex: fmt.Sprint(pp.ProposerPreference.ValidatorIndex),
				FeeRecipient:   pp.ProposerPreference.FeeRecipient,
				TargetGasLimit: fmt.Sprint(pp.ProposerPreference.TargetGasLimit),
			},
		},
	}, nil
}

// GetValidatorRegistrationSignRequest maps the request for signing type VALIDATOR_REGISTRATION.
func GetValidatorRegistrationSignRequest(request *validatorpb.SignRequest) (*ValidatorRegistrationSignRequest, error) {
	if request == nil {
		return nil, errors.New("nil sign request provided")
	}
	validatorRegistrationRequest, ok := request.Object.(*validatorpb.SignRequest_Registration)
	if !ok {
		return nil, errors.New("failed to cast request object to validator registration")
	}
	registration := validatorRegistrationRequest.Registration
	return &ValidatorRegistrationSignRequest{
		Type:        "VALIDATOR_REGISTRATION",
		SigningRoot: request.SigningRoot,
		ValidatorRegistration: &ValidatorRegistration{
			FeeRecipient: registration.FeeRecipient,
			GasLimit:     fmt.Sprint(registration.GasLimit),
			Timestamp:    fmt.Sprint(registration.Timestamp),
			Pubkey:       registration.Pubkey,
		},
	}, nil
}
