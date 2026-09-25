package decoupled

import (
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Spec: the ⊥ block reference, PDF §4. Tolerant of a nil pointer from a freshly built message.
func isEmptyCheckpoint(c *ethpb.CheckpointDecoupled) bool {
	return c == nil || (c.Slot == 0 && isEmptyRoot(c.Root))
}

func isEmptyRoot(root []byte) bool {
	for _, b := range root {
		if b != 0 {
			return false
		}
	}
	return true
}
