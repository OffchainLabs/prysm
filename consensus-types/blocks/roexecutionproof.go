package blocks

import (
	"github.com/pkg/errors"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

var (
	errNilExecutionProofEnvelope     = errors.New("received nil execution proof envelope")
	errNilExecutionProof             = errors.New("received nil signed execution proof envelope")
	errNilExecutionPayloadEnvelope   = errors.New("received nil execution payload envelope")
	errEmptyExecutionProof           = errors.New("execution proof carries no proof data")
	errUnsupportedExecutionProofType = errors.New("execution proof names an unsupported proof type")
)

type ROSignedExecutionProofEnvelope struct {
	*ethpb.SignedExecutionProofEnvelope
}

// NewROSignedExecutionProofEnvelope wraps a signed envelope after checking that
// it is well formed: its fields have the right lengths, its proof data is not
// empty and its proof type is supported. The SSZ bound enforces MAX_PROOF_SIZE.
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
	if len(p.Message.ProofData) == 0 {
		return ROSignedExecutionProofEnvelope{}, errEmptyExecutionProof
	}
	if proofType := ethpb.ProofType(p.Message.ProofType[0]); !proofType.Supported() {
		return ROSignedExecutionProofEnvelope{}, errors.Wrapf(errUnsupportedExecutionProofType, "%s", proofType)
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
	return ethpb.ProofType(p.Message.ProofType[0])
}
