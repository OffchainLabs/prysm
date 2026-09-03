// Package confirmation implements the fast confirmation rule, see
// https://github.com/ethereum/consensus-specs/blob/master/specs/phase0/fast-confirmation.md.
package confirmation

import (
	"context"
	"fmt"
	"sync"
	"time"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// FastConfirmationRule tracks the FastConfirmationStore fields of the spec.
type FastConfirmationRule struct {
	// confirmedRoot is read concurrently by the engine safe hash path while OnFastConfirmation updates it.
	mu            sync.RWMutex
	confirmedRoot [fieldparams.RootLength]byte

	previousSlotHead                          [fieldparams.RootLength]byte
	currentSlotHead                           [fieldparams.RootLength]byte
	variablesUpdatedSlot                      primitives.Slot
	currentEpochObservedJustifiedCheckpoint   forkchoicetypes.Checkpoint
	previousEpochObservedJustifiedCheckpoint  forkchoicetypes.Checkpoint
	previousEpochGreatestUnrealizedCheckpoint forkchoicetypes.Checkpoint

	support        *SupportMap
	prevSupportBuf *SupportMap
	votesBuf       []forkchoicetypes.VoteData

	// Assignment tables for epochs [current-2, current], keyed by shuffling seed.
	tables []*epochSlotTable

	fc         ForkchoiceReader
	committees CommitteeAccessor
	balances   BalanceAccessor
}

// New follows the spec's get_fast_confirmation_store, the anchor is the finalized checkpoint at startup.
func New(fc ForkchoiceReader, committees CommitteeAccessor, balances BalanceAccessor, anchorCheckpoint forkchoicetypes.Checkpoint) *FastConfirmationRule {
	registerMetrics()
	return &FastConfirmationRule{
		confirmedRoot:                             anchorCheckpoint.Root,
		previousSlotHead:                          anchorCheckpoint.Root,
		currentSlotHead:                           anchorCheckpoint.Root,
		currentEpochObservedJustifiedCheckpoint:   anchorCheckpoint,
		previousEpochObservedJustifiedCheckpoint:  anchorCheckpoint,
		previousEpochGreatestUnrealizedCheckpoint: anchorCheckpoint,
		support:    NewSupportMap(),
		fc:         fc,
		committees: committees,
		balances:   balances,
	}
}

func (f *FastConfirmationRule) ConfirmedRoot() [fieldparams.RootLength]byte {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.confirmedRoot
}

func (f *FastConfirmationRule) setConfirmedRoot(root [fieldparams.RootLength]byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.confirmedRoot = root
}

func (f *FastConfirmationRule) PreviousSlotHead() [fieldparams.RootLength]byte {
	return f.previousSlotHead
}

func (f *FastConfirmationRule) CurrentSlotHead() [fieldparams.RootLength]byte {
	return f.currentSlotHead
}

func (f *FastConfirmationRule) PreviousEpochObservedJustifiedCheckpoint() forkchoicetypes.Checkpoint {
	return f.previousEpochObservedJustifiedCheckpoint
}

func (f *FastConfirmationRule) CurrentEpochObservedJustifiedCheckpoint() forkchoicetypes.Checkpoint {
	return f.currentEpochObservedJustifiedCheckpoint
}

func (f *FastConfirmationRule) PreviousEpochGreatestUnrealizedCheckpoint() forkchoicetypes.Checkpoint {
	return f.previousEpochGreatestUnrealizedCheckpoint
}

// OnFastConfirmation must run once per slot at slot start after attestations are applied.
// The spec allows get_latest_confirmed at any point in the slot, so only tree reads hold the forkchoice read lock.
func (f *FastConfirmationRule) OnFastConfirmation(ctx context.Context, currentSlot primitives.Slot) {
	// path defaults to "full"; the stale-head branch flips it to "fallback".
	path := "full"
	defer func(start time.Time) {
		fastConfirmationDuration.WithLabelValues(path).Observe(float64(time.Since(start).Milliseconds()))
	}(time.Now())

	f.fc.RLock()
	headRoot := f.fc.CachedHeadRoot()
	var unrealizedJustified forkchoicetypes.Checkpoint
	if ujc := f.fc.UnrealizedJustifiedCheckpoint(); ujc != nil {
		unrealizedJustified = *ujc
	}
	f.fc.RUnlock()

	f.updateFastConfirmationVariables(currentSlot, headRoot, unrealizedJustified)

	f.fc.RLock()
	headSlot, headSlotErr := f.fc.Slot(f.currentSlotHead)
	confirmedRoot := f.ConfirmedRoot()
	confirmedSlot, confirmedSlotErr := f.fc.Slot(confirmedRoot)
	confirmedIsAnc, confirmedIsAncErr := f.fc.IsAncestor(f.currentSlotHead, confirmedRoot)
	f.fc.RUnlock()

	// The confirmation logic ranges over [.., currentSlot - 1], nothing to confirm at genesis.
	if currentSlot == 0 {
		return
	}

	// A head this stale always reverts to finalized, and the head state cannot compute committees past its next epoch.
	if headSlotErr != nil || slots.ToEpoch(headSlot)+1 < slots.ToEpoch(currentSlot) {
		path = "fallback"
		oldConfirmedRoot := f.ConfirmedRoot()
		f.fc.RLock()

		var (
			finalizedSlot, oldSlot     primitives.Slot
			finalizedSlotOk, oldSlotOk bool
		)

		fc := f.fc.FinalizedCheckpoint()

		// Set finalized slot for metrics.
		if fc != nil {
			if s, err := f.fc.Slot(fc.Root); err == nil {
				finalizedSlot, finalizedSlotOk = s, true
			}
		}
		// Set old slot for fallback detection.
		if s, err := f.fc.Slot(oldConfirmedRoot); err == nil {
			oldSlot, oldSlotOk = s, true
		}

		f.fc.RUnlock()

		if fc != nil {
			f.setConfirmedRoot(fc.Root)

		}

		if finalizedSlotOk {
			fastConfirmationSlot.Set(float64(finalizedSlot))
			if currentSlot >= finalizedSlot {
				fastConfirmationDistance.Set(float64(currentSlot - finalizedSlot))
			}
		}

		// Fallback only when the confirmed root regresses to finality
		// which means the confirmed slot moves backward.
		if finalizedSlotOk && oldSlotOk && finalizedSlot < oldSlot {
			fastConfirmationFallbacksTotal.Inc()
		}
		return
	}

	// Checkpoint state loads can replay from disk, keep them off the forkchoice lock.
	balances, totalActiveBalance, err := f.balances.BalanceInfoByCheckpoint(ctx, f.currentEpochObservedJustifiedCheckpoint)
	if err != nil {
		return
	}
	// Reconfirmation at epoch start uses the previous balance source, spec get_previous_balance_source.
	var prevBalances []uint64
	prevTotalActive := uint64(0)
	if slots.IsEpochStart(currentSlot) {
		prevBalances, prevTotalActive, err = f.balances.BalanceInfoByCheckpoint(ctx, f.previousEpochObservedJustifiedCheckpoint)
		if err != nil {
			prevBalances = nil
		}
	}

	// honest() needs the pulled up head state only on boundary, catch-up, or revert runs.
	getFFG := sync.OnceValue(func() *FFGStateInfo {
		ffg, err := f.balances.PulledUpHeadState(ctx, f.currentSlotHead)
		if err != nil {
			return &FFGStateInfo{TotalActiveBalance: totalActiveBalance}
		}
		return ffg
	})
	// honest() is only reachable at an epoch start once the slot's own block is already head.
	honestReachable := !slots.IsEpochStart(currentSlot) || headSlot >= currentSlot
	// The pulled up head state can copy and advance a state, precompute it off the forkchoice lock.
	if honestReachable && (confirmedSlotErr != nil || confirmedIsAncErr != nil || !confirmedIsAnc || slots.ToEpoch(confirmedSlot) < slots.ToEpoch(currentSlot)) {
		getFFG()
	}

	// Committee assignment tables read the head state, keep it off the forkchoice lock.
	if err := f.rebuildTables(ctx, currentSlot, len(balances)); err != nil {
		return
	}

	f.fc.RLock()
	equivocating := f.fc.SlashedIndices()
	f.votesBuf = f.fc.VoteSnapshot(f.votesBuf[:0])
	f.fc.RUnlock()
	votes := f.votesBuf

	f.support.Build(votes, balances, f.tables, equivocating)

	equivScorer := EquivocationScorer(f.support.EquivocationScore)

	prevSupport := f.support
	prevTotalActiveBalance := totalActiveBalance
	prevEquivScorer := equivScorer
	if prevBalances != nil {
		if f.prevSupportBuf == nil {
			f.prevSupportBuf = NewSupportMap()
		}
		ps := f.prevSupportBuf
		ps.Build(votes, prevBalances, f.tables, equivocating)
		prevSupport = ps
		prevTotalActiveBalance = prevTotalActive
		prevEquivScorer = EquivocationScorer(ps.EquivocationScore)
	}

	f.fc.RLock()
	defer f.fc.RUnlock()

	f.support.Accumulate(f.fc)
	if prevSupport != f.support {
		prevSupport.Accumulate(f.fc)
	}

	currentTarget, err := f.getCurrentTarget(ctx, slots.ToEpoch(currentSlot))
	if err != nil {
		return
	}
	honest := HonestFFGSupport(sync.OnceValues(func() (uint64, uint64) {
		ffg := getFFG()
		s := ComputeHonestFFGSupport(ctx, f.fc, ffg, votes, equivocating, currentTarget, currentSlot, equivScorer)
		return s, ffg.TotalActiveBalance
	}))

	// Back up the old confirmed root for fallback detection.
	oldConfirmedRoot := f.ConfirmedRoot()

	// Update new confirmed root.
	newConfirmedRoot := f.getLatestConfirmed(
		ctx, currentSlot, f.support, totalActiveBalance, currentTarget, honest,
		prevSupport, prevTotalActiveBalance, equivScorer, prevEquivScorer,
	)
	f.setConfirmedRoot(newConfirmedRoot)

	newSlot, err := f.fc.Slot(newConfirmedRoot)
	if err != nil {
		// This should never happen, but if it does, don't update the metrics.
		return
	}

	fastConfirmationSlot.Set(float64(newSlot))
	if currentSlot >= newSlot {
		fastConfirmationDistance.Set(float64(currentSlot - newSlot))
	}

	// Check whether the confirmed root regressed to the finalized checkpoint, which is a fallback.
	finalizedCkpt := f.fc.FinalizedCheckpoint()
	if finalizedCkpt == nil {
		// Cannot get the finalized checkpoint, don't update the metrics.
		return
	}

	if newConfirmedRoot != finalizedCkpt.Root {
		// Not a fallback, no need to check the old confirmed root.
		return
	}

	oldSlot, err := f.fc.Slot(oldConfirmedRoot)
	if err != nil {
		// Cannot get the old confirmed slot, don't update the metrics.
		return
	}

	if newSlot < oldSlot {
		// The confirmed root has regressed to the finalized checkpoint.
		fastConfirmationFallbacksTotal.Inc()
	}
}

// rebuildTables refreshes the per-epoch assignment tables. The tables
// are cached by shuffling seed.
func (f *FastConfirmationRule) rebuildTables(ctx context.Context, currentSlot primitives.Slot, sizeHint int) error {
	epoch := slots.ToEpoch(currentSlot)

	// Bound the start epoch to zero.
	startEpoch := primitives.Epoch(0)
	if epoch > 2 {
		startEpoch = epoch - 2
	}

	// Build tables for epochs [current-2, current].
	tables := make([]*epochSlotTable, 0, 3)
	for e := startEpoch; e <= epoch; e++ {
		// Compute seed, and check whether the table for this epoch is already cached.
		seed, err := f.committees.Seed(ctx, e)
		if err != nil {
			return fmt.Errorf("failed to get shuffling seed for epoch %d: %w", e, err)
		}
		if existing := f.tableForSeed(seed); existing != nil {
			tables = append(tables, existing)
			continue
		}

		// Build a new table for this epoch.
		t, err := newEpochSlotTable(ctx, f.committees, e, seed, sizeHint)
		if err != nil {
			return fmt.Errorf("failed to create epoch slot table: %w", err)
		}
		tables = append(tables, t)
	}

	// Update the tables after successful rebuild.
	f.tables = tables
	return nil
}

// tableForSeed returns the cached table built from the given shuffling seed.
func (f *FastConfirmationRule) tableForSeed(seed [32]byte) *epochSlotTable {
	for _, t := range f.tables {
		if t.seed == seed {
			return t
		}
	}
	return nil
}

// getLatestConfirmed executes the FCR algorithm: revert, restart, then advance.
//
//	<spec fn="get_latest_confirmed" fork="phase0">
//
//	confirmed_root = store.confirmed_root
//	current_epoch = get_current_store_epoch(store)
//	head = get_head(store)
//
//	# Phase 1: Revert to finalized if:
//	# 1) confirmed too old, 2) not canonical, 3) chain safety fails at epoch start
//	if (get_block_epoch(store, confirmed_root) + 1 < current_epoch
//	    or not is_ancestor(store, head, confirmed_root)
//	    or (is_start_slot_at_epoch(current_slot) and not is_confirmed_chain_safe(...))):
//	    confirmed_root = store.finalized_checkpoint.root
//
//	# Phase 2: Restart from observed justified checkpoint at epoch start
//	if (is_start_slot_at_epoch(current_slot)
//	    and compute_epoch_at_slot(get_block_slot(ojc.root)) + 1 == current_epoch
//	    and ojc == unrealized_justifications[head]
//	    and confirmed_slot < get_block_slot(ojc.root)):
//	    confirmed_root = ojc.root
//
//	# Phase 3: Advance
//	if get_block_epoch(store, confirmed_root) + 1 >= current_epoch:
//	    return find_latest_confirmed_descendant(store, confirmed_root)
//	else:
//	    return confirmed_root
//	</spec>
func (f *FastConfirmationRule) getLatestConfirmed(
	ctx context.Context,
	currentSlot primitives.Slot,
	support *SupportMap,
	totalActiveBalance uint64,
	currentTarget forkchoicetypes.Checkpoint,
	honest HonestFFGSupport,
	prevSupport *SupportMap,
	prevTotalActiveBalance uint64,
	equivScorer EquivocationScorer,
	prevEquivScorer EquivocationScorer,
) [32]byte {
	confirmedRoot := f.confirmedRoot
	currentEpoch := slots.ToEpoch(currentSlot)
	// The slot start snapshot keeps every phase on one head even if forkchoice moves mid run.
	head := f.currentSlotHead

	// --- Phase 1: Reversion ---
	confirmedSlot, err := f.fc.Slot(confirmedRoot)
	if err != nil {
		fcp := f.fc.FinalizedCheckpoint()
		if fcp == nil {
			return confirmedRoot
		}
		confirmedRoot = fcp.Root
		confirmedSlot, err = f.fc.Slot(confirmedRoot)
		if err != nil {
			return confirmedRoot
		}
	}
	confirmedEpoch := slots.ToEpoch(confirmedSlot)

	revert := false
	if confirmedEpoch+1 < currentEpoch {
		revert = true
	} else {
		isAnc, err := f.fc.IsAncestor(head, confirmedRoot)
		if err == nil && !isAnc {
			fastConfirmationReorgsTotal.Inc()
		}

		if err != nil || !isAnc {
			revert = true
		}
	}
	if !revert && slots.IsEpochStart(currentSlot) {
		if !f.isConfirmedChainSafe(ctx, confirmedRoot, currentSlot, prevSupport, prevTotalActiveBalance, prevEquivScorer) {
			revert = true
		}
	}
	if revert {
		fc := f.fc.FinalizedCheckpoint()
		if fc != nil {
			confirmedRoot = fc.Root
		}
	}

	// --- Phase 2: Restart from observed justified ---
	// The spec gates on the epoch of the checkpoint's block slot, not the checkpoint epoch, they differ across skipped boundary slots.
	ojc := f.currentEpochObservedJustifiedCheckpoint
	if slots.IsEpochStart(currentSlot) {
		ojcSlot, err := f.fc.Slot(ojc.Root)
		if err == nil && slots.ToEpoch(ojcSlot)+1 == currentEpoch {
			headUJ, err := f.fc.UnrealizedJustification(head)
			if err == nil && headUJ.Epoch == ojc.Epoch && headUJ.Root == ojc.Root {
				confirmedSlot, err := f.fc.Slot(confirmedRoot)
				if err == nil && confirmedSlot < ojcSlot {
					confirmedRoot = ojc.Root
					fastConfirmationRestartsTotal.Inc()
				}
			}
		}
	}

	// --- Phase 3: Advance ---
	confirmedSlot, err = f.fc.Slot(confirmedRoot)
	if err != nil {
		return confirmedRoot
	}
	if slots.ToEpoch(confirmedSlot)+1 >= currentEpoch {
		return f.findLatestConfirmedDescendant(
			ctx, confirmedRoot, currentSlot, support, totalActiveBalance, currentTarget, honest, equivScorer,
		)
	}
	return confirmedRoot
}

// isConfirmedChainSafe reconfirms at epoch boundaries with the previous balance source, resetting the GST assumption.
//
//	<spec fn="is_confirmed_chain_safe" fork="phase0">
//
//	if fcr_store.current_epoch_observed_justified_checkpoint != get_checkpoint_for_block(
//	        store, confirmed_root, fcr_store.current_epoch_observed_justified_checkpoint.epoch):
//	    return False
//	current_epoch = get_current_store_epoch(store)
//	if store.current_epoch_observed_justified_checkpoint.epoch + 1 >= current_epoch:
//	    start_root_exclusive = store.current_epoch_observed_justified_checkpoint.root
//	else:
//	    ancestor_at_previous_epoch_start = get_ancestor(
//	        store, confirmed_root, compute_start_slot_at_epoch(current_epoch - 1))
//	    if get_block_epoch(store, ancestor_at_previous_epoch_start) + 1 == current_epoch:
//	        start_root_exclusive = store.blocks[ancestor_at_previous_epoch_start].parent_root
//	    else:
//	        start_root_exclusive = ancestor_at_previous_epoch_start
//	chain_roots = get_ancestor_roots(store, confirmed_root, start_root_exclusive)
//	return all(is_one_confirmed(store, get_previous_balance_source(store), root) for root in chain_roots)
//	</spec>
func (f *FastConfirmationRule) isConfirmedChainSafe(
	ctx context.Context,
	confirmedRoot [32]byte,
	currentSlot primitives.Slot,
	prevSupport *SupportMap,
	prevTotalActiveBalance uint64,
	equivScorer EquivocationScorer,
) bool {
	ojc := f.currentEpochObservedJustifiedCheckpoint

	// The checkpoint of confirmed_root at ojc.Epoch must BE the observed justified checkpoint, ancestry alone is not enough.
	ojcEpochStart, err := slots.EpochStart(ojc.Epoch)
	if err != nil {
		return false
	}
	cpRoot, err := f.fc.AncestorRoot(ctx, confirmedRoot, ojcEpochStart)
	if err != nil || cpRoot != ojc.Root {
		return false
	}

	currentEpoch := slots.ToEpoch(currentSlot)

	var startRootExclusive [32]byte
	if ojc.Epoch+1 >= currentEpoch {
		startRootExclusive = ojc.Root
	} else {
		prevEpochStart, err := slots.EpochStart(currentEpoch - 1)
		if err != nil {
			return false
		}
		ancestorAtPrevStart, err := f.fc.AncestorRoot(ctx, confirmedRoot, prevEpochStart)
		if err != nil {
			return false
		}
		ancestorSlot, err := f.fc.Slot(ancestorAtPrevStart)
		if err != nil {
			return false
		}
		if slots.ToEpoch(ancestorSlot)+1 == currentEpoch {
			// The ancestor is from the previous epoch — use its parent as start.
			parent, err := f.fc.ParentRoot(ancestorAtPrevStart)
			if err != nil {
				return false
			}
			startRootExclusive = parent
		} else {
			// The ancestor is from an older epoch.
			startRootExclusive = ancestorAtPrevStart
		}
	}

	chainRoots, err := f.fc.AncestorRoots(confirmedRoot, startRootExclusive)
	if err != nil {
		return false
	}
	// Spec: all(is_one_confirmed(...) for root in chain_roots).
	// all([]) is True, so an empty chain segment is safe.
	if len(chainRoots) == 0 {
		return true
	}

	for _, root := range chainRoots {
		if !IsOneConfirmed(f.fc, prevSupport, prevTotalActiveBalance, root, currentSlot, equivScorer) {
			return false
		}
	}
	return true
}

// findLatestConfirmedDescendant implements find_latest_confirmed_descendant, pass 1 covers previous-epoch blocks, pass 2 current-epoch ones.
func (f *FastConfirmationRule) findLatestConfirmedDescendant(
	ctx context.Context,
	confirmedRoot [32]byte,
	currentSlot primitives.Slot,
	support *SupportMap,
	totalActiveBalance uint64,
	currentTarget forkchoicetypes.Checkpoint,
	honest HonestFFGSupport,
	equivScorer EquivocationScorer,
) [32]byte {
	currentEpoch := slots.ToEpoch(currentSlot)

	// Pass 1: confirm previous-epoch blocks.
	confirmedRoot = f.confirmPreviousEpochBlocks(ctx, confirmedRoot, currentSlot, currentEpoch, support, totalActiveBalance, currentTarget, honest, equivScorer)

	// Pass 2: confirm current-epoch blocks.
	confirmedRoot = f.confirmCurrentEpochBlocks(ctx, confirmedRoot, currentSlot, currentEpoch, support, totalActiveBalance, currentTarget, honest, equivScorer)

	return confirmedRoot
}

// confirmPreviousEpochBlocks is pass 1 of find_latest_confirmed_descendant.
func (f *FastConfirmationRule) confirmPreviousEpochBlocks(
	ctx context.Context,
	confirmedRoot [32]byte,
	currentSlot primitives.Slot,
	currentEpoch primitives.Epoch,
	support *SupportMap,
	totalActiveBalance uint64,
	currentTarget forkchoicetypes.Checkpoint,
	honest HonestFFGSupport,
	equivScorer EquivocationScorer,
) [32]byte {
	head := f.currentSlotHead
	confirmedSlot, err := f.fc.Slot(confirmedRoot)
	if err != nil {
		return confirmedRoot
	}
	if slots.ToEpoch(confirmedSlot)+1 != currentEpoch {
		return confirmedRoot
	}
	prevVS, err := f.fc.VotingSource(f.previousSlotHead)
	if err != nil || prevVS.Epoch+2 < currentEpoch {
		return confirmedRoot
	}
	if !f.pass1GateOpen(head, currentSlot, currentEpoch, currentTarget, honest) {
		return confirmedRoot
	}

	roots, err := f.fc.AncestorRoots(head, confirmedRoot)
	if err != nil {
		return confirmedRoot
	}
	for _, blockRoot := range roots {
		blockSlot, err := f.fc.Slot(blockRoot)
		if err != nil || slots.ToEpoch(blockSlot) == currentEpoch {
			break
		}
		isAnc, err := f.fc.IsAncestor(f.previousSlotHead, blockRoot)
		if err != nil || !isAnc {
			break
		}
		if !IsOneConfirmed(f.fc, support, totalActiveBalance, blockRoot, currentSlot, equivScorer) {
			break
		}
		confirmedRoot = blockRoot
	}
	return confirmedRoot
}

// pass1GateOpen requires an epoch start, or FFG safety plus fresh unrealized justification.
func (f *FastConfirmationRule) pass1GateOpen(
	head [32]byte,
	currentSlot primitives.Slot,
	currentEpoch primitives.Epoch,
	currentTarget forkchoicetypes.Checkpoint,
	honest HonestFFGSupport,
) bool {
	if slots.IsEpochStart(currentSlot) {
		return true
	}
	ujcStore := f.fc.UnrealizedJustifiedCheckpoint()
	unrealizedJustified := forkchoicetypes.Checkpoint{}
	if ujcStore != nil {
		unrealizedJustified = *ujcStore
	}

	if !WillNoConflictingCheckpointBeJustified(honest, currentTarget, unrealizedJustified) {
		return false
	}
	prevUJ, err1 := f.fc.UnrealizedJustification(f.previousSlotHead)
	headUJ, err2 := f.fc.UnrealizedJustification(head)
	prevFresh := err1 == nil && prevUJ.Epoch+1 >= currentEpoch
	headFresh := err2 == nil && headUJ.Epoch+1 >= currentEpoch
	return prevFresh || headFresh
}

// confirmCurrentEpochBlocks is pass 2 of find_latest_confirmed_descendant.
func (f *FastConfirmationRule) confirmCurrentEpochBlocks(
	ctx context.Context,
	confirmedRoot [32]byte,
	currentSlot primitives.Slot,
	currentEpoch primitives.Epoch,
	support *SupportMap,
	totalActiveBalance uint64,
	currentTarget forkchoicetypes.Checkpoint,
	honest HonestFFGSupport,
	equivScorer EquivocationScorer,
) [32]byte {
	head := f.currentSlotHead
	headUJ, err := f.fc.UnrealizedJustification(head)
	if !slots.IsEpochStart(currentSlot) && (err != nil || headUJ.Epoch+1 < currentEpoch) {
		return confirmedRoot
	}

	roots, err := f.fc.AncestorRoots(head, confirmedRoot)
	if err != nil || len(roots) == 0 {
		return confirmedRoot
	}

	tentative := confirmedRoot
	tentativeSlot, err := f.fc.Slot(tentative)
	if err != nil {
		return confirmedRoot
	}

	for _, blockRoot := range roots {
		blockSlot, err := f.fc.Slot(blockRoot)
		if err != nil {
			break
		}
		// First time crossing into current epoch: check justification.
		if slots.ToEpoch(blockSlot) > slots.ToEpoch(tentativeSlot) {
			if !WillCurrentTargetBeJustified(honest) {
				break
			}
		}
		if !IsOneConfirmed(f.fc, support, totalActiveBalance, blockRoot, currentSlot, equivScorer) {
			break
		}
		tentative = blockRoot
		tentativeSlot = blockSlot
	}

	// Final safety gate.
	return f.applyPass2SafetyGate(confirmedRoot, tentative, currentSlot, currentEpoch, currentTarget, honest)
}

// applyPass2SafetyGate accepts the tentative root only when it cannot be reorged out this epoch or the next.
func (f *FastConfirmationRule) applyPass2SafetyGate(
	confirmedRoot, tentative [32]byte,
	currentSlot primitives.Slot,
	currentEpoch primitives.Epoch,
	currentTarget forkchoicetypes.Checkpoint,
	honest HonestFFGSupport,
) [32]byte {
	if tentative == confirmedRoot {
		return confirmedRoot
	}
	tentativeSlot, err := f.fc.Slot(tentative)
	if err != nil {
		return confirmedRoot
	}
	tentativeEpoch := slots.ToEpoch(tentativeSlot)

	if tentativeEpoch == currentEpoch {
		return tentative
	}

	tentativeVS, err := f.fc.VotingSource(tentative)
	if err != nil || tentativeVS.Epoch+2 < currentEpoch {
		return confirmedRoot
	}

	if slots.IsEpochStart(currentSlot) {
		return tentative
	}

	ujcStore := f.fc.UnrealizedJustifiedCheckpoint()
	unrealizedJustified := forkchoicetypes.Checkpoint{}
	if ujcStore != nil {
		unrealizedJustified = *ujcStore
	}
	if WillNoConflictingCheckpointBeJustified(honest, currentTarget, unrealizedJustified) {
		return tentative
	}
	return confirmedRoot
}

// getCurrentTarget implements the spec's get_current_target.
func (f *FastConfirmationRule) getCurrentTarget(ctx context.Context, currentEpoch primitives.Epoch) (forkchoicetypes.Checkpoint, error) {
	head := f.currentSlotHead
	epochStart, err := slots.EpochStart(currentEpoch)
	if err != nil {
		return forkchoicetypes.Checkpoint{}, err
	}
	cpRoot, err := f.fc.AncestorRoot(ctx, head, epochStart)
	if err != nil {
		return forkchoicetypes.Checkpoint{}, err
	}
	return forkchoicetypes.Checkpoint{Epoch: currentEpoch, Root: cpRoot}, nil
}

// updateFastConfirmationVariables must run before get_latest_confirmed, once per slot.
//
//	<spec fn="update_fast_confirmation_variables" fork="phase0">
//
//	store.previous_slot_head = store.current_slot_head
//	store.current_slot_head = get_head(store)
//
//	# Snapshot unrealized justified at last slot of epoch
//	if is_start_slot_at_epoch(Slot(get_current_slot(store) + 1)):
//	    store.previous_epoch_greatest_unrealized_checkpoint = store.unrealized_justified_checkpoint
//
//	# Rotate observed justified at epoch boundary
//	if is_start_slot_at_epoch(get_current_slot(store)):
//	    store.previous_epoch_observed_justified_checkpoint = (
//	        store.current_epoch_observed_justified_checkpoint)
//	    store.current_epoch_observed_justified_checkpoint = (
//	        store.previous_epoch_greatest_unrealized_checkpoint)
//	</spec>
func (f *FastConfirmationRule) updateFastConfirmationVariables(currentSlot primitives.Slot, headRoot [fieldparams.RootLength]byte, unrealizedJustified forkchoicetypes.Checkpoint) {
	if f.variablesUpdatedSlot == currentSlot && currentSlot != 0 {
		return
	}
	f.variablesUpdatedSlot = currentSlot

	// Rotate slot heads.
	f.previousSlotHead = f.currentSlotHead
	f.currentSlotHead = headRoot

	// At the last slot of the epoch:
	// snapshot the store-level unrealized justified checkpoint.
	if slots.IsEpochStart(currentSlot + 1) {
		f.previousEpochGreatestUnrealizedCheckpoint = unrealizedJustified
	}

	// At epoch boundary: rotate observed justified checkpoints.
	if slots.IsEpochStart(currentSlot) {
		f.previousEpochObservedJustifiedCheckpoint = f.currentEpochObservedJustifiedCheckpoint
		f.currentEpochObservedJustifiedCheckpoint = f.previousEpochGreatestUnrealizedCheckpoint
	}
}
