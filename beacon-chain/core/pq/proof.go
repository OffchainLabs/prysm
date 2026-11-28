package pq

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/xmsssigner"
	"github.com/pkg/errors"
)

var ErrInvalidAttestationProof = errors.New("invalid attestation proof")

var ErrProofBackendUnavailable = errors.New("proof backend unavailable")

// Spec: XMSSAggregateStatement
type Statement struct {
	Pubkeys   [][]byte
	LeafIndex uint32
	Message   [32]byte
}

// One input of MergeProofs, a proof and the statements it proves
type ProofChild struct {
	Statements []Statement
	Proof      []byte
}

// Spec: the stark module
type ProofBackend interface {
	Prove(statements []Statement, signatures [][][]byte) ([]byte, error)
	Merge(children []ProofChild) ([]byte, error)
	Verify(statements []Statement, proof []byte) error
	// A succinct proof does not grow with the signatures it covers, so packing needs no size budget.
	Succinct() bool
}

// Every client on a network must use the same backend.
var Proofs ProofBackend = concatenation{}

// The spec reference backend, a proof is the signatures in statement order.
type concatenation struct{}

func (concatenation) Prove(statements []Statement, signatures [][][]byte) ([]byte, error) {
	if len(statements) != len(signatures) {
		return nil, errors.Errorf("%d statements but %d signature lists", len(statements), len(signatures))
	}
	var proof []byte
	for i, sigs := range signatures {
		if len(sigs) != len(statements[i].Pubkeys) {
			return nil, errors.Errorf("statement %d has %d pubkeys but %d signatures", i, len(statements[i].Pubkeys), len(sigs))
		}
		for _, sig := range sigs {
			if len(sig) != XMSSSignatureLength {
				return nil, errors.Errorf("signature has length %d", len(sig))
			}
			proof = append(proof, sig...)
		}
	}
	return proof, nil
}

func (concatenation) Merge(children []ProofChild) ([]byte, error) {
	var out []byte
	for _, c := range children {
		out = append(out, c.Proof...)
	}
	return out, nil
}

func (concatenation) Verify(statements []Statement, proof []byte) error {
	offset := 0
	for _, s := range statements {
		for _, pubkey := range s.Pubkeys {
			if len(proof)-offset < XMSSSignatureLength {
				return errors.Wrap(ErrInvalidAttestationProof, "proof is too short")
			}
			if err := VerifyXMSS(pubkey, s.LeafIndex, s.Message, proof[offset:offset+XMSSSignatureLength]); err != nil {
				return errors.Wrap(ErrInvalidAttestationProof, err.Error())
			}
			offset += XMSSSignatureLength
		}
	}
	if offset != len(proof) {
		return errors.Wrap(ErrInvalidAttestationProof, "proof is too long")
	}
	return nil
}

func attestationMessage(st state.ReadOnlyBeaconState, data *ethpb.AttestationData) (uint32, [32]byte, error) {
	domain, err := signing.Domain(st.Fork(), data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
	if err != nil {
		return 0, [32]byte{}, err
	}
	message, err := signing.ComputeSigningRoot(data, domain)
	if err != nil {
		return 0, [32]byte{}, err
	}
	leafIndex, err := xmsssigner.AttestationLeafIndex(data.Target.Epoch)
	if err != nil {
		return 0, [32]byte{}, err
	}
	return leafIndex, message, nil
}

// Spec: get_attestation_statements
func AttestationStatements(ctx context.Context, st state.ReadOnlyBeaconState, att ethpb.Att) ([]Statement, error) {
	data := att.GetData()
	leafIndex, message, err := attestationMessage(st, data)
	if err != nil {
		return nil, err
	}
	bits := att.GetAggregationBits()
	committeeIndices := att.CommitteeBitsVal().BitIndices()
	statements := make([]Statement, 0, len(committeeIndices))
	offset := uint64(0)
	for _, ci := range committeeIndices {
		committee, err := helpers.BeaconCommitteeFromState(ctx, st, data.Slot, primitives.CommitteeIndex(ci))
		if err != nil {
			return nil, err
		}
		if offset+uint64(len(committee)) > bits.Len() {
			return nil, errors.New("aggregation bits are shorter than the committees")
		}
		var pubkeys [][]byte
		for i, index := range committee {
			if bits.BitAt(offset + uint64(i)) {
				pubkey := st.PubkeyAtIndex(index)
				pubkeys = append(pubkeys, pubkey[:])
			}
		}
		statements = append(statements, Statement{Pubkeys: pubkeys, LeafIndex: leafIndex, Message: message})
		offset += uint64(len(committee))
	}
	return statements, nil
}

// Spec: process_stark
func VerifyAttestationProof(ctx context.Context, st state.ReadOnlyBeaconState, atts []ethpb.Att, proof []byte) error {
	var statements []Statement
	for _, att := range atts {
		s, err := AttestationStatements(ctx, st, att)
		if err != nil {
			return err
		}
		statements = append(statements, s...)
	}
	return verifyProof("block", statements, proof)
}

// Spec: is_valid_indexed_attestation, the proof check
func VerifyIndexedAttestationProof(st state.ReadOnlyBeaconState, att *ethpb.IndexedAttestationPQ) error {
	leafIndex, message, err := attestationMessage(st, att.Data)
	if err != nil {
		return err
	}
	pubkeys := make([][]byte, len(att.AttestingIndices))
	for i, index := range att.AttestingIndices {
		pubkey := st.PubkeyAtIndex(primitives.ValidatorIndex(index))
		pubkeys[i] = pubkey[:]
	}
	return verifyProof("indexed", []Statement{{Pubkeys: pubkeys, LeafIndex: leafIndex, Message: message}}, att.Proof)
}

// Spec: the signature check of the beacon_attestation gossip rule
func VerifySingleAttestation(st state.ReadOnlyBeaconState, att *ethpb.SingleAttestationPQ) error {
	leafIndex, message, err := attestationMessage(st, att.Data)
	if err != nil {
		return err
	}
	pubkey := st.PubkeyAtIndex(att.AttesterIndex)
	return VerifyXMSS(pubkey[:], leafIndex, message, att.Signature)
}

func (concatenation) Succinct() bool { return false }
