// Package eip8025 implements the consensus rules for optional execution
// proofs.
package eip8025

import (
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

var (
	errProverUnknown         = errors.New("execution proof prover is not a known validator")
	errProverInactive        = errors.New("execution proof prover is not an active validator")
	errProofSignatureInvalid = errors.New("execution proof signature is invalid")
)

// VerifyExecutionProofEnvelope verifies an execution proof envelope against the beacon state and payload.
// The execution proof itself is verified separately by the proof engine. The stateless checks (non-empty proof data,
// supported proof type) are done when the envelope is wrapped, by blocks.NewROSignedExecutionProofEnvelope.
// https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8025/beacon-chain.md#new-verify_execution_proof_envelope
func VerifyExecutionProofEnvelope(
	st state.ReadOnlyBeaconState,
	epoch primitives.Epoch,
	signedProof blocks.ROSignedExecutionProofEnvelope,
) error {
	proof := signedProof.Message

	proverIndex := signedProof.ValidatorIndex
	if uint64(proverIndex) >= uint64(st.NumValidators()) {
		return fmt.Errorf("%w: index %d", errProverUnknown, proverIndex)
	}

	validator, err := st.ValidatorAtIndexReadOnly(proverIndex)
	if err != nil {
		return fmt.Errorf("could not read prover %d: %w", proverIndex, err)
	}
	if !helpers.IsActiveValidatorUsingTrie(validator, epoch) {
		return fmt.Errorf("%w: index %d", errProverInactive, proverIndex)
	}

	domain, err := signing.Domain(
		st.Fork(),
		epoch,
		params.BeaconConfig().DomainExecutionProof,
		st.GenesisValidatorsRoot(),
	)
	if err != nil {
		return fmt.Errorf("could not compute execution proof signing domain: %w", err)
	}

	publicKey := validator.PublicKey()
	if err := signing.VerifySigningRoot(proof, publicKey[:], signedProof.Signature, domain); err != nil {
		return fmt.Errorf("%w: %w", errProofSignatureInvalid, err)
	}

	return nil
}
