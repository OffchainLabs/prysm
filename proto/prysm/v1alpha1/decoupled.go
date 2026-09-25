package eth

import "github.com/OffchainLabs/prysm/v7/encoding/bytesutil"

func (c *CheckpointDecoupled) Copy() *CheckpointDecoupled {
	if c == nil {
		return nil
	}
	return &CheckpointDecoupled{
		Slot: c.Slot,
		Root: bytesutil.SafeCopyBytes(c.Root),
	}
}
