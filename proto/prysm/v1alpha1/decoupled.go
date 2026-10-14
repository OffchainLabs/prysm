package eth

import (
	"slices"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

func (c *CheckpointDecoupled) Copy() *CheckpointDecoupled {
	if c == nil {
		return nil
	}
	return &CheckpointDecoupled{Slot: c.Slot, Root: bytesutil.SafeCopyBytes(c.Root)}
}

func (a *AttestationDataDecoupled) Copy() *AttestationDataDecoupled {
	if a == nil {
		return nil
	}
	return &AttestationDataDecoupled{
		Height:         a.Height,
		Target:         bytesutil.SafeCopyBytes(a.Target),
		Timeout:        a.Timeout,
		FinalityHeight: a.FinalityHeight,
		FinalityTarget: bytesutil.SafeCopyBytes(a.FinalityTarget),
	}
}

func (a *IndexedAttestationDecoupled) Copy() *IndexedAttestationDecoupled {
	if a == nil {
		return nil
	}
	return &IndexedAttestationDecoupled{
		AttestingIndices: slices.Clone(a.AttestingIndices),
		Data:             a.Data.Copy(),
		Signature:        bytesutil.SafeCopyBytes(a.Signature),
	}
}

func (a *AvailableAttestationData) Copy() *AvailableAttestationData {
	if a == nil {
		return nil
	}
	return &AvailableAttestationData{
		Slot:            a.Slot,
		PayloadPresent:  a.PayloadPresent,
		BeaconBlockRoot: bytesutil.SafeCopyBytes(a.BeaconBlockRoot),
	}
}

func (a *AvailableAttestation) Copy() *AvailableAttestation {
	if a == nil {
		return nil
	}
	return &AvailableAttestation{
		AggregationBits: bytesutil.SafeCopyBytes(a.AggregationBits),
		Data:            a.Data.Copy(),
		Signature:       bytesutil.SafeCopyBytes(a.Signature),
	}
}

func (p *PendingFinality) Copy() *PendingFinality {
	if p == nil {
		return nil
	}
	return &PendingFinality{
		Height:        p.Height,
		Target:        p.Target.Copy(),
		Participation: bytesutil.SafeCopyBytes(p.Participation),
	}
}

func CopySignedBeaconBlockDecoupled(sb *SignedBeaconBlockDecoupled) *SignedBeaconBlockDecoupled {
	if sb == nil {
		return nil
	}
	return &SignedBeaconBlockDecoupled{
		Block:     copyBeaconBlockDecoupled(sb.Block),
		Signature: bytesutil.SafeCopyBytes(sb.Signature),
	}
}

func copyBeaconBlockDecoupled(b *BeaconBlockDecoupled) *BeaconBlockDecoupled {
	if b == nil {
		return nil
	}
	return &BeaconBlockDecoupled{
		Slot:          b.Slot,
		ProposerIndex: b.ProposerIndex,
		ParentRoot:    bytesutil.SafeCopyBytes(b.ParentRoot),
		StateRoot:     bytesutil.SafeCopyBytes(b.StateRoot),
		Body:          copyBeaconBlockBodyDecoupled(b.Body),
	}
}

func copyBeaconBlockBodyDecoupled(body *BeaconBlockBodyDecoupled) *BeaconBlockBodyDecoupled {
	if body == nil {
		return nil
	}
	return &BeaconBlockBodyDecoupled{
		RandaoReveal:              bytesutil.SafeCopyBytes(body.RandaoReveal),
		Eth1Data:                  body.Eth1Data.Copy(),
		Graffiti:                  bytesutil.SafeCopyBytes(body.Graffiti),
		ProposerSlashings:         CopySlice(body.ProposerSlashings),
		AttesterSlashings:         CopySlice(body.AttesterSlashings),
		Attestations:              CopySlice(body.Attestations),
		Deposits:                  CopySlice(body.Deposits),
		VoluntaryExits:            CopySlice(body.VoluntaryExits),
		SyncAggregate:             body.SyncAggregate.Copy(),
		BlsToExecutionChanges:     CopySlice(body.BlsToExecutionChanges),
		SignedExecutionPayloadBid: copySignedExecutionPayloadBid(body.SignedExecutionPayloadBid),
		PayloadAttestations:       copyPayloadAttestations(body.PayloadAttestations),
		ParentExecutionRequests:   CopyExecutionRequestsGloas(body.ParentExecutionRequests),
		AvailableAttestations:     CopySlice(body.AvailableAttestations),
		SupportVotes:              bytesutil.SafeCopyBytes(body.SupportVotes),
	}
}
