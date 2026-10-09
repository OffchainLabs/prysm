// Package proofengine verifies EIP-8025 execution proofs in-process, with the zkVM verifiers of
// github.com/eth-act/ere.
// https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8025/proof-engine.md
package proofengine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// publicInputLength is the length of the SSZ encoding of a PublicInput.
const publicInputLength = fieldparams.RootLength + 1 + 8 + 2

var (
	// ErrProofInvalid is returned when an execution proof fails verification.
	ErrProofInvalid = errors.New("execution proof is invalid")
	// ErrUnsupportedProofType is returned when no verifier is configured for the proof type.
	ErrUnsupportedProofType = errors.New("no verifier configured for the execution proof type")
	// errNotSupported is returned when this binary was built without the ere verifiers.
	errNotSupported = errors.New("EIP-8025 proof verification needs cgo on linux/amd64, linux/arm64 or darwin/arm64")
)

// zkVMKind identifies the zkVM a proof type is proven with.
type zkVMKind uint8

const (
	zkVMOpenVM zkVMKind = iota
	zkVMSP1
	zkVMZisk
)

func (k zkVMKind) String() string {
	switch k {
	case zkVMOpenVM:
		return "openvm"
	case zkVMSP1:
		return "sp1"
	case zkVMZisk:
		return "zisk"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(k))
	}
}

// zkVMKindOf returns the zkVM a proof type is proven with.
func zkVMKindOf(proofType ethpb.ProofType) (zkVMKind, error) {
	switch proofType {
	case ethpb.ProofTypeEthrexOpenVM:
		return zkVMOpenVM, nil
	case ethpb.ProofTypeEthrexSP1:
		return zkVMSP1, nil
	case ethpb.ProofTypeEthrexZisk:
		return zkVMZisk, nil
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedProofType, proofType)
	}
}

// zkVMVerifier verifies the proofs of one guest program, identified by its verification key.
type zkVMVerifier interface {
	// verify returns the public values the proof commits to. It returns an error wrapping ErrProofInvalid if the
	// proof is malformed or fails verification.
	verify(proof []byte) ([]byte, error)
}

// typeVerifier verifies the proofs of one proof type.
type typeVerifier struct {
	zkVM    zkVMVerifier
	kind    zkVMKind
	standIn []byte // standIn is a genuine proof of this proof type, verified in place of each mock proof. It is only set in mock mode.
}

// Engine verifies execution proofs. It is safe for concurrent use.
type Engine struct {
	verifiers map[ethpb.ProofType]typeVerifier
	// slots bounds the number of concurrent verifications, each of them holding an OS thread in cgo.
	slots chan struct{}
}

// New returns an engine verifying the proof types of the configuration file at configPath, or every proof type
// with the built-in verification keys if configPath is empty. With mockProofs, it also accepts the mock proofs of
// zkboost's mock zkVMs, for devnets only (see mock.go).
func New(configPath string, mockProofs bool) (*Engine, error) {
	keys := defaultVerificationKeys()
	if configPath != "" {
		var err error
		if keys, err = loadVerificationKeys(configPath); err != nil {
			return nil, fmt.Errorf("load verification keys: %w", err)
		}
	}

	engine, err := newEngine(keys, newZkVMVerifier)
	if err != nil {
		return nil, err
	}

	if mockProofs {
		standIns, err := standInProofs()
		if err != nil {
			return nil, err
		}

		if err := engine.enableMockProofs(standIns); err != nil {
			return nil, fmt.Errorf("enable mock proofs: %w", err)
		}
	}

	return engine, nil
}

func newEngine(
	keys map[ethpb.ProofType][]byte,
	newVerifier func(zkVMKind, []byte) (zkVMVerifier, error),
) (*Engine, error) {
	engine := &Engine{
		verifiers: make(map[ethpb.ProofType]typeVerifier, len(keys)),
		slots:     make(chan struct{}, max(1, runtime.NumCPU()/2)),
	}

	for proofType, key := range keys {
		kind, err := zkVMKindOf(proofType)
		if err != nil {
			return nil, err
		}

		verifier, err := newVerifier(kind, key)
		if err != nil {
			return nil, fmt.Errorf("new %s verifier for %s: %w", kind, proofType, err)
		}

		engine.verifiers[proofType] = typeVerifier{zkVM: verifier, kind: kind}
	}

	return engine, nil
}

// Verify verifies an execution proof against the expected public input, the SSZ encoding of a PublicInput.
// https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8025/proof-engine.md#new-verify_execution_proof
func (e *Engine) Verify(ctx context.Context, proofType ethpb.ProofType, proofData, publicInput []byte) error {
	verifier, ok := e.verifiers[proofType]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnsupportedProofType, proofType)
	}

	select {
	case e.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-e.slots }()

	start := time.Now()
	publicValues, err := verifier.publicValues(proofData)
	result := verificationValid
	if err != nil {
		result = verificationInvalid
	}
	verificationDuration.WithLabelValues(proofType.String(), string(result)).Observe(time.Since(start).Seconds())

	if err != nil {
		return fmt.Errorf("verify %s proof: %w", proofType, err)
	}

	// A valid proof of another payload, or of a failed validation, must not validate this payload.
	if !publicValuesMatch(verifier.kind, publicValues, publicInput) {
		return fmt.Errorf("%w: the %s proof commits to other public values", ErrProofInvalid, proofType)
	}

	return nil
}

// publicValues verifies a proof and returns the public values it commits to. It returns an error wrapping
// ErrProofInvalid if the proof is malformed or fails verification.
func (v typeVerifier) publicValues(proofData []byte) ([]byte, error) {
	if v.standIn != nil {
		if mockPublicValues, ok := decodeMockProof(proofData); ok {
			// Mock mode, devnets only: a mock proof cannot be verified. Spend the cost of a real verification on
			// the stand-in proof and discard its result, which says nothing about this payload. The public values
			// come from the mock proof, and are checked by the caller. See mock.go.
			_, _ = v.zkVM.verify(v.standIn)
			return mockPublicValues, nil
		}
	}

	return v.zkVM.verify(proofData)
}

// PublicInput returns the SSZ encoding of the PublicInput of the payload whose new payload request has this root.
// The payload is claimed valid, on the chain and for the stateless input schema of this node.
// https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8025/beacon-chain.md#new-get_execution_proof
func PublicInput(newPayloadRequestRoot [fieldparams.RootLength]byte) []byte {
	cfg := params.BeaconConfig()

	publicInput := make([]byte, 0, publicInputLength)
	publicInput = append(publicInput, newPayloadRequestRoot[:]...)
	publicInput = append(publicInput, 1) // successful_validation
	publicInput = binary.LittleEndian.AppendUint64(publicInput, cfg.DepositChainID)
	publicInput = binary.LittleEndian.AppendUint16(publicInput, uint16(cfg.StatelessInputSchemaId))

	return publicInput
}

// publicValuesMatch reports whether the public values a proof commits to are the expected public input.
// OpenVM and ZisK commit to fixed-size public values, so their encoding is padded with zeros.
func publicValuesMatch(kind zkVMKind, publicValues, publicInput []byte) bool {
	if kind == zkVMSP1 {
		return bytes.Equal(publicValues, publicInput)
	}

	if len(publicValues) < len(publicInput) || !bytes.Equal(publicValues[:len(publicInput)], publicInput) {
		return false
	}

	for _, b := range publicValues[len(publicInput):] {
		if b != 0 {
			return false
		}
	}

	return true
}
