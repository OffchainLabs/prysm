package params

import "github.com/OffchainLabs/prysm/v6/consensus-types/primitives"

const BasisPoints = primitives.BP(10000)

// SlotBP returns the basis points for a given slot.
func SlotBP() primitives.BP {
	return primitives.BP(12000)
}
