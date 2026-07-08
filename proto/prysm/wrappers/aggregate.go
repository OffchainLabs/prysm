package wrappers

import (
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Gloas and Electra aggregate-and-proof (and their nested attestation) types
// share an identical wire shape; they differ only in merkleization — the Gloas
// attestation is a ProgressiveContainer, the Electra one a standard container.
// These helpers convert between them at the boundary: decode/produce the Gloas
// form at the edge (so hash_tree_root and signatures are progressive-correct),
// and convert to the Electra form just before feeding value into existing
// Electra-typed processing. All conversions deep-copy so the result never
// aliases the input's backing slices. Each returns nil for a nil input.

// AttestationElectraToGloas re-types an Electra attestation as the Gloas form.
func AttestationElectraToGloas(a *ethpb.AttestationElectra) *ethpb.AttestationGloas {
	if a == nil {
		return nil
	}
	return &ethpb.AttestationGloas{
		AggregationBits: bytesutil.SafeCopyBytes(a.AggregationBits),
		Data:            a.Data.Copy(),
		Signature:       bytesutil.SafeCopyBytes(a.Signature),
		CommitteeBits:   bytesutil.SafeCopyBytes(a.CommitteeBits),
	}
}

// AttestationGloasToElectra re-types a Gloas attestation as the Electra form.
func AttestationGloasToElectra(a *ethpb.AttestationGloas) *ethpb.AttestationElectra {
	if a == nil {
		return nil
	}
	return &ethpb.AttestationElectra{
		AggregationBits: bytesutil.SafeCopyBytes(a.AggregationBits),
		Data:            a.Data.Copy(),
		Signature:       bytesutil.SafeCopyBytes(a.Signature),
		CommitteeBits:   bytesutil.SafeCopyBytes(a.CommitteeBits),
	}
}

// AggregateAttestationAndProofElectraToGloas re-types an Electra
// aggregate-and-proof as the Gloas form (nested attestation converted too).
func AggregateAttestationAndProofElectraToGloas(a *ethpb.AggregateAttestationAndProofElectra) *ethpb.AggregateAttestationAndProofGloas {
	if a == nil {
		return nil
	}
	return &ethpb.AggregateAttestationAndProofGloas{
		AggregatorIndex: a.AggregatorIndex,
		Aggregate:       AttestationElectraToGloas(a.Aggregate),
		SelectionProof:  bytesutil.SafeCopyBytes(a.SelectionProof),
	}
}

// AggregateAttestationAndProofGloasToElectra re-types a Gloas
// aggregate-and-proof as the Electra form.
func AggregateAttestationAndProofGloasToElectra(a *ethpb.AggregateAttestationAndProofGloas) *ethpb.AggregateAttestationAndProofElectra {
	if a == nil {
		return nil
	}
	return &ethpb.AggregateAttestationAndProofElectra{
		AggregatorIndex: a.AggregatorIndex,
		Aggregate:       AttestationGloasToElectra(a.Aggregate),
		SelectionProof:  bytesutil.SafeCopyBytes(a.SelectionProof),
	}
}

// SignedAggregateAttestationAndProofElectraToGloas re-types a signed Electra
// aggregate-and-proof as the Gloas form.
func SignedAggregateAttestationAndProofElectraToGloas(a *ethpb.SignedAggregateAttestationAndProofElectra) *ethpb.SignedAggregateAttestationAndProofGloas {
	if a == nil {
		return nil
	}
	return &ethpb.SignedAggregateAttestationAndProofGloas{
		Message:   AggregateAttestationAndProofElectraToGloas(a.Message),
		Signature: bytesutil.SafeCopyBytes(a.Signature),
	}
}

// SignedAggregateAttestationAndProofGloasToElectra re-types a signed Gloas
// aggregate-and-proof as the Electra form.
func SignedAggregateAttestationAndProofGloasToElectra(a *ethpb.SignedAggregateAttestationAndProofGloas) *ethpb.SignedAggregateAttestationAndProofElectra {
	if a == nil {
		return nil
	}
	return &ethpb.SignedAggregateAttestationAndProofElectra{
		Message:   AggregateAttestationAndProofGloasToElectra(a.Message),
		Signature: bytesutil.SafeCopyBytes(a.Signature),
	}
}
