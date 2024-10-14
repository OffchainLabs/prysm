package verification

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/crypto/bls"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	log "github.com/sirupsen/logrus"
)

const (
	RequireBuilderActiveNotSlashed Requirement = iota
	RequireBuilderSufficientBalance
	RequireKnownParentBlockHash
	RequireKnownParentBlockRoot
	RequireCurrentOrNextSlot
)

// ExecutionPayloadHeaderGossipRequirements defines the list of requirements for gossip
// signed execution payload header.
var ExecutionPayloadHeaderGossipRequirements = []Requirement{
	RequireBuilderActiveNotSlashed,
	RequireBuilderSufficientBalance,
	RequireKnownParentBlockHash,
	RequireBlockRootSeen,
	RequireCurrentOrNextSlot,
	RequireSignatureValid,
}

// GossipExecutionPayloadHeaderRequirements is a requirement list for gossip execution payload header  messages.
var GossipExecutionPayloadHeaderRequirements = RequirementList(PayloadAttGossipRequirements)

var (
	ErrBuilderSlashed             = errors.New("builder is slashed")
	ErrBuilderInactive            = errors.New("builder is inactive")
	ErrBuilderInsufficientBalance = errors.New("insufficient builder balance")
	ErrUnknownParentBlockHash     = errors.New("unknown parent block hash")
	ErrUnknownParentBlockRoot     = errors.New("unknown parent block root")
	ErrIncorrectPayloadHeaderSlot = errors.New("incorrect payload header slot")
)

// HeaderVerifier is a verifier for execution payload headers.
type HeaderVerifier struct {
	*sharedResources
	results *results
	h       interfaces.ROSignedExecutionPayloadHeader
	st      state.ReadOnlyBeaconState
}

var _ ExecutionPayloadHeaderVerifier = &HeaderVerifier{}

// VerifyBuilderActiveNotSlashed verifies that the builder is active and not slashed.
func (v *HeaderVerifier) VerifyBuilderActiveNotSlashed() (err error) {
	defer v.record(RequireBuilderActiveNotSlashed, &err)

	h, err := v.h.Header()
	if err != nil {
		return err
	}
	val, err := v.st.ValidatorAtIndexReadOnly(h.BuilderIndex())
	if err != nil {
		return err
	}

	if val.Slashed() {
		log.WithFields(headerLogFields(h)).Error(ErrBuilderSlashed.Error())
		return ErrBuilderSlashed
	}

	t := slots.ToEpoch(v.clock.CurrentSlot())
	if !helpers.IsActiveValidatorUsingTrie(val, t) {
		log.WithFields(headerLogFields(h)).Error(ErrBuilderInactive.Error())
		return ErrBuilderInactive
	}

	return nil
}

// VerifyBuilderSufficientBalance verifies that the builder has a sufficient balance with respect to MinBuilderBalance.
func (v *HeaderVerifier) VerifyBuilderSufficientBalance() (err error) {
	defer v.record(RequireBuilderSufficientBalance, &err)

	h, err := v.h.Header()
	if err != nil {
		return err
	}
	bal, err := v.st.BalanceAtIndex(h.BuilderIndex())
	if err != nil {
		return err
	}

	minBuilderBalance := params.BeaconConfig().MinBuilderBalance
	if uint64(h.Value())+minBuilderBalance > bal {
		log.WithFields(headerLogFields(h)).Errorf("insufficient builder balance %d - minimal builder balance %d", bal, minBuilderBalance)
		return ErrBuilderInsufficientBalance
	}
	return nil
}

// VerifyParentBlockHashSeen verifies that the parent block hash is known.
func (v *HeaderVerifier) VerifyParentBlockHashSeen(seen func([32]byte) bool) (err error) {
	defer v.record(RequireKnownParentBlockHash, &err)

	h, err := v.h.Header()
	if err != nil {
		return err
	}

	if seen != nil && seen(h.ParentBlockHash()) {
		return nil
	}

	log.WithFields(headerLogFields(h)).Error(ErrUnknownParentBlockHash.Error())
	return ErrUnknownParentBlockHash
}

// VerifyParentBlockRootSeen verifies that the parent block root is known.
func (v *HeaderVerifier) VerifyParentBlockRootSeen(seen func([32]byte) bool) (err error) {
	defer v.record(RequireKnownParentBlockRoot, &err)

	h, err := v.h.Header()
	if err != nil {
		return err
	}

	if seen != nil && seen(h.ParentBlockRoot()) {
		return nil
	}

	log.WithFields(headerLogFields(h)).Error(ErrUnknownParentBlockRoot.Error())
	return ErrUnknownParentBlockRoot
}

// VerifySignature verifies the signature of the execution payload header taking in validator and the genesis root.
// It uses header's slot for fork version.
func (v *HeaderVerifier) VerifySignature() (err error) {
	defer v.record(RequireSignatureValid, &err)

	err = validatePayloadHeaderSignature(v.st, v.h)
	if err != nil {
		h, envErr := v.h.Header()
		if envErr != nil {
			return err
		}
		if errors.Is(err, signing.ErrSigFailedToVerify) {
			log.WithFields(headerLogFields(h)).Error("signature failed to validate")
		} else {
			log.WithFields(headerLogFields(h)).WithError(err).Error("could not validate signature")
		}
		return err
	}
	return nil
}

// VerifyCurrentOrNextSlot verifies that the header slot is either the current slot or the next slot.
func (v *HeaderVerifier) VerifyCurrentOrNextSlot() (err error) {
	defer v.record(RequireCurrentOrNextSlot, &err)

	h, err := v.h.Header()
	if err != nil {
		return err
	}
	if h.Slot() == v.clock.CurrentSlot()+1 || h.Slot() == v.clock.CurrentSlot() {
		return nil
	}

	log.WithFields(headerLogFields(h)).Errorf("does not match current or next slot %d", v.clock.CurrentSlot())
	return ErrIncorrectPayloadHeaderSlot
}

// SatisfyRequirement satisfies a requirement.
func (v *HeaderVerifier) SatisfyRequirement(req Requirement) {
	v.record(req, nil)
}

// record records the result of a requirement verification.
func (v *HeaderVerifier) record(req Requirement, err *error) {
	if err == nil || *err == nil {
		v.results.record(req, nil)
		return
	}

	v.results.record(req, *err)
}

// headerLogFields returns log fields for a ROExecutionPayloadHeader instance.
func headerLogFields(h interfaces.ROExecutionPayloadHeaderEPBS) log.Fields {
	return log.Fields{
		"builderIndex":    h.BuilderIndex(),
		"blockHash":       fmt.Sprintf("%#x", h.BlockHash()),
		"parentBlockHash": fmt.Sprintf("%#x", h.ParentBlockHash()),
		"parentBlockRoot": fmt.Sprintf("%#x", h.ParentBlockRoot()),
		"slot":            h.Slot(),
		"value":           h.Value(),
	}
}

// validatePayloadHeaderSignature validates the signature of the execution payload header.
func validatePayloadHeaderSignature(st state.ReadOnlyBeaconState, sh interfaces.ROSignedExecutionPayloadHeader) error {
	h, err := sh.Header()
	if err != nil {
		return err
	}

	pubkey := st.PubkeyAtIndex(h.BuilderIndex())
	pub, err := bls.PublicKeyFromBytes(pubkey[:])
	if err != nil {
		return err
	}

	s := sh.Signature()
	sig, err := bls.SignatureFromBytes(s[:])
	if err != nil {
		return err
	}

	currentEpoch := slots.ToEpoch(h.Slot())
	f, err := forks.Fork(currentEpoch)
	if err != nil {
		return err
	}

	domain, err := signing.Domain(f, currentEpoch, params.BeaconConfig().DomainBeaconBuilder, st.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	root, err := sh.SigningRoot(domain)
	if err != nil {
		return err
	}
	if !sig.Verify(pub, root[:]) {
		return signing.ErrSigFailedToVerify
	}

	return nil
}
