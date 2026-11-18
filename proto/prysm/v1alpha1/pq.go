package eth

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

func (v *ValidatorPQ) Copy() *ValidatorPQ {
	if v == nil {
		return nil
	}
	return &ValidatorPQ{
		PublicKey:                  bytesutil.SafeCopyBytes(v.PublicKey),
		VrfRoot:                    bytesutil.SafeCopyBytes(v.VrfRoot),
		WithdrawalCredentials:      bytesutil.SafeCopyBytes(v.WithdrawalCredentials),
		EffectiveBalance:           v.EffectiveBalance,
		Slashed:                    v.Slashed,
		ActivationEligibilityEpoch: v.ActivationEligibilityEpoch,
		ActivationEpoch:            v.ActivationEpoch,
		ExitEpoch:                  v.ExitEpoch,
		WithdrawableEpoch:          v.WithdrawableEpoch,
	}
}

func (b *BuilderPQ) Copy() *BuilderPQ {
	if b == nil {
		return nil
	}
	return &BuilderPQ{
		Pubkey:            bytesutil.SafeCopyBytes(b.Pubkey),
		Version:           bytesutil.SafeCopyBytes(b.Version),
		ExecutionAddress:  bytesutil.SafeCopyBytes(b.ExecutionAddress),
		Balance:           b.Balance,
		DepositEpoch:      b.DepositEpoch,
		WithdrawableEpoch: b.WithdrawableEpoch,
	}
}

func (p *ValidatorPreregistration) Copy() *ValidatorPreregistration {
	if p == nil {
		return nil
	}
	return &ValidatorPreregistration{
		Pubkey:                bytesutil.SafeCopyBytes(p.Pubkey),
		WithdrawalCredentials: bytesutil.SafeCopyBytes(p.WithdrawalCredentials),
		VrfRoot:               bytesutil.SafeCopyBytes(p.VrfRoot),
		ExpirySlot:            p.ExpirySlot,
	}
}

func (p *BuilderPreregistration) Copy() *BuilderPreregistration {
	if p == nil {
		return nil
	}
	return &BuilderPreregistration{
		Pubkey:                bytesutil.SafeCopyBytes(p.Pubkey),
		WithdrawalCredentials: bytesutil.SafeCopyBytes(p.WithdrawalCredentials),
		ExpirySlot:            p.ExpirySlot,
	}
}

func (a *AttestationPQ) Version() int {
	return version.PQ
}

func (a *AttestationPQ) IsNil() bool {
	return a == nil || a.Data == nil
}

func (*AttestationPQ) IsSingle() bool {
	return false
}

func (a *AttestationPQ) IsAggregated() bool {
	return a.AggregationBits.Count() > 1
}

func (a *AttestationPQ) Clone() Att {
	return a.Copy()
}

func (a *AttestationPQ) Copy() *AttestationPQ {
	if a == nil {
		return nil
	}
	return &AttestationPQ{
		AggregationBits: bytesutil.SafeCopyBytes(a.AggregationBits),
		Data:            a.Data.Copy(),
		CommitteeBits:   bytesutil.SafeCopyBytes(a.CommitteeBits),
	}
}

func (*AttestationPQ) GetAttestingIndex() primitives.ValidatorIndex {
	return 0
}

func (a *AttestationPQ) CommitteeBitsVal() bitfield.Bitfield {
	return a.CommitteeBits
}

func (*AttestationPQ) GetSignature() []byte {
	return nil
}

func (*AttestationPQ) SetSignature([]byte) {}

func (a *AttestationPQ) GetCommitteeIndex() primitives.CommitteeIndex {
	indices := a.CommitteeBits.BitIndices()
	if len(indices) == 0 {
		return 0
	}
	return primitives.CommitteeIndex(uint64(indices[0]))
}

func (a *SingleAttestationPQ) Version() int {
	return version.PQ
}

func (a *SingleAttestationPQ) IsNil() bool {
	return a == nil || a.Data == nil
}

func (*SingleAttestationPQ) IsSingle() bool {
	return true
}

func (*SingleAttestationPQ) IsAggregated() bool {
	return false
}

func (a *SingleAttestationPQ) Clone() Att {
	return a.Copy()
}

func (a *SingleAttestationPQ) Copy() *SingleAttestationPQ {
	if a == nil {
		return nil
	}
	return &SingleAttestationPQ{
		CommitteeId:   a.CommitteeId,
		AttesterIndex: a.AttesterIndex,
		Data:          a.Data.Copy(),
		Signature:     bytesutil.SafeCopyBytes(a.Signature),
	}
}

func (a *SingleAttestationPQ) GetAttestingIndex() primitives.ValidatorIndex {
	return a.AttesterIndex
}

func (a *SingleAttestationPQ) CommitteeBitsVal() bitfield.Bitfield {
	cb := primitives.NewAttestationCommitteeBitsPQ()
	cb.SetBitAt(uint64(a.CommitteeId), true)
	return cb
}

func (*SingleAttestationPQ) GetAggregationBits() bitfield.Bitlist {
	return nil
}

func (a *SingleAttestationPQ) SetSignature(sig []byte) {
	a.Signature = sig
}

func (a *SingleAttestationPQ) GetCommitteeIndex() primitives.CommitteeIndex {
	return a.CommitteeId
}

func (a *IndexedAttestationPQ) Version() int {
	return version.PQ
}

func (a *IndexedAttestationPQ) IsNil() bool {
	return a == nil || a.Data == nil
}

func (*IndexedAttestationPQ) GetSignature() []byte {
	return nil
}

func (a *IndexedAttestationPQ) Copy() *IndexedAttestationPQ {
	if a == nil {
		return nil
	}
	var indices []uint64
	if a.AttestingIndices != nil {
		indices = make([]uint64, len(a.AttestingIndices))
		copy(indices, a.AttestingIndices)
	}
	return &IndexedAttestationPQ{
		AttestingIndices: indices,
		Data:             a.Data.Copy(),
		Proof:            bytesutil.SafeCopyBytes(a.Proof),
	}
}

func (a *AttesterSlashingPQ) Version() int {
	return version.PQ
}

func (a *AttesterSlashingPQ) IsNil() bool {
	return a == nil || a.Attestation_1.IsNil() || a.Attestation_2.IsNil()
}

func (a *AttesterSlashingPQ) FirstAttestation() IndexedAtt {
	return a.Attestation_1
}

func (a *AttesterSlashingPQ) SecondAttestation() IndexedAtt {
	return a.Attestation_2
}

func (a *AttesterSlashingPQ) Copy() *AttesterSlashingPQ {
	if a == nil {
		return nil
	}
	return &AttesterSlashingPQ{
		Attestation_1: a.Attestation_1.Copy(),
		Attestation_2: a.Attestation_2.Copy(),
	}
}

func (a *AggregateAttestationAndProofPQ) Version() int {
	return version.PQ
}

func (a *AggregateAttestationAndProofPQ) IsNil() bool {
	return a == nil || a.Aggregate.IsNil()
}

func (a *AggregateAttestationAndProofPQ) AggregateVal() Att {
	return a.Aggregate
}

func (*AggregateAttestationAndProofPQ) GetSelectionProof() []byte {
	return nil
}

func (a *SignedAggregateAttestationAndProofPQ) Version() int {
	return version.PQ
}

func (a *SignedAggregateAttestationAndProofPQ) IsNil() bool {
	return a == nil || a.Message.IsNil()
}

func (a *SignedAggregateAttestationAndProofPQ) AggregateAttestationAndProof() AggregateAttAndProof {
	return a.Message
}

func (h *SignedBeaconBlockHeaderPQ) Copy() *SignedBeaconBlockHeaderPQ {
	if h == nil {
		return nil
	}
	return &SignedBeaconBlockHeaderPQ{
		Header:    h.Header.Copy(),
		Signature: bytesutil.SafeCopyBytes(h.Signature),
	}
}

func (s *ProposerSlashingPQ) Copy() *ProposerSlashingPQ {
	if s == nil {
		return nil
	}
	return &ProposerSlashingPQ{
		Header_1: s.Header_1.Copy(),
		Header_2: s.Header_2.Copy(),
	}
}

func (sb *SignedExecutionPayloadBidPQ) Copy() *SignedExecutionPayloadBidPQ {
	if sb == nil {
		return nil
	}
	return &SignedExecutionPayloadBidPQ{
		Message:   sb.Message.Copy(),
		Signature: bytesutil.SafeCopyBytes(sb.Signature),
	}
}

func CopyExecutionRequestsPQ(e *enginev1.ExecutionRequestsPQ) *enginev1.ExecutionRequestsPQ {
	if e == nil {
		return nil
	}
	gloas := CopyExecutionRequestsGloas(&enginev1.ExecutionRequestsGloas{
		Deposits:        e.Deposits,
		Withdrawals:     e.Withdrawals,
		Consolidations:  e.Consolidations,
		BuilderDeposits: e.BuilderDeposits,
		BuilderExits:    e.BuilderExits,
	})
	out := &enginev1.ExecutionRequestsPQ{
		Deposits:                  gloas.Deposits,
		Withdrawals:               gloas.Withdrawals,
		Consolidations:            gloas.Consolidations,
		BuilderDeposits:           gloas.BuilderDeposits,
		BuilderExits:              gloas.BuilderExits,
		ValidatorPreregistrations: make([]*enginev1.ValidatorPreregistrationRequest, len(e.ValidatorPreregistrations)),
		BuilderPreregistrations:   make([]*enginev1.BuilderPreregistrationRequest, len(e.BuilderPreregistrations)),
		VoluntaryExits:            make([]*enginev1.VoluntaryExitRequest, len(e.VoluntaryExits)),
	}
	for i, r := range e.ValidatorPreregistrations {
		out.ValidatorPreregistrations[i] = &enginev1.ValidatorPreregistrationRequest{
			Pubkey:                bytesutil.SafeCopyBytes(r.Pubkey),
			WithdrawalCredentials: bytesutil.SafeCopyBytes(r.WithdrawalCredentials),
			VrfRoot:               bytesutil.SafeCopyBytes(r.VrfRoot),
		}
	}
	for i, r := range e.BuilderPreregistrations {
		out.BuilderPreregistrations[i] = &enginev1.BuilderPreregistrationRequest{
			Pubkey:                bytesutil.SafeCopyBytes(r.Pubkey),
			WithdrawalCredentials: bytesutil.SafeCopyBytes(r.WithdrawalCredentials),
		}
	}
	for i, r := range e.VoluntaryExits {
		out.VoluntaryExits[i] = &enginev1.VoluntaryExitRequest{Pubkey: bytesutil.SafeCopyBytes(r.Pubkey)}
	}
	return out
}

func CopySignedBeaconBlockPQ(sb *SignedBeaconBlockPQ) *SignedBeaconBlockPQ {
	if sb == nil {
		return nil
	}
	return &SignedBeaconBlockPQ{
		Block:     copyBeaconBlockPQ(sb.Block),
		Signature: bytesutil.SafeCopyBytes(sb.Signature),
	}
}

func copyBeaconBlockPQ(b *BeaconBlockPQ) *BeaconBlockPQ {
	if b == nil {
		return nil
	}
	return &BeaconBlockPQ{
		Slot:          b.Slot,
		ProposerIndex: b.ProposerIndex,
		ParentRoot:    bytesutil.SafeCopyBytes(b.ParentRoot),
		StateRoot:     bytesutil.SafeCopyBytes(b.StateRoot),
		Body:          copyBeaconBlockBodyPQ(b.Body),
	}
}

func copyBeaconBlockBodyPQ(body *BeaconBlockBodyPQ) *BeaconBlockBodyPQ {
	if body == nil {
		return nil
	}
	return &BeaconBlockBodyPQ{
		RandaoReveal:              bytesutil.SafeCopyBytes(body.RandaoReveal),
		Eth1Data:                  body.Eth1Data.Copy(),
		Graffiti:                  bytesutil.SafeCopyBytes(body.Graffiti),
		ProposerSlashings:         CopySlice(body.ProposerSlashings),
		AttesterSlashings:         CopySlice(body.AttesterSlashings),
		Attestations:              CopySlice(body.Attestations),
		Deposits:                  CopySlice(body.Deposits),
		SignedExecutionPayloadBid: body.SignedExecutionPayloadBid.Copy(),
		ParentExecutionRequests:   CopyExecutionRequestsPQ(body.ParentExecutionRequests),
		Proof:                     bytesutil.SafeCopyBytes(body.Proof),
	}
}
