package eth

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// Version returns the fork version this container was introduced in.
func (a *BatchAttestation) Version() int {
	return version.BatchAttestation
}

// IsNil returns true when this container is missing required fields.
func (a *BatchAttestation) IsNil() bool {
	return a == nil || a.Data == nil
}

// IsSingle reports whether the container represents a single attester. Batch
// containers carry N ≥ 2 attesters by construction; gossip validation rejects
// batches with fewer than 2 bits set.
func (*BatchAttestation) IsSingle() bool {
	return false
}

// IsAggregated reports whether this is a multi-attester aggregate. Batches
// always carry pre-aggregated votes.
func (*BatchAttestation) IsAggregated() bool {
	return true
}

// Clone returns a deep copy of this batch as an Att.
func (a *BatchAttestation) Clone() Att {
	return a.Copy()
}

// Copy returns a deep copy of this batch.
func (a *BatchAttestation) Copy() *BatchAttestation {
	if a == nil {
		return nil
	}
	return &BatchAttestation{
		CommitteeIndex:   a.CommitteeIndex,
		AggregationBits:  bytesutil.SafeCopyBytes(a.AggregationBits),
		Data:             a.Data.Copy(),
		Signature:        bytesutil.SafeCopyBytes(a.Signature),
		Batcher:          a.Batcher,
		BatchSeal:        bytesutil.SafeCopyBytes(a.BatchSeal),
		BatcherSignature: bytesutil.SafeCopyBytes(a.BatcherSignature),
	}
}

// GetAttestingIndex returns a sentinel zero — batches carry multiple
// attesters so there is no single attesting index.
func (*BatchAttestation) GetAttestingIndex() primitives.ValidatorIndex {
	return 0
}

// CommitteeBitsVal returns a 1-hot vector marking this batch's single
// committee. Lets us reuse single-committee aggregation paths downstream.
func (a *BatchAttestation) CommitteeBitsVal() bitfield.Bitfield {
	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(uint64(a.CommitteeIndex), true)
	return cb
}

// SetSignature replaces the aggregate attestation signature.
func (a *BatchAttestation) SetSignature(sig []byte) {
	a.Signature = sig
}

// ToAttestationElectra turns this batch into the pool-native AttestationElectra
// shape. The gossip-only batch_seal and batcher_signature fields are stripped
// per EIP-8243; the on-chain Attestation container is unchanged.
func (a *BatchAttestation) ToAttestationElectra() *AttestationElectra {
	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(uint64(a.CommitteeIndex), true)
	return &AttestationElectra{
		AggregationBits: bytesutil.SafeCopyBytes(a.AggregationBits),
		Data:            a.Data,
		Signature:       bytesutil.SafeCopyBytes(a.Signature),
		CommitteeBits:   cb,
	}
}
