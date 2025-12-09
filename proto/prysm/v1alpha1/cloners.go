package eth

import (
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

type copier[T any] interface {
	Copy() T
}

func CopySlice[T any, C copier[T]](original []C) []T {
	// Create a new slice with the same length as the original
	newSlice := make([]T, len(original))
	for i := range newSlice {
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

// CopySignedBeaconBlockGloas copies the provided signed beacon block Gloas object.
func CopySignedBeaconBlockGloas(sb *SignedBeaconBlockGloas) *SignedBeaconBlockGloas {
	if sb == nil {
		return nil
	}
	return &SignedBeaconBlockGloas{
		Block:     copyBeaconBlockGloas(sb.Block),
		Signature: bytesutil.SafeCopyBytes(sb.Signature),
	}
}

// copyBeaconBlockGloas copies the provided beacon block Gloas object.
func copyBeaconBlockGloas(b *BeaconBlockGloas) *BeaconBlockGloas {
	if b == nil {
		return nil
	}
	return &BeaconBlockGloas{
		Slot:          b.Slot,
		ProposerIndex: b.ProposerIndex,
		ParentRoot:    bytesutil.SafeCopyBytes(b.ParentRoot),
		StateRoot:     bytesutil.SafeCopyBytes(b.StateRoot),
		Body:          copyBeaconBlockBodyGloas(b.Body),
	}
}

// copyPayloadAttestation copies the provided payload attestation object.
func copyPayloadAttestation(pa *PayloadAttestation) *PayloadAttestation {
	if pa == nil {
		return nil
	}
	copied := &PayloadAttestation{
		AggregationBits: pa.AggregationBits,
		Signature:       bytesutil.SafeCopyBytes(pa.Signature),
	}
	if pa.Data != nil {
		copied.Data = &PayloadAttestationData{
			BeaconBlockRoot:   bytesutil.SafeCopyBytes(pa.Data.BeaconBlockRoot),
			Slot:              pa.Data.Slot,
			PayloadPresent:    pa.Data.PayloadPresent,
			BlobDataAvailable: pa.Data.BlobDataAvailable,
		}
	}
	return copied
}

// copyPayloadAttestations copies a slice of payload attestations.
func copyPayloadAttestations(pas []*PayloadAttestation) []*PayloadAttestation {
	if len(pas) == 0 {
		return nil
	}
	copied := make([]*PayloadAttestation, len(pas))
	for i, pa := range pas {
		copied[i] = copyPayloadAttestation(pa)
	}
	return copied
}

// copySignedExecutionPayloadBid copies the provided signed execution payload header.
func copySignedExecutionPayloadBid(header *SignedExecutionPayloadBid) *SignedExecutionPayloadBid {
	if header == nil {
		return nil
	}
	copied := &SignedExecutionPayloadBid{
		Signature: bytesutil.SafeCopyBytes(header.Signature),
	}
	if header.Message != nil {
		copied.Message = &ExecutionPayloadBid{
			ParentBlockHash:        bytesutil.SafeCopyBytes(header.Message.ParentBlockHash),
			ParentBlockRoot:        bytesutil.SafeCopyBytes(header.Message.ParentBlockRoot),
			BlockHash:              bytesutil.SafeCopyBytes(header.Message.BlockHash),
			FeeRecipient:           bytesutil.SafeCopyBytes(header.Message.FeeRecipient),
			GasLimit:               header.Message.GasLimit,
			BuilderIndex:           header.Message.BuilderIndex,
			Slot:                   header.Message.Slot,
			Value:                  header.Message.Value,
			BlobKzgCommitmentsRoot: bytesutil.SafeCopyBytes(header.Message.BlobKzgCommitmentsRoot),
		}
	}
	return copied
}

// copyBeaconBlockBodyGloas copies the provided beacon block body Gloas object.
func copyBeaconBlockBodyGloas(body *BeaconBlockBodyGloas) *BeaconBlockBodyGloas {
	if body == nil {
		return nil
	}

	copied := &BeaconBlockBodyGloas{
		RandaoReveal: bytesutil.SafeCopyBytes(body.RandaoReveal),
		Graffiti:     bytesutil.SafeCopyBytes(body.Graffiti),
	}

	if body.Eth1Data != nil {
		copied.Eth1Data = body.Eth1Data.Copy()
	}

	if body.SyncAggregate != nil {
		copied.SyncAggregate = body.SyncAggregate.Copy()
	}

	copied.ProposerSlashings = CopySlice(body.ProposerSlashings)
	copied.AttesterSlashings = CopySlice(body.AttesterSlashings)
	copied.Attestations = CopySlice(body.Attestations)
	copied.Deposits = CopySlice(body.Deposits)
	copied.VoluntaryExits = CopySlice(body.VoluntaryExits)
	copied.BlsToExecutionChanges = CopySlice(body.BlsToExecutionChanges)

	copied.SignedExecutionPayloadBid = copySignedExecutionPayloadBid(body.SignedExecutionPayloadBid)
	copied.PayloadAttestations = copyPayloadAttestations(body.PayloadAttestations)

	return copied
}
