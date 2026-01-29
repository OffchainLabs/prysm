package blocks

import (
	"errors"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

var (
	errNilExecutionProof    = errors.New("execution proof is nil")
	errEmptyBlockRoot       = errors.New("block root is empty")
	errInvalidBlockRootSize = errors.New("block root has invalid size")
	errInvalidBlockHashSize = errors.New("block hash has invalid size")
)

// ROExecutionProof represents a read-only execution proof with its block root.
type ROExecutionProof struct {
	*ethpb.ExecutionProof
	blockRoot [fieldparams.RootLength]byte
}

func roExecutionProofNilCheck(ep *ethpb.ExecutionProof) error {
	if ep == nil {
		return errNilExecutionProof
	}

	if len(ep.BlockRoot) == 0 {
		return errEmptyBlockRoot
	}

	if len(ep.BlockRoot) != fieldparams.RootLength {
		return errInvalidBlockRootSize
	}

	if len(ep.BlockHash) != fieldparams.RootLength {
		return errInvalidBlockHashSize
	}

	return nil
}

// NewROExecutionProof creates a new ROExecutionProof from the given ExecutionProof.
// The block root is extracted from the ExecutionProof's BlockRoot field.
func NewROExecutionProof(ep *ethpb.ExecutionProof) (ROExecutionProof, error) {
	if err := roExecutionProofNilCheck(ep); err != nil {
		return ROExecutionProof{}, err
	}

	return ROExecutionProof{
		ExecutionProof: ep,
		blockRoot:      bytesutil.ToBytes32(ep.BlockRoot),
	}, nil
}

// NewROExecutionProofWithRoot creates a new ROExecutionProof with a given root.
func NewROExecutionProofWithRoot(ep *ethpb.ExecutionProof, root [fieldparams.RootLength]byte) (ROExecutionProof, error) {
	if err := roExecutionProofNilCheck(ep); err != nil {
		return ROExecutionProof{}, err
	}

	return ROExecutionProof{
		ExecutionProof: ep,
		blockRoot:      root,
	}, nil
}

// BlockRoot returns the block root of the execution proof.
func (p *ROExecutionProof) BlockRoot() [fieldparams.RootLength]byte {
	return p.blockRoot
}

// Slot returns the slot of the execution proof.
func (p *ROExecutionProof) Slot() primitives.Slot {
	return p.ExecutionProof.Slot
}

// ProofId returns the proof ID of the execution proof.
func (p *ROExecutionProof) ProofId() primitives.ExecutionProofId {
	return p.ExecutionProof.ProofId
}

// BlockHash returns the block hash of the execution proof.
func (p *ROExecutionProof) BlockHash() [32]byte {
	return bytesutil.ToBytes32(p.ExecutionProof.BlockHash)
}

// VerifiedROExecutionProof represents an ROExecutionProof that has undergone full verification.
type VerifiedROExecutionProof struct {
	ROExecutionProof
}

// NewVerifiedROExecutionProof "upgrades" an ROExecutionProof to a VerifiedROExecutionProof.
// This method should only be used by the verification package.
func NewVerifiedROExecutionProof(ro ROExecutionProof) VerifiedROExecutionProof {
	return VerifiedROExecutionProof{ROExecutionProof: ro}
}
