package blocks

import (
	"errors"
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

var (
	errNilExecutionProof          = errors.New("execution proof is nil")
	errEmptyProofData             = errors.New("proof data is empty")
	errEmptyNewPayloadRequestRoot = errors.New("new payload request root is empty")
)

// ROExecutionProof represents a read-only execution proof with its block root.
type ROSignedExecutionProof struct {
	*ethpb.SignedExecutionProof
	blockRoot [fieldparams.RootLength]byte
	epoch     primitives.Epoch
}

func roSignedExecutionProofNilCheck(sep *ethpb.SignedExecutionProof) error {
	if sep == nil {
		return errNilExecutionProof
	}

	ep := sep.Message

	if len(ep.ProofData) == 0 {
		return errEmptyProofData
	}

	if len(ep.PublicInput.NewPayloadRequestRoot) == 0 {
		return errEmptyNewPayloadRequestRoot
	}

	return nil
}

// NewROSignedExecutionProofWithRoot creates a new ROSignedExecutionProof with a given root.
func NewROSignedExecutionProof(
	signedExecutionProof *ethpb.SignedExecutionProof,
	root [fieldparams.RootLength]byte,
	epoch primitives.Epoch,
) (ROSignedExecutionProof, error) {
	if err := roSignedExecutionProofNilCheck(signedExecutionProof); err != nil {
		return ROSignedExecutionProof{}, fmt.Errorf("ro signed execution proof nil check: %w", err)
	}

	roSignedExecutionProof := ROSignedExecutionProof{
		SignedExecutionProof: signedExecutionProof,
		blockRoot:            root,
		epoch:                epoch,
	}

	return roSignedExecutionProof, nil
}

// BlockRoot returns the block root of the execution proof.
func (p *ROSignedExecutionProof) BlockRoot() [fieldparams.RootLength]byte {
	return p.blockRoot
}

// Epoch returns the epoch of the execution proof.
func (p *ROSignedExecutionProof) Epoch() primitives.Epoch {
	return p.epoch
}

// // ProofType returns the proof type of the execution proof.
// func (p *ROExecutionProof) ProofType() primitives.ProofType {
// 	return p.ExecutionProof.ProofType
// }

// VerifiedROExecutionProof represents an ROExecutionProof that has undergone full verification.
type VerifiedROSignedExecutionProof struct {
	ROSignedExecutionProof
}

// NewVerifiedROExecutionProof "upgrades" an ROExecutionProof to a VerifiedROExecutionProof.
// This method should only be used by the verification package.
func NewVerifiedROSignedExecutionProof(ro ROSignedExecutionProof) VerifiedROSignedExecutionProof {
	return VerifiedROSignedExecutionProof{ROSignedExecutionProof: ro}
}
