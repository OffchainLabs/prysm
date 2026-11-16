package xmsssigner

import (
	"fmt"
	"math"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
)

const (
	ReservedLeafCount    = signingprotection.ReservedLeafCount // Leaves for messages without a leaf of their own (`XMSS_RESERVED_LEAVES`).
	AttestationLeafCount = uint64(1 << 29)                     // Leaves for attestations, one per target epoch (`XMSS_ATTESTATION_LEAVES`).
	BuilderBidLeafCount  = uint64(1 << 30)                     // Leaves for builder bids (`XMSS_BUILDER_BID_LEAVES`).
	LeavesPerSlot        = uint64(1 << 3)                      // Leaves for slot-based messages, per slot (`XMSS_LEAVES_PER_SLOT`).
)

// AttestationLeafIndex returns the leaf at which an attestation for target `epoch` is signed
// (`compute_xmss_attestation_leaf_index`).
// It refuses an epoch beyond the attestation leaves, whose leaf would be a builder bid leaf.
func AttestationLeafIndex(epoch primitives.Epoch) (uint32, error) {
	if uint64(epoch) >= AttestationLeafCount {
		return 0, fmt.Errorf("target epoch %d is beyond the %d attestation leaves", epoch, AttestationLeafCount)
	}

	return uint32(ReservedLeafCount + uint64(epoch)), nil // #nosec G115 -- Lower than 2^15 + 2^29.
}

// SlotLeafIndex returns the leaf at which a message of type `message` is signed for `slot`
// (`compute_xmss_leaf_index`). The values of `message` are the `LEAF_OFFSET_*` constants.
// It refuses a message type without a leaf offset, and a slot whose leaf is beyond the leaves of a key.
func SlotLeafIndex(slot primitives.Slot, message signingprotection.SlotMessage) (uint32, error) {
	offset := uint64(message)
	if offset >= LeavesPerSlot {
		return 0, fmt.Errorf("leaf offset %d of %s is not lower than %d", offset, message, LeavesPerSlot)
	}

	const first = ReservedLeafCount + AttestationLeafCount + BuilderBidLeafCount
	const maxSlot = (math.MaxUint32 - first - (LeavesPerSlot - 1)) / LeavesPerSlot

	if uint64(slot) > maxSlot {
		return 0, fmt.Errorf("slot %d is beyond the last slot %d with leaves", slot, uint64(maxSlot))
	}

	return uint32(first + uint64(slot)*LeavesPerSlot + offset), nil // #nosec G115 -- At most 2^32 - 1.
}
