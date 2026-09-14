package blocks

import (
	"github.com/pkg/errors"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

var (
	errNilExecutionProofEnvelope   = errors.New("received nil execution proof envelope")
	errNilExecutionProof           = errors.New("received nil signed execution proof envelope")
	errNilExecutionPayloadEnvelope = errors.New("received nil execution payload envelope")
)

type ROSignedExecutionProofEnvelope struct {
	*ethpb.SignedExecutionProofEnvelope
}

// NewROSignedExecutionProofEnvelope wraps a signed envelope after checking that
// it is well formed.
func NewROSignedExecutionProofEnvelope(p *ethpb.SignedExecutionProofEnvelope) (ROSignedExecutionProofEnvelope, error) {
	if p == nil {
		return ROSignedExecutionProofEnvelope{}, errNilExecutionProof
	}
	if p.Message == nil {
		return ROSignedExecutionProofEnvelope{}, errNilExecutionProofEnvelope
	}
	if len(p.Message.BeaconBlockRoot) != fieldparams.RootLength {
		return ROSignedExecutionProofEnvelope{}, errors.Errorf(
			"execution proof beacon block root has length %d, want %d",
			len(p.Message.BeaconBlockRoot), fieldparams.RootLength,
		)
	}
	if len(p.Message.ProofType) != 1 {
		return ROSignedExecutionProofEnvelope{}, errors.Errorf(
			"execution proof type has length %d, want 1", len(p.Message.ProofType),
		)
	}
	return ROSignedExecutionProofEnvelope{SignedExecutionProofEnvelope: p}, nil
}

// EnvelopeRoot returns the hash tree root of the envelope. EIP-8025 uses it to
// recognise a proof that has already been processed.
func (p ROSignedExecutionProofEnvelope) EnvelopeRoot() ([fieldparams.RootLength]byte, error) {
	return p.Message.HashTreeRoot()
}

// BeaconBlockRoot returns the beacon block whose payload this proof attests to.
func (p ROSignedExecutionProofEnvelope) BeaconBlockRoot() [fieldparams.RootLength]byte {
	return [fieldparams.RootLength]byte(p.Message.BeaconBlockRoot)
}

// ProofType returns the proof system, guest program and version identifier.
func (p ROSignedExecutionProofEnvelope) ProofType() ethpb.ProofType {
	return p.Message.ProofTypeValue()
}

// VerifiedROSignedExecutionProofEnvelope is a signed execution proof envelope
// that has passed every EIP-8025 gossip check, including verification of the
// proof itself by the proof engine.
type VerifiedROSignedExecutionProofEnvelope struct {
	ROSignedExecutionProofEnvelope
}

// NewVerifiedROSignedExecutionProofEnvelope marks an envelope as verified. Only
// call it once the proof engine has accepted the proof.
func NewVerifiedROSignedExecutionProofEnvelope(p ROSignedExecutionProofEnvelope) VerifiedROSignedExecutionProofEnvelope {
	return VerifiedROSignedExecutionProofEnvelope{ROSignedExecutionProofEnvelope: p}
}
