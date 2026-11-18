package signingprotection

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// SlotMessage is a message type signed at a leaf derived from its slot.
// The values follow the `LEAF_OFFSET_*` constants.
type SlotMessage uint64

const (
	Block                    SlotMessage = 0 // LEAF_OFFSET_BEACON_PROPOSER
	AggregateAndProof        SlotMessage = 1 // LEAF_OFFSET_AGGREGATE_AND_PROOF
	PayloadAttestation       SlotMessage = 2 // LEAF_OFFSET_PTC_ATTESTER
	ProposerPreferences      SlotMessage = 3 // LEAF_OFFSET_PROPOSER_PREFERENCES
	ExecutionPayloadEnvelope SlotMessage = 4 // LEAF_OFFSET_EXECUTION_PAYLOAD_ENVELOPE
)

// String returns the name of the message type.
func (m SlotMessage) String() string {
	switch m {
	case Block:
		return "block"
	case AggregateAndProof:
		return "aggregate and proof"
	case PayloadAttestation:
		return "payload attestation"
	case ProposerPreferences:
		return "proposer preferences"
	case ExecutionPayloadEnvelope:
		return "execution payload envelope"
	default:
		return fmt.Sprintf("unknown slot message %d", uint64(m))
	}
}

// CheckAndSaveAttestation records an attestation with `source` and `target` epochs for `pubkey`.
// It refuses, without recording anything, an attestation whose source is lower than the recorded source,
// or whose target is lower than or equal to the recorded target.
func (s *Store) CheckAndSaveAttestation(pubkey [PubkeyLength]byte, source, target primitives.Epoch) error {
	if source > target {
		return fmt.Errorf("%w: attestation source epoch %d is higher than target epoch %d", ErrSigningRefused, source, target)
	}

	err := s.update(pubkey, func(protection *ValidatorSigningProtection) error {
		incomingSource, incomingTarget := uint64(source), uint64(target)

		if recorded := protection.LastSignedAttestationSourceEpoch; recorded != nil && incomingSource < *recorded {
			return fmt.Errorf("%w: attestation source epoch %d is lower than recorded source epoch %d", ErrSigningRefused, incomingSource, *recorded)
		}

		if recorded := protection.LastSignedAttestationTargetEpoch; recorded != nil && incomingTarget <= *recorded {
			return fmt.Errorf("%w: attestation target epoch %d is lower than or equal to recorded target epoch %d", ErrSigningRefused, incomingTarget, *recorded)
		}

		protection.LastSignedAttestationSourceEpoch = &incomingSource
		protection.LastSignedAttestationTargetEpoch = &incomingTarget

		return nil
	})
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	return nil
}

// CheckAndSaveSlotMessage records a message of type `message` at `slot` for `pubkey`.
// It refuses, without recording anything, a slot lower than or equal to the recorded slot for this message type.
func (s *Store) CheckAndSaveSlotMessage(pubkey [PubkeyLength]byte, message SlotMessage, slot primitives.Slot) error {
	if _, err := slotWatermark(&ValidatorSigningProtection{}, message); err != nil {
		return fmt.Errorf("slot watermark: %w", err)
	}

	err := s.update(pubkey, func(protection *ValidatorSigningProtection) error {
		watermark, err := slotWatermark(protection, message)
		if err != nil {
			return fmt.Errorf("slot watermark: %w", err)
		}

		incomingSlot := uint64(slot)
		if recorded := *watermark; recorded != nil && incomingSlot <= *recorded {
			return fmt.Errorf("%w: %s slot %d is lower than or equal to recorded slot %d", ErrSigningRefused, message, incomingSlot, *recorded)
		}

		*watermark = &incomingSlot
		return nil
	})
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	return nil
}

// NextReservedLeaf allocates and records the next unused reserved leaf of `pubkey`, and returns it.
// Leaf 0 is never returned, since it is the leaf of the key's preregistration.
func (s *Store) NextReservedLeaf(pubkey [PubkeyLength]byte) (uint64, error) {
	leaf := uint64(0)

	err := s.update(pubkey, func(protection *ValidatorSigningProtection) error {
		next := protection.LastSignedReservedLeaf + 1
		if next >= ReservedLeafCount {
			return fmt.Errorf("%w: all %d reserved leaves are used", ErrSigningRefused, ReservedLeafCount)
		}

		protection.LastSignedReservedLeaf = next
		leaf = next
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("update: %w", err)
	}

	return leaf, nil
}

// Merge raises the watermarks of `pubkey` to those of `other`, field by field.
// It never lowers a watermark, so it can only make the protection stricter.
// TODO: Remove this last comment when we use this function.
// It is used to carry the attestation watermarks of a validator from its BLS key to its XMSS key at the fork.
func (s *Store) Merge(pubkey [PubkeyLength]byte, other *ValidatorSigningProtection) error {
	if other == nil {
		return nil
	}

	err := s.update(pubkey, func(protection *ValidatorSigningProtection) error {
		protection.LastSignedReservedLeaf = max(protection.LastSignedReservedLeaf, other.LastSignedReservedLeaf)
		protection.LastSignedAttestationSourceEpoch = maxPtr(protection.LastSignedAttestationSourceEpoch, other.LastSignedAttestationSourceEpoch)
		protection.LastSignedAttestationTargetEpoch = maxPtr(protection.LastSignedAttestationTargetEpoch, other.LastSignedAttestationTargetEpoch)
		protection.LastSignedBlockSlot = maxPtr(protection.LastSignedBlockSlot, other.LastSignedBlockSlot)
		protection.LastSignedAggregateAndProofSlot = maxPtr(protection.LastSignedAggregateAndProofSlot, other.LastSignedAggregateAndProofSlot)
		protection.LastSignedPayloadAttestationSlot = maxPtr(protection.LastSignedPayloadAttestationSlot, other.LastSignedPayloadAttestationSlot)
		protection.LastSignedProposerPreferencesSlot = maxPtr(protection.LastSignedProposerPreferencesSlot, other.LastSignedProposerPreferencesSlot)
		protection.LastSignedExecutionPayloadEnvelopeSlot = maxPtr(protection.LastSignedExecutionPayloadEnvelopeSlot, other.LastSignedExecutionPayloadEnvelopeSlot)
		return nil
	})
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	return nil
}

// slotWatermark returns the address of the watermark of `message` in `protection`.
func slotWatermark(protection *ValidatorSigningProtection, message SlotMessage) (**uint64, error) {
	switch message {
	case Block:
		return &protection.LastSignedBlockSlot, nil
	case AggregateAndProof:
		return &protection.LastSignedAggregateAndProofSlot, nil
	case PayloadAttestation:
		return &protection.LastSignedPayloadAttestationSlot, nil
	case ProposerPreferences:
		return &protection.LastSignedProposerPreferencesSlot, nil
	case ExecutionPayloadEnvelope:
		return &protection.LastSignedExecutionPayloadEnvelopeSlot, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrSigningRefused, message)
	}
}

// maxPtr returns the highest of `a` and `b`, a nil value being lower than any value.
func maxPtr(a, b *uint64) *uint64 {
	if a == nil {
		return copyPtr(b)
	}

	if b == nil || *a >= *b {
		return copyPtr(a)
	}

	return copyPtr(b)
}

// copyPtr returns a copy of `p`.
func copyPtr(p *uint64) *uint64 {
	if p == nil {
		return nil
	}

	v := *p
	return &v
}
