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
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

var (
	errProofDataEmpty        = errors.New("execution proof carries no proof data")
	errProofDataTooLarge     = errors.New("execution proof exceeds MAX_PROOF_SIZE")
	errProofTypeUnsupported  = errors.New("execution proof names an unsupported proof type")
	errProverUnknown         = errors.New("execution proof prover is not a known validator")
	errProverInactive        = errors.New("execution proof prover is not an active validator")
	errProofSignatureInvalid = errors.New("execution proof signature is invalid")
)

// VerifyExecutionProofEnvelope verifies an execution proof envelope against the beacon state and payload.
// The execution proof itself is verified separately by the proof engine.
// https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8025/beacon-chain.md#new-verify_execution_proof_envelope
func VerifyExecutionProofEnvelope(
	st state.ReadOnlyBeaconState,
	signedProof blocks.ROSignedExecutionProofEnvelope,
) error {
	proof := signedProof.Message

	proverIndex := signedProof.ValidatorIndex
	if uint64(proverIndex) >= uint64(st.NumValidators()) {
		return fmt.Errorf("%w: index %d", errProverUnknown, proverIndex)
	}

	if len(proof.ProofData) == 0 {
		return errProofDataEmpty
	}
	if uint64(len(proof.ProofData)) > params.BeaconConfig().MaxProofSize {
		return fmt.Errorf("%w: %d bytes", errProofDataTooLarge, len(proof.ProofData))
	}
	if !signedProof.ProofType().Supported() {
		return fmt.Errorf("%w: %s", errProofTypeUnsupported, signedProof.ProofType())
	}

	validator, err := st.ValidatorAtIndexReadOnly(proverIndex)
	if err != nil {
		return fmt.Errorf("could not read prover %d: %w", proverIndex, err)
	}
	if !helpers.IsActiveValidatorUsingTrie(validator, slots.ToEpoch(st.Slot())) {
		return fmt.Errorf("%w: index %d", errProverInactive, proverIndex)
	}

	domain, err := signing.Domain(
		st.Fork(),
		slots.ToEpoch(st.Slot()),
		params.BeaconConfig().DomainExecutionProof,
		st.GenesisValidatorsRoot(),
	)
	if err != nil {
		return fmt.Errorf("could not compute execution proof signing domain: %w", err)
	}

	signingRoot, err := signing.ComputeSigningRoot(proof, domain)
	if err != nil {
		return fmt.Errorf("could not compute execution proof signing root: %w", err)
	}

	publicKey := validator.PublicKey()
	pubkey, err := bls.PublicKeyFromBytes(publicKey[:])
	if err != nil {
		return fmt.Errorf("could not decode prover public key: %w", err)
	}
	signature, err := bls.SignatureFromBytes(signedProof.Signature)
	if err != nil {
		return fmt.Errorf("%w: %w", errProofSignatureInvalid, err)
	}
	if !signature.Verify(pubkey, signingRoot[:]) {
		return errProofSignatureInvalid
	}
	return nil
}
