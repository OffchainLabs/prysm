package decoupled

import (
	"bytes"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Spec: the ⊥ block reference, PDF §4. The state hashes this pointer, so it is a value, never nil.
func emptyCheckpoint() *ethpb.CheckpointDecoupled {
	return &ethpb.CheckpointDecoupled{Slot: 0, Root: make([]byte, fieldparams.RootLength)}
}

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

func checkpointsEqual(a, b *ethpb.CheckpointDecoupled) bool {
	if isEmptyCheckpoint(a) || isEmptyCheckpoint(b) {
		return isEmptyCheckpoint(a) && isEmptyCheckpoint(b)
	}
	return a.Slot == b.Slot && bytes.Equal(a.Root, b.Root)
}
