package verification

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	log "github.com/sirupsen/logrus"
)

const (
	RequireBuilderValid Requirement = iota
	RequirePayloadHashValid
)

// ExecutionPayloadEnvelopeGossipRequirements defines the list of requirements for gossip
// execution payload envelopes.
var ExecutionPayloadEnvelopeGossipRequirements = []Requirement{
	RequireBlockRootSeen,
	RequireBlockRootValid,
	RequireBuilderValid,
	RequirePayloadHashValid,
	RequireSignatureValid,
}

// GossipExecutionPayloadEnvelopeRequirements is a requirement list for gossip execution payload envelopes.
var GossipExecutionPayloadEnvelopeRequirements = RequirementList(ExecutionPayloadEnvelopeGossipRequirements)

var (
	ErrEnvelopeBlockRootNotSeen   = errors.New("block root not seen")
	ErrEnvelopeBlockRootInvalid   = errors.New("block root invalid")
	ErrIncorrectEnvelopeBuilder   = errors.New("builder index does not match committed header")
	ErrIncorrectEnvelopeBlockHash = errors.New("block hash does not match committed header")
	ErrInvalidEnvelope            = errors.New("invalid payload attestation message")
)

var _ ExecutionPayloadEnvelopeVerifier = &EnvelopeVerifier{}

// EnvelopeVerifier is a read-only verifier for execution payload envelopes.
type EnvelopeVerifier struct {
	results *results
	e       interfaces.ROSignedExecutionPayloadEnvelope
}

// VerifyBlockRootSeen verifies if the block root has been seen before.
func (v *EnvelopeVerifier) VerifyBlockRootSeen(parentSeen func([32]byte) bool) (err error) {
	defer v.record(RequireBlockRootSeen, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return err
	}
	if parentSeen != nil && parentSeen(env.BeaconBlockRoot()) {
		return nil
	}
	log.WithFields(envelopeLogFields(env)).Error(ErrEnvelopeBlockRootNotSeen.Error())
	return ErrEnvelopeBlockRootNotSeen
}

// VerifyBlockRootValid verifies if the block root is valid.
func (v *EnvelopeVerifier) VerifyBlockRootValid(badBlock func([32]byte) bool) (err error) {
	defer v.record(RequireBlockRootValid, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return err
	}
	if badBlock != nil && badBlock(env.BeaconBlockRoot()) {
		log.WithFields(envelopeLogFields(env)).Error(ErrEnvelopeBlockRootInvalid.Error())
		return ErrPayloadAttBlockRootInvalid
	}
	return nil
}

// VerifyBuilderValid checks that the builder index matches the one in the
// payload header
func (v *EnvelopeVerifier) VerifyBuilderValid(header interfaces.ROExecutionPayloadHeaderEPBS) (err error) {
	defer v.record(RequireBuilderValid, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return err
	}
	if header.BuilderIndex() != env.BuilderIndex() {
		log.WithFields(envelopeLogFields(env)).Error(ErrIncorrectEnvelopeBuilder.Error())
		return ErrIncorrectEnvelopeBuilder
	}
	return nil
}

// VerifyPayloadHash checks that the payload blockhash matches the one in the
// payload header
func (v *EnvelopeVerifier) VerifyPayloadHash(header interfaces.ROExecutionPayloadHeaderEPBS) (err error) {
	defer v.record(RequirePayloadHashValid, &err)
	env, err := v.e.Envelope()
	if err != nil {
		return err
	}
	if env.PayloadWithheld() {
		return nil
	}
	payload, err := env.Execution()
	if err != nil {
		return err
	}
	if header.BlockHash() != [32]byte(payload.BlockHash()) {
		log.WithFields(envelopeLogFields(env)).Error(ErrIncorrectEnvelopeBlockHash.Error())
		return ErrIncorrectEnvelopeBlockHash
	}
	return nil
}

// VerifySignature verifies the signature of the execution payload envelope.
func (v *EnvelopeVerifier) VerifySignature(st state.BeaconState) (err error) {
	defer v.record(RequireSignatureValid, &err)

	err = validatePayloadEnvelopeSignature(st, v.e)
	if err != nil {
		env, envErr := v.e.Envelope()
		if envErr != nil {
			return err
		}
		if errors.Is(err, signing.ErrSigFailedToVerify) {
			log.WithFields(envelopeLogFields(env)).Error("signature failed to validate")
		} else {
			log.WithFields(envelopeLogFields(env)).WithError(err).Error("could not validate signature")
		}
		return err
	}
	return nil
}

// SetSlot initializes the internal slot member of the execution payload
// envelope
func (v *EnvelopeVerifier) SetSlot(slot primitives.Slot) error {
	env, err := v.e.Envelope()
	if err != nil {
		return err
	}
	env.SetSlot(slot)
	return nil
}

// SatisfyRequirement allows the caller to manually mark a requirement as satisfied.
func (v *EnvelopeVerifier) SatisfyRequirement(req Requirement) {
	v.record(req, nil)
}

// envelopeLogFields returns log fields for a ROExecutionPayloadEnvelope instance.
func envelopeLogFields(e interfaces.ROExecutionPayloadEnvelope) log.Fields {
	return log.Fields{
		"builderIndex":    e.BuilderIndex(),
		"beaconBlockRoot": fmt.Sprintf("%#x", e.BeaconBlockRoot()),
		"PayloadWithheld": e.PayloadWithheld(),
		"stateRoot":       fmt.Sprintf("%#x", e.StateRoot()),
	}
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
func validatePayloadEnvelopeSignature(st state.BeaconState, e interfaces.ROSignedExecutionPayloadEnvelope) error {
	env, err := e.Envelope()
	if err != nil {
		return err
	}
	val, err := st.ValidatorAtIndex(env.BuilderIndex())
	if err != nil {
		return err
	}
	pub, err := bls.PublicKeyFromBytes(val.PublicKey)
	if err != nil {
		return err
	}
	s := e.Signature()
	sig, err := bls.SignatureFromBytes(s[:])
	if err != nil {
		return err
	}
	currentEpoch := slots.ToEpoch(st.Slot())
	domain, err := signing.Domain(st.Fork(), currentEpoch, params.BeaconConfig().DomainBeaconBuilder, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	root, err := e.SigningRoot(domain)
	if err != nil {
		return err
	}
	if !sig.Verify(pub, root[:]) {
		return signing.ErrSigFailedToVerify
	}
	return nil
}
