package eth

import (
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

type copier[T any] interface {
	Copy() T
}

func CopySlice[T any, C copier[T]](original []C) []T {
	// Create a new slice with the same length as the original
	newSlice := make([]T, len(original))
	for i := 0; i < len(newSlice); i++ {
		newSlice[i] = original[i].Copy()
	}
	return newSlice
}

// CopyValidator copies the provided validator.
func CopyValidator(val *Validator) *Validator {
	pubKey := make([]byte, len(val.PublicKey))
	copy(pubKey, val.PublicKey)
	withdrawalCreds := make([]byte, len(val.WithdrawalCredentials))
	copy(withdrawalCreds, val.WithdrawalCredentials)
	return &Validator{
		PublicKey:                  pubKey,
		WithdrawalCredentials:      withdrawalCreds,
		EffectiveBalance:           val.EffectiveBalance,
		Slashed:                    val.Slashed,
		ActivationEligibilityEpoch: val.ActivationEligibilityEpoch,
		ActivationEpoch:            val.ActivationEpoch,
		ExitEpoch:                  val.ExitEpoch,
		WithdrawableEpoch:          val.WithdrawableEpoch,
	}
}

// CopySyncCommitteeMessage copies the provided sync committee message object.
func CopySyncCommitteeMessage(s *SyncCommitteeMessage) *SyncCommitteeMessage {
	if s == nil {
		return nil
	}
	return &SyncCommitteeMessage{
		Slot:           s.Slot,
		BlockRoot:      bytesutil.SafeCopyBytes(s.BlockRoot),
		ValidatorIndex: s.ValidatorIndex,
		Signature:      bytesutil.SafeCopyBytes(s.Signature),
	}
}

// CopySyncCommitteeContribution copies the provided sync committee contribution object.
func CopySyncCommitteeContribution(c *SyncCommitteeContribution) *SyncCommitteeContribution {
	if c == nil {
		return nil
	}
	return &SyncCommitteeContribution{
		Slot:              c.Slot,
		BlockRoot:         bytesutil.SafeCopyBytes(c.BlockRoot),
		SubcommitteeIndex: c.SubcommitteeIndex,
		AggregationBits:   bytesutil.SafeCopyBytes(c.AggregationBits),
		Signature:         bytesutil.SafeCopyBytes(c.Signature),
	}
}

// CopySignedBeaconBlockEPBS copies the provided SignedBeaconBlockEPBS.
func CopySignedBeaconBlockEPBS(sigBlock *SignedBeaconBlockEpbs) *SignedBeaconBlockEpbs {
	if sigBlock == nil {
		return nil
	}
	return &SignedBeaconBlockEpbs{
		Block:     CopyBeaconBlockEPBS(sigBlock.Block),
		Signature: bytesutil.SafeCopyBytes(sigBlock.Signature),
	}
}

// CopyBeaconBlockEPBS copies the provided CopyBeaconBlockEPBS.
func CopyBeaconBlockEPBS(block *BeaconBlockEpbs) *BeaconBlockEpbs {
	if block == nil {
		return nil
	}
	return &BeaconBlockEpbs{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    bytesutil.SafeCopyBytes(block.ParentRoot),
		StateRoot:     bytesutil.SafeCopyBytes(block.StateRoot),
		Body:          CopyBeaconBlockBodyEPBS(block.Body),
	}
}

// CopyBeaconBlockBodyEPBS copies the provided CopyBeaconBlockBodyEPBS.
func CopyBeaconBlockBodyEPBS(body *BeaconBlockBodyEpbs) *BeaconBlockBodyEpbs {
	if body == nil {
		return nil
	}
	return &BeaconBlockBodyEpbs{
		RandaoReveal:                 bytesutil.SafeCopyBytes(body.RandaoReveal),
		Eth1Data:                     body.Eth1Data.Copy(),
		Graffiti:                     bytesutil.SafeCopyBytes(body.Graffiti),
		ProposerSlashings:            CopySlice(body.ProposerSlashings),
		AttesterSlashings:            CopySlice(body.AttesterSlashings),
		Attestations:                 CopySlice(body.Attestations),
		Deposits:                     CopySlice(body.Deposits),
		VoluntaryExits:               CopySlice(body.VoluntaryExits),
		SyncAggregate:                body.SyncAggregate.Copy(),
		BlsToExecutionChanges:        CopySlice(body.BlsToExecutionChanges),
		SignedExecutionPayloadHeader: CopySignedExecutionPayloadHeader(body.SignedExecutionPayloadHeader),
		PayloadAttestations:          CopyPayloadAttestation(body.PayloadAttestations),
	}
}

// CopySignedExecutionPayloadHeader copies the provided SignedExecutionPayloadHeader.
func CopySignedExecutionPayloadHeader(payload *enginev1.SignedExecutionPayloadHeader) *enginev1.SignedExecutionPayloadHeader {
	if payload == nil {
		return nil
	}
	return &enginev1.SignedExecutionPayloadHeader{
		Message:   CopyExecutionPayloadHeaderEPBS(payload.Message),
		Signature: bytesutil.SafeCopyBytes(payload.Signature),
	}
}

// CopyExecutionPayloadHeaderEPBS copies the provided execution payload header object.
func CopyExecutionPayloadHeaderEPBS(payload *enginev1.ExecutionPayloadHeaderEPBS) *enginev1.ExecutionPayloadHeaderEPBS {
	if payload == nil {
		return nil
	}
	return &enginev1.ExecutionPayloadHeaderEPBS{
		ParentBlockHash:        bytesutil.SafeCopyBytes(payload.ParentBlockHash),
		ParentBlockRoot:        bytesutil.SafeCopyBytes(payload.ParentBlockRoot),
		BlockHash:              bytesutil.SafeCopyBytes(payload.BlockHash),
		BuilderIndex:           payload.BuilderIndex,
		Slot:                   payload.Slot,
		Value:                  payload.Value,
		BlobKzgCommitmentsRoot: bytesutil.SafeCopyBytes(payload.BlobKzgCommitmentsRoot),
	}
}

// CopyPayloadAttestation copies the provided PayloadAttestation array.
func CopyPayloadAttestation(attestations []*PayloadAttestation) []*PayloadAttestation {
	if attestations == nil {
		return nil
	}
	newAttestations := make([]*PayloadAttestation, len(attestations))
	for i, att := range attestations {
		newAttestations[i] = &PayloadAttestation{
			AggregationBits: bytesutil.SafeCopyBytes(att.AggregationBits),
			Data:            CopyPayloadAttestationData(att.Data),
			Signature:       bytesutil.SafeCopyBytes(att.Signature),
		}
	}
	return newAttestations
}

// CopyPayloadAttestationData copies the provided PayloadAttestationData.
func CopyPayloadAttestationData(data *PayloadAttestationData) *PayloadAttestationData {
	if data == nil {
		return nil
	}
	return &PayloadAttestationData{
		BeaconBlockRoot: bytesutil.SafeCopyBytes(data.BeaconBlockRoot),
		Slot:            data.Slot,
		PayloadStatus:   data.PayloadStatus,
	}
}
