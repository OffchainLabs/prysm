package verification

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// ExecutionPayloadEnvelopeVerifier defines the methods implemented by the ROSignedExecutionPayloadEnvelope.
type ExecutionPayloadEnvelopeVerifier interface {
	VerifyBlockRootSeen(func([32]byte) bool) error
	VerifyBlockRootValid(func([32]byte) bool) error
	VerifySlotAboveFinalized(primitives.Epoch) error
	VerifySlotMatchesBlock(primitives.Slot) error
	VerifyBuilderValid(interfaces.ROExecutionPayloadBid) error
	VerifyPayloadHash(interfaces.ROExecutionPayloadBid) error
	VerifyExecutionRequestsRoot(interfaces.ROExecutionPayloadBid) error
	VerifySignature(context.Context, state.ReadOnlyBeaconState) error
	SatisfyRequirement(Requirement)
}

// NewExecutionPayloadEnvelopeVerifier is a function signature that can be used by code that needs to be
// able to mock Initializer.NewExecutionPayloadEnvelopeVerifier without complex setup.
type NewExecutionPayloadEnvelopeVerifier func(e interfaces.ROSignedExecutionPayloadEnvelope, reqs []Requirement) ExecutionPayloadEnvelopeVerifier

// ExecutionPayloadEnvelopeGossipRequirements defines the list of requirements for gossip
// execution payload envelopes.
var ExecutionPayloadEnvelopeGossipRequirements = []Requirement{
	RequireBlockRootSeen,
	RequireBlockRootValid,
	RequireEnvelopeSlotAboveFinalized,
	RequireEnvelopeSlotMatchesBlock,
	RequireBuilderValid,
	RequirePayloadHashValid,
	RequireExecutionRequestsRootValid,
	RequireBuilderSignatureValid,
}

// GossipExecutionPayloadEnvelopeRequirements is a requirement list for gossip execution payload envelopes.
var GossipExecutionPayloadEnvelopeRequirements = requirementList(ExecutionPayloadEnvelopeGossipRequirements)

var (
	ErrEnvelopeBlockRootNotSeen       = errors.New("block root not seen")
	ErrEnvelopeBlockRootInvalid       = errors.New("block root invalid")
	ErrEnvelopeSlotBeforeFinalized    = errors.New("envelope slot is before finalized checkpoint")
	ErrEnvelopeSlotMismatch           = errors.New("envelope slot does not match block slot")
	ErrIncorrectEnvelopeBuilder       = errors.New("builder index does not match committed header")
	ErrIncorrectEnvelopeBlockHash     = errors.New("block hash does not match committed header")
	ErrIncorrectExecutionRequestsRoot = errors.New("execution requests root does not match committed bid")
)

var _ ExecutionPayloadEnvelopeVerifier = &EnvelopeVerifier{}

// EnvelopeVerifier is a read-only verifier for execution payload envelopes.
type EnvelopeVerifier struct {
	results *results
	e       interfaces.ROSignedExecutionPayloadEnvelope
}

// VerifyBlockRootSeen verifies if the block root has been seen before.
func (v *EnvelopeVerifier) VerifyBlockRootSeen(blockRootSeen func([32]byte) bool) (err error) {
	defer v.record(RequireBlockRootSeen, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return errors.Wrap(err, "failed to get envelope")
	}
	if blockRootSeen != nil && blockRootSeen(env.BeaconBlockRoot()) {
		return nil
	}
	return fmt.Errorf("%w: root=%#x slot=%d builder=%d", ErrEnvelopeBlockRootNotSeen, env.BeaconBlockRoot(), env.Slot(), env.BuilderIndex())
}

// VerifyBlockRootValid verifies if the block root is valid.
func (v *EnvelopeVerifier) VerifyBlockRootValid(badBlock func([32]byte) bool) (err error) {
	defer v.record(RequireBlockRootValid, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return errors.Wrap(err, "failed to get envelope")
	}
	if badBlock != nil && badBlock(env.BeaconBlockRoot()) {
		return fmt.Errorf("%w: root=%#x slot=%d builder=%d", ErrEnvelopeBlockRootInvalid, env.BeaconBlockRoot(), env.Slot(), env.BuilderIndex())
	}
	return nil
}

// VerifySlotAboveFinalized ensures the envelope slot is not before the latest finalized epoch start.
func (v *EnvelopeVerifier) VerifySlotAboveFinalized(finalizedEpoch primitives.Epoch) (err error) {
	defer v.record(RequireEnvelopeSlotAboveFinalized, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return errors.Wrap(err, "failed to get envelope")
	}
	startSlot, err := slots.EpochStart(finalizedEpoch)
	if err != nil {
		return errors.Wrapf(ErrEnvelopeSlotBeforeFinalized, "error computing epoch start slot for finalized checkpoint (%d) %s", finalizedEpoch, err.Error())
	}
	if env.Slot() < startSlot {
		return fmt.Errorf("%w: slot=%d start=%d", ErrEnvelopeSlotBeforeFinalized, env.Slot(), startSlot)
	}
	return nil
}

// VerifySlotMatchesBlock ensures the envelope slot matches the block slot.
func (v *EnvelopeVerifier) VerifySlotMatchesBlock(blockSlot primitives.Slot) (err error) {
	defer v.record(RequireEnvelopeSlotMatchesBlock, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return errors.Wrap(err, "failed to get envelope")
	}
	if env.Slot() != blockSlot {
		return fmt.Errorf("%w: envelope=%d block=%d", ErrEnvelopeSlotMismatch, env.Slot(), blockSlot)
	}
	return nil
}

// VerifyBuilderValid checks that the builder index matches the one in the bid.
func (v *EnvelopeVerifier) VerifyBuilderValid(bid interfaces.ROExecutionPayloadBid) (err error) {
	defer v.record(RequireBuilderValid, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return errors.Wrap(err, "failed to get envelope")
	}
	if bid.BuilderIndex() != env.BuilderIndex() {
		return fmt.Errorf("%w: envelope=%d bid=%d", ErrIncorrectEnvelopeBuilder, env.BuilderIndex(), bid.BuilderIndex())
	}
	return nil
}

// VerifyPayloadHash checks that the payload blockhash matches the one in the bid.
func (v *EnvelopeVerifier) VerifyPayloadHash(bid interfaces.ROExecutionPayloadBid) (err error) {
	defer v.record(RequirePayloadHashValid, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return errors.Wrap(err, "failed to get envelope")
	}
	if env.IsBlinded() {
		return nil
	}
	if bid.BlockHash() != env.BlockHash() {
		return fmt.Errorf("%w: payload=%#x bid=%#x", ErrIncorrectEnvelopeBlockHash, env.BlockHash(), bid.BlockHash())
	}
	return nil
}

// VerifyExecutionRequestsRoot checks that hash_tree_root(envelope.execution_requests) == bid.execution_requests_root.
func (v *EnvelopeVerifier) VerifyExecutionRequestsRoot(bid interfaces.ROExecutionPayloadBid) (err error) {
	defer v.record(RequireExecutionRequestsRootValid, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return errors.Wrap(err, "failed to get envelope")
	}
	requestsRoot, err := env.ExecutionRequests().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute execution requests root")
	}
	bidRoot := bid.ExecutionRequestsRoot()
	if requestsRoot != bidRoot {
		return fmt.Errorf("%w: envelope=%#x bid=%#x", ErrIncorrectExecutionRequestsRoot, requestsRoot, bidRoot)
	}
	return nil
}

// VerifySignature verifies the signature of the execution payload envelope.
func (v *EnvelopeVerifier) VerifySignature(ctx context.Context, st state.ReadOnlyBeaconState) (err error) {
	defer v.record(RequireBuilderSignatureValid, &err)

	err = validatePayloadEnvelopeSignature(ctx, st, v.e)
	if err != nil {
		env, envErr := v.e.Envelope()
		if envErr != nil {
			return errors.Wrap(err, "failed to get envelope for signature validation")
		}
		return errors.Wrapf(err, "signature validation failed: root=%#x slot=%d builder=%d", env.BeaconBlockRoot(), env.Slot(), env.BuilderIndex())
	}
	return nil
}

// SatisfyRequirement allows the caller to manually mark a requirement as satisfied.
func (v *EnvelopeVerifier) SatisfyRequirement(req Requirement) {
	v.record(req, nil)
}

// record records the result of a requirement verification.
func (v *EnvelopeVerifier) record(req Requirement, err *error) {
	if err == nil || *err == nil {
		v.results.record(req, nil)
		return
	}

	v.results.record(req, *err)
}

// validatePayloadEnvelopeSignature verifies the signature of a signed execution payload envelope
func validatePayloadEnvelopeSignature(ctx context.Context, st state.ReadOnlyBeaconState, e interfaces.ROSignedExecutionPayloadEnvelope) error {
	env, err := e.Envelope()
	if err != nil {
		return errors.Wrap(err, "failed to get envelope")
	}
	var pubkey []byte
	if env.BuilderIndex() == params.BeaconConfig().BuilderIndexSelfBuild {
		proposerIdx, err := helpers.BeaconProposerIndexAtSlot(ctx, st, env.Slot())
		if err != nil {
			return errors.Wrap(err, "failed to get proposer index at slot")
		}
		val, err := st.ValidatorAtIndex(proposerIdx)
		if err != nil {
			return errors.Wrap(err, "failed to get proposer validator")
		}
		pubkey = val.PublicKey
	} else {
		builderPubkey, err := st.BuilderPubkey(env.BuilderIndex())
		if err != nil {
			return errors.Wrap(err, "failed to get builder pubkey")
		}
		pubkey = builderPubkey[:]
	}
	pub, err := bls.PublicKeyFromBytes(pubkey)
	if err != nil {
		return errors.Wrap(err, "invalid public key")
	}
	s := e.Signature()
	sig, err := bls.SignatureFromBytes(s[:])
	if err != nil {
		return errors.Wrap(err, "invalid signature format")
	}
	currentEpoch := slots.ToEpoch(st.Slot())
	domain, err := signing.Domain(st.Fork(), currentEpoch, params.BeaconConfig().DomainBeaconBuilder, st.GenesisValidatorsRoot())
	if err != nil {
		return errors.Wrap(err, "failed to compute signing domain")
	}
	root, err := e.SigningRoot(domain)
	if err != nil {
		return errors.Wrap(err, "failed to compute signing root")
	}
	if !sig.Verify(pub, root[:]) {
		return signing.ErrSigFailedToVerify
	}
	return nil
}
