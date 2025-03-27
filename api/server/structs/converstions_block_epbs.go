package structs

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/api/server"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// ----------------------------------------------------------------------------
// Epbs
// ----------------------------------------------------------------------------

// nolint:gocognit
func (b *BeaconBlockEpbs) ToConsensus() (*eth.BeaconBlockEpbs, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, server.NewDecodeError(errNilValue, "Body")
	}
	if b.Body.Eth1Data == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.Eth1Data")
	}
	if b.Body.SyncAggregate == nil {
		return nil, server.NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := bytesutil.DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := bytesutil.DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := bytesutil.DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.DepositCount")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(b.Body.Eth1Data.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Eth1Data.BlockHash")
	}
	graffiti, err := bytesutil.DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsElectraToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsElectraToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.Deposits")
	}
	exits, err := SignedExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}
	syncCommitteeSig, err := bytesutil.DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeSignature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
	}
	signedPayloadHeader, err := b.Body.SignedExecutionPayloadHeader.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.SignedExecutionPayloadHeader")
	}

	blsChanges, err := SignedBLSChangesToConsensus(b.Body.BLSToExecutionChanges)
	if err != nil {
		return nil, server.NewDecodeError(err, "Body.BLSToExecutionChanges")
	}
	payloadAttestations := make([]*eth.PayloadAttestation, len(b.Body.PayloadAttestations))
	for i, p := range b.Body.PayloadAttestations {
		payloadAttestations[i], err = p.ToConsensus()
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Body.PayloadAttestations[%d]", i))
		}
	}

	return &eth.BeaconBlockEpbs{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &eth.BeaconBlockBodyEpbs{
			RandaoReveal: randaoReveal,
			Eth1Data: &eth.Eth1Data{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &eth.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSig,
			},
			BlsToExecutionChanges:        blsChanges,
			SignedExecutionPayloadHeader: signedPayloadHeader,
			PayloadAttestations:          payloadAttestations,
		},
	}, nil
}

func (p *PayloadAttestation) ToConsensus() (*eth.PayloadAttestation, error) {
	if p == nil {
		return nil, errNilValue
	}
	aggregationBits, err := bytesutil.DecodeHexWithLength(p.AggregationBits, fieldparams.PTCSize/8)
	if err != nil {
		return nil, server.NewDecodeError(err, "AggregationBits")
	}
	data, err := p.Data.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Data")
	}
	sig, err := bytesutil.DecodeHexWithLength(p.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	return &eth.PayloadAttestation{
		AggregationBits: aggregationBits,
		Data:            data,
		Signature:       sig,
	}, nil
}

func (p *PayloadAttestationData) ToConsensus() (*eth.PayloadAttestationData, error) {
	if p == nil {
		return nil, errNilValue
	}
	beaconBlockRoot, err := bytesutil.DecodeHexWithLength(p.BeaconBlockRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "BeaconBlockRoot")
	}
	slot, err := strconv.ParseUint(p.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	payloadStatus, err := strconv.ParseUint(p.PayloadStatus, 10, 8)
	if err != nil {
		return nil, server.NewDecodeError(err, "PayloadStatus")
	}
	return &eth.PayloadAttestationData{
		BeaconBlockRoot: beaconBlockRoot,
		Slot:            primitives.Slot(slot),
		PayloadStatus:   bytesutil.Bytes1(payloadStatus),
	}, nil
}

func (p *SignedExecutionPayloadHeader) ToConsensus() (*enginev1.SignedExecutionPayloadHeader, error) {
	if p == nil {
		return nil, errNilValue
	}
	sig, err := bytesutil.DecodeHexWithLength(p.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	header, err := p.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Header")
	}
	return &enginev1.SignedExecutionPayloadHeader{
		Message:   header,
		Signature: sig,
	}, nil
}

func (p *ExecutionPayloadHeaderEPBS) ToConsensus() (*enginev1.ExecutionPayloadHeaderEPBS, error) {
	parentBlockHash, err := bytesutil.DecodeHexWithLength(p.ParentBlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentBlockHash")
	}
	parentBlockRoot, err := bytesutil.DecodeHexWithLength(p.ParentBlockRoot, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "ParentBlockRoot")
	}
	blockHash, err := bytesutil.DecodeHexWithLength(p.BlockHash, common.HashLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "BlockHash")
	}
	gasLimit, err := strconv.ParseUint(p.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "GasLimit")
	}
	builderIndex, err := strconv.ParseUint(p.BuilderIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "BuilderIndex")
	}
	slot, err := strconv.ParseUint(p.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	value, err := strconv.ParseUint(p.Value, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Value")
	}
	blobKzgCommitmentsRoot, err := bytesutil.DecodeHexWithLength(p.BlobKzgCommitmentsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "BlobKzgCommitmentsRoot")
	}
	return &enginev1.ExecutionPayloadHeaderEPBS{
		ParentBlockHash:        parentBlockHash,
		ParentBlockRoot:        parentBlockRoot,
		BlockHash:              blockHash,
		GasLimit:               gasLimit,
		BuilderIndex:           primitives.ValidatorIndex(builderIndex),
		Slot:                   primitives.Slot(slot),
		Value:                  value,
		BlobKzgCommitmentsRoot: blobKzgCommitmentsRoot,
	}, nil
}

func (b *SignedBeaconBlockEpbs) ToConsensus() (*eth.SignedBeaconBlockEpbs, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := bytesutil.DecodeHexWithLength(b.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	block, err := b.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	return &eth.SignedBeaconBlockEpbs{
		Block:     block,
		Signature: sig,
	}, nil
}

func BeaconBlockEpbsFromConsensus(b *eth.BeaconBlockEpbs) (*BeaconBlockEpbs, error) {
	signedPayloadHeader, err := SignedExecutionPayloadHeaderFromConsensus(b.Body.SignedExecutionPayloadHeader)
	if err != nil {
		return nil, err
	}
	payloadAttestations, err := PayloadAttestationsFromConsensus(b.Body.PayloadAttestations)
	if err != nil {
		return nil, err
	}
	return &BeaconBlockEpbs{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBodyEpbs{
			RandaoReveal:      hexutil.Encode(b.Body.RandaoReveal),
			Eth1Data:          Eth1DataFromConsensus(b.Body.Eth1Data),
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: ProposerSlashingsFromConsensus(b.Body.ProposerSlashings),
			AttesterSlashings: AttesterSlashingsElectraFromConsensus(b.Body.AttesterSlashings),
			Attestations:      AttsElectraFromConsensus(b.Body.Attestations),
			Deposits:          DepositsFromConsensus(b.Body.Deposits),
			VoluntaryExits:    SignedExitsFromConsensus(b.Body.VoluntaryExits),
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:      hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignature: hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeSignature),
			},
			BLSToExecutionChanges:        SignedBLSChangesFromConsensus(b.Body.BlsToExecutionChanges),
			SignedExecutionPayloadHeader: signedPayloadHeader,
			PayloadAttestations:          payloadAttestations,
		},
	}, nil
}

func SignedExecutionPayloadEnvelopeFromConsensus(b *enginev1.SignedExecutionPayloadEnvelope) (*SignedExecutionPayloadEnvelope, error) {
	payload, err := ExecutionPayloadEnvelopeFromConsensus(b.Message)
	if err != nil {
		return nil, err
	}
	return &SignedExecutionPayloadEnvelope{
		Message:   payload,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func ExecutionPayloadEnvelopeFromConsensus(b *enginev1.ExecutionPayloadEnvelope) (*ExecutionPayloadEnvelope, error) {
	payload, err := ExecutionPayloadDenebFromConsensus(b.Payload)
	if err != nil {
		return nil, err
	}
	committments := make([]string, len(b.BlobKzgCommitments))
	for i, c := range b.BlobKzgCommitments {
		committments[i] = hexutil.Encode(c)
	}

	executionRequests := ExecutionRequestsFromConsensus(b.ExecutionRequests)
	return &ExecutionPayloadEnvelope{
		Payload:            payload,
		ExecutionRequests:  executionRequests,
		BuilderIndex:       fmt.Sprintf("%d", b.BuilderIndex),
		BeaconBlockRoot:    hexutil.Encode(b.BeaconBlockRoot),
		Slot:               fmt.Sprintf("%d", b.Slot),
		BlobKzgCommitments: committments,
		StateRoot:          hexutil.Encode(b.BeaconStateRoot),
	}, nil
}

func SignedBeaconBlockEpbsFromConsensus(b *eth.SignedBeaconBlockEpbs) (*SignedBeaconBlockEpbs, error) {
	block, err := BeaconBlockEpbsFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	return &SignedBeaconBlockEpbs{
		Message:   block,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func SignedExecutionPayloadHeaderFromConsensus(b *enginev1.SignedExecutionPayloadHeader) (*SignedExecutionPayloadHeader, error) {
	header, err := ExecutionPayloadHeaderEPBSFromConsensus(b.Message)
	if err != nil {
		return nil, err
	}
	return &SignedExecutionPayloadHeader{
		Message:   header,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func ExecutionPayloadHeaderEPBSFromConsensus(b *enginev1.ExecutionPayloadHeaderEPBS) (*ExecutionPayloadHeaderEPBS, error) {
	return &ExecutionPayloadHeaderEPBS{
		ParentBlockHash:        hexutil.Encode(b.ParentBlockHash),
		ParentBlockRoot:        hexutil.Encode(b.ParentBlockRoot),
		BlockHash:              hexutil.Encode(b.BlockHash),
		GasLimit:               fmt.Sprintf("%d", b.GasLimit),
		BuilderIndex:           fmt.Sprintf("%d", b.BuilderIndex),
		Slot:                   fmt.Sprintf("%d", b.Slot),
		Value:                  fmt.Sprintf("%d", b.Value),
		BlobKzgCommitmentsRoot: hexutil.Encode(b.BlobKzgCommitmentsRoot),
	}, nil
}

func PayloadAttestationsFromConsensus(b []*eth.PayloadAttestation) ([]*PayloadAttestation, error) {
	payloadAttestations := make([]*PayloadAttestation, len(b))
	for i, p := range b {
		data, err := PayloadAttestationDataFromConsensus(p.Data)
		if err != nil {
			return nil, err
		}
		payloadAttestations[i] = &PayloadAttestation{
			AggregationBits: hexutil.Encode(p.AggregationBits),
			Data:            data,
			Signature:       hexutil.Encode(p.Signature),
		}
	}
	return payloadAttestations, nil
}

func PayloadAttestationDataFromConsensus(b *eth.PayloadAttestationData) (*PayloadAttestationData, error) {
	return &PayloadAttestationData{
		BeaconBlockRoot: hexutil.Encode(b.BeaconBlockRoot),
		Slot:            fmt.Sprintf("%d", b.Slot),
		PayloadStatus:   fmt.Sprintf("%d", b.PayloadStatus),
	}, nil
}
