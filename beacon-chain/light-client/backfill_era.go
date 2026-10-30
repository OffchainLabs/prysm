package light_client

import (
	"bytes"
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	light_client "github.com/OffchainLabs/prysm/v7/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/era"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// NewLightClientEpochDataFromEra replays the blocks of blockFile and builds the record of each
// epoch E with start_slot(E-1) in [S, S+SLOTS_PER_HISTORICAL_ROOT).
//
//   - startFile holds the state at slot S. Its own blocks are read only when slot S is empty, to
//     find the parent of the first record.
//   - blockFile holds the blocks of slots [S, S+SLOTS_PER_HISTORICAL_ROOT). For the genesis era it
//     must hold the genesis block, as Nimbus writes it.
//   - nextFile holds the block at S+SLOTS_PER_HISTORICAL_ROOT. When it is nil, the record of epoch
//     (S+SLOTS_PER_HISTORICAL_ROOT)/SLOTS_PER_EPOCH is not built.
//
// Every replayed block's state root is checked. Records at or before the Altair fork epoch are
// not built. It also returns, keyed by period, the current sync committee of each period that has
// a checkpoint block.
func NewLightClientEpochDataFromEra(
	ctx context.Context,
	startFile, blockFile, nextFile *era.Reader,
) ([]light_client.EpochData, map[uint64]*pb.SyncCommittee, error) {
	cfg := params.BeaconConfig()
	start := startFile.StateSlot()
	end := start + cfg.SlotsPerHistoricalRoot
	if blockFile.StateSlot() != end {
		return nil, nil, fmt.Errorf("block file has its state at slot %d, want %d", blockFile.StateSlot(), end)
	}
	if nextFile != nil && nextFile.StateSlot() != end+cfg.SlotsPerHistoricalRoot {
		return nil, nil, fmt.Errorf("next file has its state at slot %d, want %d", nextFile.StateSlot(), end+cfg.SlotsPerHistoricalRoot)
	}
	epoch := slots.ToEpoch(start) + 1 // the record being collected
	lastEpoch := slots.ToEpoch(end)
	if nextFile == nil {
		lastEpoch--
	}
	if lastEpoch <= cfg.AltairForkEpoch {
		return nil, nil, nil
	}
	// The committee caches hold a few epochs. Shared with other replays, they would keep evicting
	// each other's entries.
	ctx = helpers.WithIsolatedCaches(ctx)

	// The state of startFile already has the block at slot start applied, if there is one.
	st, err := startFile.State()
	if err != nil {
		return nil, nil, err
	}
	finality := make(map[primitives.Epoch]*FinalizedRootProof)
	parent, err := blockFile.Block(start)
	if err != nil {
		return nil, nil, err
	}
	if parent != nil {
		if finality[slots.ToEpoch(start)], err = NewFinalizedRootProofFromBeaconState(ctx, st); err != nil {
			return nil, nil, err
		}
	} else if parent, err = startFile.Block(st.LatestBlockHeader().Slot); err != nil {
		return nil, nil, err
	} else if parent == nil {
		return nil, nil, fmt.Errorf("start file has no block at slot %d", st.LatestBlockHeader().Slot)
	}

	last := parent // the latest block so far
	var blks []interfaces.ReadOnlySignedBeaconBlock
	var records []light_client.EpochData
	committees := make(map[uint64]*pb.SyncCommittee)

	// finish builds the record of epoch from the blocks collected so far. st must be the
	// post-state of last.
	finish := func() error {
		if epoch > cfg.AltairForkEpoch {
			var committeeProof *CurrentSyncCommitteeProof
			var err error
			if len(blks) > 0 {
				if committeeProof, err = NewCurrentSyncCommitteeProofFromBeaconState(ctx, st); err != nil {
					return err
				}
				period := slots.SyncCommitteePeriod(slots.ToEpoch(last.Block().Slot()))
				if committees[period] == nil {
					if committees[period], err = st.CurrentSyncCommittee(); err != nil {
						return err
					}
				}
			}
			rec, err := NewLightClientEpochData(ctx, epoch, parent, blks, finality[epoch-1], committeeProof)
			if err != nil {
				return errors.Wrapf(err, "could not build record for epoch %d", epoch)
			}
			records = append(records, rec)
		}
		delete(finality, epoch-1)
		if len(blks) > 0 {
			parent = blks[len(blks)-1]
		}
		blks = nil
		epoch++
		return nil
	}

	for slot := start + 1; slot <= end && epoch <= lastEpoch; slot++ {
		file := blockFile
		if slot == end {
			if nextFile == nil {
				break
			}
			file = nextFile
		}
		blk, err := file.Block(slot)
		if err != nil {
			return nil, nil, err
		}
		if blk == nil {
			continue
		}
		// Records whose start slot is before this block are complete.
		for epoch <= lastEpoch && slots.UnsafeEpochStart(epoch) < slot {
			if err := finish(); err != nil {
				return nil, nil, err
			}
		}
		if epoch > lastEpoch {
			break
		}
		if st, err = replayBlock(ctx, st, blk); err != nil {
			return nil, nil, errors.Wrapf(err, "could not replay block at slot %d", slot)
		}
		if slots.ToEpoch(slot) > slots.ToEpoch(last.Block().Slot()) {
			if finality[slots.ToEpoch(slot)], err = NewFinalizedRootProofFromBeaconState(ctx, st); err != nil {
				return nil, nil, err
			}
		}
		blks = append(blks, blk)
		last = blk
	}
	for epoch <= lastEpoch {
		if err := finish(); err != nil {
			return nil, nil, err
		}
	}
	return records, committees, nil
}

// replayBlock applies blk to st and checks the state root. Unlike
// transition.ExecuteStateTransitionNoVerifyAnySig, it does not collect the signature sets, which
// aggregate the public keys of every attestation only to be discarded.
func replayBlock(ctx context.Context, st state.BeaconState, blk interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
	st, err := transition.ProcessSlots(ctx, st, blk.Block().Slot())
	if err != nil {
		return nil, errors.Wrap(err, "could not process slots")
	}
	// The block's attestations need the committees of this epoch and the previous one, and every
	// attestation looks up the proposer. Cache them before, or each lookup scans the validators
	// until a background fill lands.
	epoch := slots.ToEpoch(st.Slot())
	for _, e := range []primitives.Epoch{max(epoch, 1) - 1, epoch} {
		if _, err := helpers.ActiveValidatorIndices(ctx, st, e); err != nil {
			return nil, errors.Wrapf(err, "could not get active validators of epoch %d", e)
		}
	}
	helpers.WaitForCommitteeCacheFills(ctx)
	if st, err = transition.ProcessBlockForStateRoot(ctx, st, blk); err != nil {
		return nil, errors.Wrap(err, "could not process block")
	}
	root, err := st.HashTreeRoot(ctx)
	if err != nil {
		return nil, err
	}
	if want := blk.Block().StateRoot(); !bytes.Equal(root[:], want[:]) {
		return nil, fmt.Errorf("could not validate state root, wanted: %#x, received: %#x", want, root)
	}
	return st, nil
}
