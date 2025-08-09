package eth

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/go-bitfield"
	"google.golang.org/protobuf/proto"
)

// Att defines common functionality for all attestation types.
type Att interface {
	proto.Message
	ssz.Marshaler
	ssz.Unmarshaler
	ssz.HashRoot
	Version() int
	IsNil() bool
	IsSingle() bool
	IsAggregated() bool
	Clone() Att
	GetAggregationBits() bitfield.Bitlist
	GetAttestingIndex() primitives.ValidatorIndex
	GetData() *AttestationData
	CommitteeBitsVal() bitfield.Bitfield
	GetSignature() []byte
	SetSignature(sig []byte)
	GetCommitteeIndex() primitives.CommitteeIndex
}

// IndexedAtt defines common functionality for all indexed attestation types.
type IndexedAtt interface {
	proto.Message
	ssz.Marshaler
	ssz.Unmarshaler
	ssz.HashRoot
	Version() int
	GetAttestingIndices() []uint64
	GetData() *AttestationData
	GetSignature() []byte
	IsNil() bool
}

// SignedAggregateAttAndProof defines common functionality for all signed aggregate attestation types.
type SignedAggregateAttAndProof interface {
	proto.Message
	ssz.Marshaler
	ssz.Unmarshaler
	ssz.HashRoot
	Version() int
	AggregateAttestationAndProof() AggregateAttAndProof
	GetSignature() []byte
	IsNil() bool
}

// AggregateAttAndProof defines common functionality for all aggregate attestation types.
type AggregateAttAndProof interface {
	proto.Message
	ssz.Marshaler
	ssz.Unmarshaler
	ssz.HashRoot
	Version() int
	GetAggregatorIndex() primitives.ValidatorIndex
	AggregateVal() Att
	GetSelectionProof() []byte
	IsNil() bool
}

// AttSlashing defines common functionality for all attestation slashing types.
type AttSlashing interface {
	proto.Message
	ssz.Marshaler
	ssz.Unmarshaler
	ssz.HashRoot
	Version() int
	FirstAttestation() IndexedAtt
	SecondAttestation() IndexedAtt
	IsNil() bool
}

// Copy --
func (cp *Checkpoint) Copy() *Checkpoint {
	if cp == nil {
		return nil
	}
	return &Checkpoint{
		Epoch: cp.Epoch,
		Root:  bytesutil.SafeCopyBytes(cp.Root),
	}
}

// Copy --
func (attData *AttestationData) Copy() *AttestationData {
	if attData == nil {
		return nil
	}
	return &AttestationData{
		Slot:            attData.Slot,
		CommitteeIndex:  attData.CommitteeIndex,
		BeaconBlockRoot: bytesutil.SafeCopyBytes(attData.BeaconBlockRoot),
		Source:          attData.Source.Copy(),
		Target:          attData.Target.Copy(),
	}
}

// Version --
func (a *Attestation) Version() int {
	return version.Phase0
}

// IsNil --
func (a *Attestation) IsNil() bool {
	return a == nil || a.Data == nil
}

// IsSingle returns true when the attestation can have only a single attester index.
func (*Attestation) IsSingle() bool {
	return false
}

// IsAggregated --
func (a *Attestation) IsAggregated() bool {
	return a.AggregationBits.Count() > 1
}

// Clone --
func (a *Attestation) Clone() Att {
	return a.Copy()
}

// Copy --
func (a *Attestation) Copy() *Attestation {
	if a == nil {
		return nil
	}
	return &Attestation{
		AggregationBits: bytesutil.SafeCopyBytes(a.AggregationBits),
		Data:            a.Data.Copy(),
		Signature:       bytesutil.SafeCopyBytes(a.Signature),
	}
}

// GetAttestingIndex --
func (*Attestation) GetAttestingIndex() primitives.ValidatorIndex {
	return 0
}

// CommitteeBitsVal --
func (a *Attestation) CommitteeBitsVal() bitfield.Bitfield {
	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(uint64(a.Data.CommitteeIndex), true)
	return cb
}

// SetSignature --
func (a *Attestation) SetSignature(sig []byte) {
	a.Signature = sig
}

// GetCommitteeIndex --
func (a *Attestation) GetCommitteeIndex() primitives.CommitteeIndex {
	if a == nil || a.Data == nil {
		return 0
	}
	return a.Data.CommitteeIndex
}

// Version --
func (a *PendingAttestation) Version() int {
	return version.Phase0
}

// IsNil --
func (a *PendingAttestation) IsNil() bool {
	return a == nil || a.Data == nil
}

// IsSingle returns true when the attestation can have only a single attester index.
func (*PendingAttestation) IsSingle() bool {
	return false
}

// IsAggregated --
func (a *PendingAttestation) IsAggregated() bool {
	return a.AggregationBits.Count() > 1
}

// Clone --
func (a *PendingAttestation) Clone() Att {
	return a.Copy()
}

// Copy --
func (a *PendingAttestation) Copy() *PendingAttestation {
	if a == nil {
		return nil
	}
	return &PendingAttestation{
		AggregationBits: bytesutil.SafeCopyBytes(a.AggregationBits),
		Data:            a.Data.Copy(),
		InclusionDelay:  a.InclusionDelay,
		ProposerIndex:   a.ProposerIndex,
	}
}

// GetAttestingIndex --
func (*PendingAttestation) GetAttestingIndex() primitives.ValidatorIndex {
	return 0
}

// CommitteeBitsVal --
func (a *PendingAttestation) CommitteeBitsVal() bitfield.Bitfield {
	return nil
}

// GetSignature --
func (a *PendingAttestation) GetSignature() []byte {
	return nil
}

// SetSignature --
func (a *PendingAttestation) SetSignature(_ []byte) {}

// GetCommitteeIndex --
func (a *PendingAttestation) GetCommitteeIndex() primitives.CommitteeIndex {
	if a == nil || a.Data == nil {
		return 0
	}
	return a.Data.CommitteeIndex
}

// Version --
func (a *AttestationElectra) Version() int {
	return version.Electra
}

// IsNil --
func (a *AttestationElectra) IsNil() bool {
	return a == nil || a.Data == nil
}

// IsSingle returns true when the attestation can have only a single attester index.
func (*AttestationElectra) IsSingle() bool {
	return false
}

// IsAggregated --
func (a *AttestationElectra) IsAggregated() bool {
	return a.AggregationBits.Count() > 1
}

// Clone --
func (a *AttestationElectra) Clone() Att {
	return a.Copy()
}

// Copy --
func (a *AttestationElectra) Copy() *AttestationElectra {
	if a == nil {
		return nil
	}
	return &AttestationElectra{
		AggregationBits: bytesutil.SafeCopyBytes(a.AggregationBits),
		CommitteeBits:   bytesutil.SafeCopyBytes(a.CommitteeBits),
		Data:            a.Data.Copy(),
		Signature:       bytesutil.SafeCopyBytes(a.Signature),
	}
}

// GetAttestingIndex --
func (*AttestationElectra) GetAttestingIndex() primitives.ValidatorIndex {
	return 0
}

// CommitteeBitsVal --
func (a *AttestationElectra) CommitteeBitsVal() bitfield.Bitfield {
	return a.CommitteeBits
}

// SetSignature --
func (a *AttestationElectra) SetSignature(sig []byte) {
	a.Signature = sig
}

// GetCommitteeIndex --
func (a *AttestationElectra) GetCommitteeIndex() primitives.CommitteeIndex {
	if len(a.CommitteeBits) == 0 {
		return 0
	}
	indices := a.CommitteeBits.BitIndices()
	if len(indices) == 0 {
		return 0
	} else if len(indices) != 1 {
		log.WithField("indices", a.CommitteeBits).Debugf("expected 1 committee bit indice got %d", len(indices))
	}
	return primitives.CommitteeIndex(uint64(indices[0]))
}

// Version --
func (a *SingleAttestation) Version() int {
	return version.Electra
}

// IsNil --
func (a *SingleAttestation) IsNil() bool {
	return a == nil || a.Data == nil
}

// IsAggregated --
func (a *SingleAttestation) IsAggregated() bool {
	return false
}

// IsSingle returns true when the attestation can have only a single attester index.
func (*SingleAttestation) IsSingle() bool {
	return true
}

// Clone --
func (a *SingleAttestation) Clone() Att {
	return a.Copy()
}

// Copy --
func (a *SingleAttestation) Copy() *SingleAttestation {
	if a == nil {
		return nil
	}
	return &SingleAttestation{
		CommitteeId:   a.CommitteeId,
		AttesterIndex: a.AttesterIndex,
		Data:          a.Data.Copy(),
		Signature:     bytesutil.SafeCopyBytes(a.Signature),
	}
}

// GetAttestingIndex --
func (a *SingleAttestation) GetAttestingIndex() primitives.ValidatorIndex {
	return a.AttesterIndex
}

// CommitteeBitsVal --
func (a *SingleAttestation) CommitteeBitsVal() bitfield.Bitfield {
	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(uint64(a.CommitteeId), true)
	return cb
}

// GetAggregationBits --
func (a *SingleAttestation) GetAggregationBits() bitfield.Bitlist {
	return nil
}

// SetSignature --
func (a *SingleAttestation) SetSignature(sig []byte) {
	a.Signature = sig
}

// GetCommitteeIndex --
func (a *SingleAttestation) GetCommitteeIndex() primitives.CommitteeIndex {
	return a.CommitteeId
}

// ToAttestationElectra converts the attestation to an AttestationElectra.
func (a *SingleAttestation) ToAttestationElectra(committee []primitives.ValidatorIndex) *AttestationElectra {
	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(uint64(a.CommitteeId), true)

	ab := bitfield.NewBitlist(uint64(len(committee)))
	for i, ix := range committee {
		if a.AttesterIndex == ix {
			ab.SetBitAt(uint64(i), true)
			break
		}
	}

	return &AttestationElectra{
		AggregationBits: ab,
		Data:            a.Data,
		Signature:       a.Signature,
		CommitteeBits:   cb,
	}
}

// Version --
func (a *IndexedAttestation) Version() int {
	return version.Phase0
}

// IsNil --
func (a *IndexedAttestation) IsNil() bool {
	return a == nil || a.Data == nil
}

// Version --
func (a *IndexedAttestationElectra) Version() int {
	return version.Electra
}

// IsNil --
func (a *IndexedAttestationElectra) IsNil() bool {
	return a == nil || a.Data == nil
}

// Copy --
func (a *IndexedAttestation) Copy() *IndexedAttestation {
	var indices []uint64
	if a == nil {
		return nil
	} else if a.AttestingIndices != nil {
		indices = make([]uint64, len(a.AttestingIndices))
		copy(indices, a.AttestingIndices)
	}
	return &IndexedAttestation{
		AttestingIndices: indices,
		Data:             a.Data.Copy(),
		Signature:        bytesutil.SafeCopyBytes(a.Signature),
	}
}

// Copy --
func (a *IndexedAttestationElectra) Copy() *IndexedAttestationElectra {
	var indices []uint64
	if a == nil {
		return nil
	} else if a.AttestingIndices != nil {
		indices = make([]uint64, len(a.AttestingIndices))
		copy(indices, a.AttestingIndices)
	}
	return &IndexedAttestationElectra{
		AttestingIndices: indices,
		Data:             a.Data.Copy(),
		Signature:        bytesutil.SafeCopyBytes(a.Signature),
	}
}

// Version --
func (a *AttesterSlashing) Version() int {
	return version.Phase0
}

// IsNil --
func (a *AttesterSlashing) IsNil() bool {
	return a == nil ||
		a.Attestation_1 == nil || a.Attestation_1.IsNil() ||
		a.Attestation_2 == nil || a.Attestation_2.IsNil()
}

// FirstAttestation --
func (a *AttesterSlashing) FirstAttestation() IndexedAtt {
	return a.Attestation_1
}

// SecondAttestation --
func (a *AttesterSlashing) SecondAttestation() IndexedAtt {
	return a.Attestation_2
}

// Version --
func (a *AttesterSlashingElectra) Version() int {
	return version.Electra
}

// IsNil --
func (a *AttesterSlashingElectra) IsNil() bool {
	return a == nil ||
		a.Attestation_1 == nil || a.Attestation_1.IsNil() ||
		a.Attestation_2 == nil || a.Attestation_2.IsNil()
}

// FirstAttestation --
func (a *AttesterSlashingElectra) FirstAttestation() IndexedAtt {
	return a.Attestation_1
}

// SecondAttestation --
func (a *AttesterSlashingElectra) SecondAttestation() IndexedAtt {
	return a.Attestation_2
}

func (a *AttesterSlashing) Copy() *AttesterSlashing {
	if a == nil {
		return nil
	}
	return &AttesterSlashing{
		Attestation_1: a.Attestation_1.Copy(),
		Attestation_2: a.Attestation_2.Copy(),
	}
}

// Copy --
func (a *AttesterSlashingElectra) Copy() *AttesterSlashingElectra {
	if a == nil {
		return nil
	}
	return &AttesterSlashingElectra{
		Attestation_1: a.Attestation_1.Copy(),
		Attestation_2: a.Attestation_2.Copy(),
	}
}

// Version --
func (a *AggregateAttestationAndProof) Version() int {
	return version.Phase0
}

// IsNil --
func (a *AggregateAttestationAndProof) IsNil() bool {
	return a == nil || a.Aggregate == nil || a.Aggregate.IsNil()
}

// AggregateVal --
func (a *AggregateAttestationAndProof) AggregateVal() Att {
	return a.Aggregate
}

// Version --
func (a *AggregateAttestationAndProofElectra) Version() int {
	return version.Electra
}

// IsNil --
func (a *AggregateAttestationAndProofElectra) IsNil() bool {
	return a == nil || a.Aggregate == nil || a.Aggregate.IsNil()
}

// AggregateVal --
func (a *AggregateAttestationAndProofElectra) AggregateVal() Att {
	return a.Aggregate
}

// Version --
func (a *SignedAggregateAttestationAndProof) Version() int {
	return version.Phase0
}

// IsNil --
func (a *SignedAggregateAttestationAndProof) IsNil() bool {
	return a == nil || a.Message == nil || a.Message.IsNil()
}

// AggregateAttestationAndProof --
func (a *SignedAggregateAttestationAndProof) AggregateAttestationAndProof() AggregateAttAndProof {
	return a.Message
}

// Version --
func (a *SignedAggregateAttestationAndProofElectra) Version() int {
	return version.Electra
}

// IsNil --
func (a *SignedAggregateAttestationAndProofElectra) IsNil() bool {
	return a == nil || a.Message == nil || a.Message.IsNil()
}

// AggregateAttestationAndProof --
func (a *SignedAggregateAttestationAndProofElectra) AggregateAttestationAndProof() AggregateAttAndProof {
	return a.Message
}
