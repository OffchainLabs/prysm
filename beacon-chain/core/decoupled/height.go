package decoupled

import (
	"slices"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// Spec: process_height_events, finalization first, then a target quorum, then a progress quorum after the timeout delay.
func ProcessHeightEvents(st state.BeaconState) error {
	target, err := st.TargetParticipation()
	if err != nil {
		return err
	}
	n := target.Len()
	pending, err := st.PendingFinality()
	if err != nil {
		return err
	}
	last := -1
	for i, p := range pending {
		if quorum(bitfield.Bitlist(p.Participation).Count(), n) {
			last = i
		}
	}
	if last >= 0 {
		p := pending[last]
		if err := st.SetFinalizedHeight(p.Height); err != nil {
			return err
		}
		if err := st.SetFinalized(p.Target); err != nil {
			return err
		}
		if err := st.SetFinalizedCheckpoint(checkpoint(p.Target)); err != nil {
			return err
		}
		pending = pending[last+1:]
		if err := st.SetPendingFinality(pending); err != nil {
			return err
		}
	}
	entry, err := st.Entry()
	if err != nil {
		return err
	}
	if quorum(target.Count(), n) {
		height, err := st.Height()
		if err != nil {
			return err
		}
		if err := st.SetJustifiedHeight(height); err != nil {
			return err
		}
		if err := st.SetJustified(entry); err != nil {
			return err
		}
		if err := st.SetCurrentJustifiedCheckpoint(checkpoint(entry)); err != nil {
			return err
		}
		pending = append(slices.Clone(pending), &ethpb.PendingFinality{Height: height, Target: entry, Participation: bitfield.NewBitlist(n)})
		if len(pending) > fieldparams.MaxPendingFinality {
			pending = pending[len(pending)-fieldparams.MaxPendingFinality:]
		}
		if err := st.SetPendingFinality(pending); err != nil {
			return err
		}
		return advanceHeight(st, n)
	}
	progress, err := st.Progress()
	if err != nil {
		return err
	}
	if quorum(progress.Count(), n) && st.Slot() >= entry.Slot+params.BeaconConfig().DecoupledTimeoutDelay {
		return advanceHeight(st, n)
	}
	return nil
}

// Spec: q = ceil(2n/3) over unit weights, n counts the whole registry including exited validators, not stake.
func quorum(count, n uint64) bool {
	return n > 0 && 3*count >= 2*n
}

// Spec: advance_height. The new entry is the block being processed, whose root is only known once its state root is.
func advanceHeight(st state.BeaconState, n uint64) error {
	height, err := st.Height()
	if err != nil {
		return err
	}
	if err := st.SetHeight(height + 1); err != nil {
		return err
	}
	if err := st.SetEntry(&ethpb.CheckpointDecoupled{Slot: st.Slot(), Root: make([]byte, 32)}); err != nil {
		return err
	}
	if err := st.SetTargetParticipation(bitfield.NewBitlist(n)); err != nil {
		return err
	}
	return st.SetProgress(bitfield.NewBitlist(n))
}

// Runs at the next slot with the latest block root, the first time it is computable after an advance.
func FillEntryRoot(st state.BeaconState, root [32]byte) error {
	entry, err := st.Entry()
	if err != nil {
		return err
	}
	if !bytesutil.ZeroRoot(entry.Root) {
		return nil
	}
	return st.SetEntry(&ethpb.CheckpointDecoupled{Slot: entry.Slot, Root: root[:]})
}

// Gloas finality consumers read epoch checkpoints, so J and F are mirrored there by the epoch of their slot.
func checkpoint(c *ethpb.CheckpointDecoupled) *ethpb.Checkpoint {
	return &ethpb.Checkpoint{Epoch: slots.ToEpoch(c.Slot), Root: bytesutil.SafeCopyBytes(c.Root)}
}
