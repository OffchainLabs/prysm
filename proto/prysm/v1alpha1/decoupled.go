package eth

import "github.com/OffchainLabs/prysm/v7/encoding/bytesutil"

func (c *CheckpointDecoupled) Copy() *CheckpointDecoupled {
	if c == nil {
		return nil
	}
	return &CheckpointDecoupled{
		Slot: c.Slot,
		Root: bytesutil.SafeCopyBytes(c.Root),
	}
}

func (a *AttestationDataDecoupled) Copy() *AttestationDataDecoupled {
	if a == nil {
		return nil
	}
	return &AttestationDataDecoupled{
		Slot:           a.Slot,
		SafeBlockRoot:  bytesutil.SafeCopyBytes(a.SafeBlockRoot),
		Target:         a.Target.Copy(),
		Height:         a.Height,
		Timeout:        a.Timeout,
		FinalityTarget: a.FinalityTarget.Copy(),
		FinalityHeight: a.FinalityHeight,
	}
}

func (a *AttestationDecoupled) Copy() *AttestationDecoupled {
	if a == nil {
		return nil
	}
	return &AttestationDecoupled{
		AggregationBits: bytesutil.SafeCopyBytes(a.AggregationBits),
		Data:            a.Data.Copy(),
		Signature:       bytesutil.SafeCopyBytes(a.Signature),
		CommitteeBits:   bytesutil.SafeCopyBytes(a.CommitteeBits),
	}
}

func (a *IndexedAttestationDecoupled) Copy() *IndexedAttestationDecoupled {
	if a == nil {
		return nil
	}
	var indices []uint64
	if a.AttestingIndices != nil {
		indices = make([]uint64, len(a.AttestingIndices))
		copy(indices, a.AttestingIndices)
	}
	return &IndexedAttestationDecoupled{
		AttestingIndices: indices,
		Data:             a.Data.Copy(),
		Signature:        bytesutil.SafeCopyBytes(a.Signature),
	}
}

func (a *AttesterSlashingDecoupled) Copy() *AttesterSlashingDecoupled {
	if a == nil {
		return nil
	}
	return &AttesterSlashingDecoupled{
		Attestation_1: a.Attestation_1.Copy(),
		Attestation_2: a.Attestation_2.Copy(),
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
	copied := &BeaconBlockBodyDecoupled{
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
	copied.ParentExecutionRequests = CopyExecutionRequestsGloas(body.ParentExecutionRequests)
	copied.AvailableAttestations = CopySlice(body.AvailableAttestations)
	copied.SupportVotes = bytesutil.SafeCopyBytes(body.SupportVotes)
	return copied
}
