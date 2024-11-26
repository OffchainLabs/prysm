package eth

import (
	"fmt"

	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"google.golang.org/protobuf/proto"
)

// Att defines common functionality for all attestation types.
type Att interface {
	proto.Message
	ssz.Marshaler
	ssz.Unmarshaler
	ssz.HashRoot
	Version() int
	Clone() Att
	GetAggregationBits() bitfield.Bitlist
	GetData() *AttestationData
	CommitteeBitsVal() bitfield.Bitfield
	GetSignature() []byte
	GetCommitteeIndex() (primitives.CommitteeIndex, error)
	IsNil() bool
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

// CommitteeBitsVal --
func (a *Attestation) CommitteeBitsVal() bitfield.Bitfield {
	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(uint64(a.Data.CommitteeIndex), true)
	return cb
}

// GetCommitteeIndex --
func (a *Attestation) GetCommitteeIndex() (primitives.CommitteeIndex, error) {
	if a == nil || a.Data == nil {
		return 0, errors.New("nil attestation data")
	}
	return a.Data.CommitteeIndex, nil
}

// Version --
func (a *PendingAttestation) Version() int {
	return version.Phase0
}

// IsNil --
func (a *PendingAttestation) IsNil() bool {
	return a == nil || a.Data == nil
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

// CommitteeBitsVal --
func (a *PendingAttestation) CommitteeBitsVal() bitfield.Bitfield {
	return nil
}

// GetSignature --
func (a *PendingAttestation) GetSignature() []byte {
	return nil
}

// GetCommitteeIndex --
func (a *PendingAttestation) GetCommitteeIndex() (primitives.CommitteeIndex, error) {
	if a == nil || a.Data == nil {
		return 0, errors.New("nil attestation data")
	}
	return a.Data.CommitteeIndex, nil
}

// Version --
func (a *AttestationElectra) Version() int {
	return version.Electra
}

// IsNil --
func (a *AttestationElectra) IsNil() bool {
	return a == nil || a.Data == nil
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

// CommitteeBitsVal --
func (a *AttestationElectra) CommitteeBitsVal() bitfield.Bitfield {
	return a.CommitteeBits
}

// GetCommitteeIndex --
func (a *AttestationElectra) GetCommitteeIndex() (primitives.CommitteeIndex, error) {
	if a == nil || a.Data == nil {
		return 0, errors.New("nil attestation data")
	}
	if len(a.CommitteeBits) == 0 {
		return 0, errors.New("no committee bits found in attestation")
	}
	if a.Data.CommitteeIndex != 0 {
		return 0, fmt.Errorf("attestation data's committee index must be 0 but was %d", a.Data.CommitteeIndex)
	}
	indices := a.CommitteeBits.BitIndices()
	if len(indices) != 1 {
		return 0, fmt.Errorf("exactly 1 committee index must be set but %d were set", len(indices))
	}
	return primitives.CommitteeIndex(uint64(indices[0])), nil
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
