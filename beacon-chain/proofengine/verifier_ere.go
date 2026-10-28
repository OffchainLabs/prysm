//go:build cgo && ((linux && (amd64 || arm64)) || (darwin && arm64))

package proofengine

import (
	"errors"
	"fmt"

	ere "github.com/nalepae/go-ere"
)

// ereVerifier verifies proofs with the ere verifier of a zkVM.
type ereVerifier struct {
	verifier *ere.Verifier
}

func newZkVMVerifier(kind zkVMKind, verificationKey []byte) (zkVMVerifier, error) {
	var ereKind ere.ZkVMKind
	switch kind {
	case zkVMOpenVM:
		ereKind = ere.OpenVM
	case zkVMSP1:
		ereKind = ere.SP1
	case zkVMZisk:
		ereKind = ere.Zisk
	default:
		return nil, fmt.Errorf("unknown zkVM %s", kind)
	}

	verifier, err := ere.New(ereKind, verificationKey)
	if err != nil {
		return nil, fmt.Errorf("new ere verifier: %w", err)
	}

	return &ereVerifier{verifier: verifier}, nil
}

func (v *ereVerifier) verify(proof []byte) ([]byte, error) {
	publicValues, err := v.verifier.Verify(proof)
	if errors.Is(err, ere.ErrDecodeProof) || errors.Is(err, ere.ErrVerify) {
		return nil, fmt.Errorf("%w: %w", ErrProofInvalid, err)
	}
	if err != nil {
		return nil, fmt.Errorf("ere verify: %w", err)
	}

	return publicValues, nil
}
