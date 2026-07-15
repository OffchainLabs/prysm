package confirmation

import (
	"context"
	"testing"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// mockForkchoiceReader is a minimal mock of ForkchoiceReader for scaffold tests.
type mockForkchoiceReader struct {
	headRoot  [32]byte
	ujc       *forkchoicetypes.Checkpoint
	finalized *forkchoicetypes.Checkpoint
}

func (m *mockForkchoiceReader) RLock()   {}
func (m *mockForkchoiceReader) RUnlock() {}
func (m *mockForkchoiceReader) CachedHeadRoot() [32]byte {
	return m.headRoot
}
func (m *mockForkchoiceReader) FinalizedCheckpoint() *forkchoicetypes.Checkpoint {
	if m.finalized != nil {
		return m.finalized
	}
	return &forkchoicetypes.Checkpoint{}
}
func (m *mockForkchoiceReader) JustifiedCheckpoint() *forkchoicetypes.Checkpoint {
	return &forkchoicetypes.Checkpoint{}
}
func (m *mockForkchoiceReader) UnrealizedJustifiedCheckpoint() *forkchoicetypes.Checkpoint {
	if m.ujc != nil {
		return m.ujc
	}
	return &forkchoicetypes.Checkpoint{}
}
func (m *mockForkchoiceReader) Slot(_ [32]byte) (primitives.Slot, error) { return 0, nil }
func (m *mockForkchoiceReader) ParentRoot(_ [32]byte) ([32]byte, error) {
	return [32]byte{}, nil
}
func (m *mockForkchoiceReader) IsOptimistic(_ [32]byte) (bool, error) { return false, nil }
func (m *mockForkchoiceReader) AncestorRoot(_ context.Context, _ [32]byte, _ primitives.Slot) ([32]byte, error) {
	return [32]byte{}, nil
}
func (m *mockForkchoiceReader) AncestorRoots(_ [32]byte, _ [32]byte) ([][32]byte, error) {
	return nil, nil
}
func (m *mockForkchoiceReader) IsAncestor(_ [32]byte, _ [32]byte) (bool, error) {
	return false, nil
}
func (m *mockForkchoiceReader) UnrealizedJustification(_ [32]byte) (*forkchoicetypes.Checkpoint, error) {
	return &forkchoicetypes.Checkpoint{}, nil
}
func (m *mockForkchoiceReader) VotingSource(_ [32]byte) (*forkchoicetypes.Checkpoint, error) {
	return &forkchoicetypes.Checkpoint{}, nil
}
func (m *mockForkchoiceReader) TargetRootForEpoch(_ [32]byte, _ primitives.Epoch) ([32]byte, error) {
	return [32]byte{}, nil
}
func (m *mockForkchoiceReader) VoteSnapshot(buf []forkchoicetypes.VoteData) []forkchoicetypes.VoteData {
	return buf
}
func (m *mockForkchoiceReader) SlashedIndices() map[primitives.ValidatorIndex]bool {
	return nil
}

// alwaysJustified opens every FFG gate, 3*total > total and 3*total >= 2*total.
var alwaysJustified HonestFFGSupport = func() (uint64, uint64) { return 320_000, 320_000 }

// mockCommitteeAccessor is a minimal mock of CommitteeAccessor.
type mockCommitteeAccessor struct{}

func (m *mockCommitteeAccessor) Committee(_ context.Context, _ primitives.Slot) ([]primitives.ValidatorIndex, error) {
	return nil, nil
}

func (m *mockCommitteeAccessor) Seed(_ context.Context, epoch primitives.Epoch) ([32]byte, error) {
	// Derive different value per epoch.
	return [32]byte{0x5E, byte(epoch), byte(epoch >> 8)}, nil
}

// mockBalanceAccessor is a minimal mock of BalanceAccessor.
type mockBalanceAccessor struct{}

func (m *mockBalanceAccessor) BalanceInfoByCheckpoint(_ context.Context, _ [32]byte) ([]uint64, uint64, error) {
	return nil, 0, nil
}
func (m *mockBalanceAccessor) PulledUpHeadState(_ context.Context, _ [32]byte) (*FFGStateInfo, error) {
	return &FFGStateInfo{}, nil
}

func TestNew(t *testing.T) {
	anchor := [fieldparams.RootLength]byte{0x01}
	anchorCp := forkchoicetypes.Checkpoint{Epoch: 1, Root: anchor}
	fcr := New(&mockForkchoiceReader{}, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)

	require.Equal(t, anchor, fcr.ConfirmedRoot())
	require.Equal(t, anchor, fcr.previousSlotHead)
	require.Equal(t, anchor, fcr.currentSlotHead)
	require.Equal(t, anchorCp, fcr.currentEpochObservedJustifiedCheckpoint)
	require.Equal(t, anchorCp, fcr.previousEpochObservedJustifiedCheckpoint)
	require.Equal(t, anchorCp, fcr.previousEpochGreatestUnrealizedCheckpoint)
}

func TestOnFastConfirmation_NoChange(t *testing.T) {
	anchor := [fieldparams.RootLength]byte{0x01}
	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: anchor}
	fc := &chainTestFC{
		mockForkchoiceReader: mockForkchoiceReader{headRoot: anchor},
		parents:              map[[32]byte][32]byte{},
		slotsByRoot:          map[[32]byte]primitives.Slot{anchor: 0},
	}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)

	// No blocks, no votes. Confirmed root stays at anchor.
	fcr.OnFastConfirmation(t.Context(), 1)
	require.Equal(t, anchor, fcr.ConfirmedRoot())
}

func TestOnFastConfirmation_StaleHeadRevertsToFinalized(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	finalizedRoot := [32]byte{0xF0}
	head := [32]byte{1}
	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: [32]byte{0x10}}

	fc := &chainTestFC{
		mockForkchoiceReader: mockForkchoiceReader{
			headRoot:  head,
			finalized: &forkchoicetypes.Checkpoint{Epoch: 1, Root: finalizedRoot},
		},
		slotsByRoot: map[[32]byte]primitives.Slot{head: spe},
	}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)

	// Head is in epoch 1 while the clock is at epoch 3, the rule must revert to finalized.
	fcr.OnFastConfirmation(t.Context(), 3*spe)
	require.Equal(t, finalizedRoot, fcr.ConfirmedRoot())
}

// TestUpdateFastConfirmationVariables_GenesisSlot tests that slot 0 (genesis)
// correctly triggers the epoch-boundary rotation (since slot 0 is an epoch start)
// and does not trigger the last-slot-of-epoch snapshot (since slot 1 is not an
// epoch start). This guards against unsigned underflow in slot arithmetic.
func TestUpdateFastConfirmationVariables_GenesisSlot(t *testing.T) {
	anchor := [32]byte{0x10}
	head := [32]byte{1}
	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: anchor}
	ujc := &forkchoicetypes.Checkpoint{Epoch: 5, Root: [32]byte{0xBB}}

	fc := &mockForkchoiceReader{headRoot: head, ujc: ujc}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)

	// At slot 0: IsEpochStart(0) = true → epoch-boundary rotation fires.
	// IsEpochStart(0+1) = IsEpochStart(1) = false → no unrealized snapshot.
	fcr.OnFastConfirmation(t.Context(), 0)

	// Slot head rotation: previous=anchor, current=head
	require.Equal(t, anchor, fcr.previousSlotHead)
	require.Equal(t, head, fcr.currentSlotHead)

	// Epoch-boundary rotation at slot 0:
	// previous_observed = old current_observed (anchorCp)
	// current_observed = previous_greatest_unrealized (anchorCp, since no snapshot happened)
	require.Equal(t, anchorCp, fcr.previousEpochObservedJustifiedCheckpoint)
	require.Equal(t, anchorCp, fcr.currentEpochObservedJustifiedCheckpoint)

	// The unrealized snapshot should NOT have fired (slot 1 is not epoch start),
	// so previousEpochGreatestUnrealizedCheckpoint stays at its initial value.
	require.Equal(t, anchorCp, fcr.previousEpochGreatestUnrealizedCheckpoint)
}

func TestUpdateFastConfirmationVariables_SlotHeadRotation(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	head1 := [32]byte{1}
	head2 := [32]byte{2}
	head3 := [32]byte{3}
	anchor := [32]byte{0x10}
	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: anchor}

	fc := &mockForkchoiceReader{headRoot: head1}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)

	// Slot 1: first call rotates previous=anchor, current=head1
	fcr.OnFastConfirmation(t.Context(), 1)
	require.Equal(t, anchor, fcr.previousSlotHead)
	require.Equal(t, head1, fcr.currentSlotHead)

	// Slot 2: head changes to head2
	fc.headRoot = head2
	fcr.OnFastConfirmation(t.Context(), 2)
	require.Equal(t, head1, fcr.previousSlotHead)
	require.Equal(t, head2, fcr.currentSlotHead)

	// Slot 3: head changes to head3
	fc.headRoot = head3
	fcr.OnFastConfirmation(t.Context(), 3)
	require.Equal(t, head2, fcr.previousSlotHead)
	require.Equal(t, head3, fcr.currentSlotHead)

	// Non-epoch boundary: observed checkpoints don't rotate
	require.Equal(t, anchorCp, fcr.currentEpochObservedJustifiedCheckpoint)
	require.Equal(t, anchorCp, fcr.previousEpochObservedJustifiedCheckpoint)
	_ = spe
}

func TestUpdateFastConfirmationVariables_EpochBoundary(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	anchor := [32]byte{0x10}
	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: anchor}
	ujc := &forkchoicetypes.Checkpoint{Epoch: 1, Root: [32]byte{0xAA}}

	fc := &mockForkchoiceReader{headRoot: anchor, ujc: ujc}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)

	// Run through epoch 0 slots, reaching the last slot
	for slot := primitives.Slot(1); slot < spe-1; slot++ {
		fcr.OnFastConfirmation(t.Context(), slot)
	}
	// At last slot of epoch 0 (slot 31 with spe=32), next slot is epoch start:
	// should snapshot unrealized justified
	fcr.OnFastConfirmation(t.Context(), spe-1)
	require.Equal(t, primitives.Epoch(1), fcr.previousEpochGreatestUnrealizedCheckpoint.Epoch)
	require.Equal(t, [32]byte{0xAA}, fcr.previousEpochGreatestUnrealizedCheckpoint.Root)

	// Observed checkpoints haven't rotated yet (we're still in epoch 0)
	require.Equal(t, anchorCp, fcr.currentEpochObservedJustifiedCheckpoint)
	require.Equal(t, anchorCp, fcr.previousEpochObservedJustifiedCheckpoint)

	// At epoch boundary (slot 32): observed checkpoints rotate
	fcr.OnFastConfirmation(t.Context(), spe)
	require.Equal(t, anchorCp, fcr.previousEpochObservedJustifiedCheckpoint)
	// current observed = previous greatest unrealized = ujc
	require.Equal(t, primitives.Epoch(1), fcr.currentEpochObservedJustifiedCheckpoint.Epoch)
	require.Equal(t, [32]byte{0xAA}, fcr.currentEpochObservedJustifiedCheckpoint.Root)
}

// chainTestFC is a richer mock supporting the full ForkchoiceReader for algorithm tests.
type chainTestFC struct {
	mockForkchoiceReader
	parents                  map[[32]byte][32]byte
	slotsByRoot              map[[32]byte]primitives.Slot
	optimistic               map[[32]byte]bool
	unrealizedJustifications map[[32]byte]*forkchoicetypes.Checkpoint
	votingSources            map[[32]byte]*forkchoicetypes.Checkpoint
}

func (m *chainTestFC) ParentRoot(root [32]byte) ([32]byte, error) {
	p, ok := m.parents[root]
	if !ok {
		return [32]byte{}, nil
	}
	return p, nil
}
func (m *chainTestFC) Slot(root [32]byte) (primitives.Slot, error) {
	s, ok := m.slotsByRoot[root]
	if !ok {
		return 0, ErrUnknownRoot
	}
	return s, nil
}
func (m *chainTestFC) IsOptimistic(root [32]byte) (bool, error) {
	return m.optimistic[root], nil
}
func (m *chainTestFC) IsAncestor(root [32]byte, ancestorRoot [32]byte) (bool, error) {
	if root == ancestorRoot {
		return true, nil
	}
	r := root
	for {
		p, ok := m.parents[r]
		if !ok || p == ([32]byte{}) {
			return false, nil
		}
		if p == ancestorRoot {
			return true, nil
		}
		r = p
	}
}
func (m *chainTestFC) AncestorRoot(_ context.Context, root [32]byte, slot primitives.Slot) ([32]byte, error) {
	r := root
	for {
		s, err := m.Slot(r)
		if err != nil {
			return [32]byte{}, err
		}
		if s <= slot {
			return r, nil
		}
		p, ok := m.parents[r]
		if !ok || p == ([32]byte{}) {
			return [32]byte{}, nil
		}
		r = p
	}
}
func (m *chainTestFC) AncestorRoots(root [32]byte, terminalRoot [32]byte) ([][32]byte, error) {
	termSlot, err := m.Slot(terminalRoot)
	if err != nil {
		return nil, err
	}
	var result [][32]byte
	r := root
	for {
		s, err := m.Slot(r)
		if err != nil {
			return nil, nil
		}
		if s <= termSlot {
			return nil, nil
		}
		result = append(result, r)
		p, ok := m.parents[r]
		if !ok || p == ([32]byte{}) {
			return nil, nil
		}
		if p == terminalRoot {
			for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
				result[i], result[j] = result[j], result[i]
			}
			return result, nil
		}
		r = p
	}
}
func (m *chainTestFC) UnrealizedJustification(root [32]byte) (*forkchoicetypes.Checkpoint, error) {
	if cp, ok := m.unrealizedJustifications[root]; ok {
		return cp, nil
	}
	return &forkchoicetypes.Checkpoint{}, nil
}
func (m *chainTestFC) VotingSource(root [32]byte) (*forkchoicetypes.Checkpoint, error) {
	if cp, ok := m.votingSources[root]; ok {
		return cp, nil
	}
	return &forkchoicetypes.Checkpoint{}, nil
}
func (m *chainTestFC) TargetRootForEpoch(root [32]byte, epoch primitives.Epoch) ([32]byte, error) {
	epochStart, err := slots.EpochStart(epoch)
	if err != nil {
		return [32]byte{}, err
	}
	return m.AncestorRoot(context.Background(), root, epochStart)
}

// TestIsConfirmedChainSafe_Passes tests that a confirmed chain is safe when
// all blocks have sufficient support.
//
// Chain: root0(slot 32, epoch 1 start) → root1(slot 33) → root2(slot 34) → root3(slot 35)
// Observed justified checkpoint: root0 (epoch 1), root0 is the chain's checkpoint block at epoch 1.
// Current slot: 36
func TestIsConfirmedChainSafe_Passes(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	root2 := [32]byte{2}
	root3 := [32]byte{3}

	fc := &chainTestFC{
		mockForkchoiceReader: mockForkchoiceReader{headRoot: root3},
		parents: map[[32]byte][32]byte{
			root1: root0,
			root2: root1,
			root3: root2,
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: spe,     // slot 32
			root1: spe + 1, // slot 33
			root2: spe + 2, // slot 34
			root3: spe + 3, // slot 35
		},
		optimistic: map[[32]byte]bool{},
	}

	anchorCp := forkchoicetypes.Checkpoint{Epoch: 1, Root: root0}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)

	// Build support: every slot from 33-35 has 10,000 voting for root3.
	// TotalSupport for root1 = 30,000 (all descendants credit up).
	// committeeWeight = 320,000 / 32 = 10,000
	// For root1 (slot 33, parent slot 32):
	//   maximumSupport = EstimateWeight(320000, 33, 35) = 10000 * 3 = 30,000
	//   proposerScore = 4,000
	//   adversarial = 30000 * 25/100 = 7,500
	//   threshold = (30000 + 4000 + 2*7500) / 2 = 24,500
	//   attestation score for root1 = 30,000 > 24,500 ✓
	sm := NewSupportMap()
	addTestSupport(sm, spe+1, root3, 10_000)
	addTestSupport(sm, spe+2, root3, 10_000)
	addTestSupport(sm, spe+3, root3, 10_000)
	rebuildTotalSupportFromSlotRoot(sm, fc)

	totalBalance := uint64(320_000)
	currentSlot := spe + 4 // slot 36

	result := fcr.isConfirmedChainSafe(context.Background(), root3, currentSlot, sm, totalBalance, zeroEquivScorer)
	require.Equal(t, true, result)
}

// TestIsConfirmedChainSafe_EmptyChainSegment tests that isConfirmedChainSafe
// returns true when confirmedRoot equals the startRootExclusive (empty chain).
// The spec uses all(is_one_confirmed(...) for root in chain_roots), and all([]) is True.
func TestIsConfirmedChainSafe_EmptyChainSegment(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	root0 := [32]byte{0x10}

	fc := &chainTestFC{
		mockForkchoiceReader: mockForkchoiceReader{headRoot: root0},
		parents:              map[[32]byte][32]byte{},
		slotsByRoot:          map[[32]byte]primitives.Slot{root0: 0},
		optimistic:           map[[32]byte]bool{},
	}

	// confirmedRoot == observed justified root → empty chain segment
	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: root0}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)

	sm := NewSupportMap()
	result := fcr.isConfirmedChainSafe(context.Background(), root0, spe, sm, 320_000, zeroEquivScorer)
	require.Equal(t, true, result)
}

// TestIsConfirmedChainSafe_FailsNotDescendant tests that the check fails when
// confirmed root is not a descendant of the observed justified checkpoint.
func TestIsConfirmedChainSafe_FailsNotDescendant(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	root0 := [32]byte{0x10}
	rootOther := [32]byte{0x20} // not in root0's chain

	fc := &chainTestFC{
		parents:     map[[32]byte][32]byte{},
		slotsByRoot: map[[32]byte]primitives.Slot{root0: 0, rootOther: 1},
	}

	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: root0}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)

	sm := NewSupportMap()
	result := fcr.isConfirmedChainSafe(context.Background(), rootOther, spe, sm, 320_000, zeroEquivScorer)
	require.Equal(t, false, result)
}

// TestFindLatestConfirmedDescendant_Pass1 tests that pass 1 confirms previous-epoch
// blocks and stops at the epoch boundary.
//
// Chain: root0(slot 28) → root1(slot 29) → root2(slot 30) → root3(slot 31) → root4(slot 32)
// Confirmed: root0 (epoch 0, since confirmed epoch+1==current epoch)
// Current slot: 33 (epoch 1)
// Previous slot head: root4 (descendant of all blocks)
// Voting source of previous head: epoch 0 (fresh: 0+2 >= 1)
// At epoch start: pass 1 gate opens.
// root1-root3 should be confirmed (previous epoch), root4 should NOT (current epoch).
func TestFindLatestConfirmedDescendant_Pass1(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	root2 := [32]byte{2}
	root3 := [32]byte{3}
	root4 := [32]byte{4}
	currentSlot := spe + 1 // slot 33, epoch 1

	fc := &chainTestFC{
		mockForkchoiceReader: mockForkchoiceReader{headRoot: root4},
		parents: map[[32]byte][32]byte{
			root1: root0,
			root2: root1,
			root3: root2,
			root4: root3,
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: spe - 4, // slot 28
			root1: spe - 3, // slot 29
			root2: spe - 2, // slot 30
			root3: spe - 1, // slot 31
			root4: spe,     // slot 32 (epoch 1)
		},
		optimistic: map[[32]byte]bool{},
		votingSources: map[[32]byte]*forkchoicetypes.Checkpoint{
			root4: {Epoch: 0}, // fresh: 0+2 >= 1
		},
	}

	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: root0}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)
	fcr.previousSlotHead = root4
	fcr.currentSlotHead = root4

	// High support for all blocks
	sm := NewSupportMap()
	addTestSupport(sm, spe-3, root4, 10_000)
	addTestSupport(sm, spe-2, root4, 10_000)
	addTestSupport(sm, spe-1, root4, 10_000)
	addTestSupport(sm, spe, root4, 10_000)
	rebuildTotalSupportFromSlotRoot(sm, fc)

	totalBalance := uint64(320_000)

	result := fcr.findLatestConfirmedDescendant(
		context.Background(), root0, currentSlot, sm, totalBalance, forkchoicetypes.Checkpoint{}, alwaysJustified, zeroEquivScorer,
	)

	// Pass 1 confirms root1, root2, root3 (previous epoch).
	// Pass 2 also runs and confirms root4 (current epoch) since support is high
	// and unrealized justification is fresh.
	require.Equal(t, root4, result)
}

// TestFindLatestConfirmedDescendant_NoAdvance tests that confirmation doesn't advance
// when support is too low.
func TestFindLatestConfirmedDescendant_NoAdvance(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	currentSlot := spe + 1

	fc := &chainTestFC{
		mockForkchoiceReader: mockForkchoiceReader{headRoot: root1},
		parents: map[[32]byte][32]byte{
			root1: root0,
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: spe - 2,
			root1: spe - 1,
		},
		optimistic: map[[32]byte]bool{},
		votingSources: map[[32]byte]*forkchoicetypes.Checkpoint{
			root1: {Epoch: 0},
		},
	}

	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: root0}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)
	fcr.previousSlotHead = root1
	fcr.currentSlotHead = root1

	// No support at all
	sm := NewSupportMap()
	rebuildTotalSupportFromSlotRoot(sm, fc)
	totalBalance := uint64(320_000)

	result := fcr.findLatestConfirmedDescendant(
		context.Background(), root0, currentSlot, sm, totalBalance, forkchoicetypes.Checkpoint{}, alwaysJustified, zeroEquivScorer,
	)

	// No advance: confirmed stays at root0
	require.Equal(t, root0, result)
}

// TestGetLatestConfirmed_RevertsWhenTooOld tests that confirmed root reverts to
// finalized when it's more than 1 epoch behind.
func TestGetLatestConfirmed_RevertsWhenTooOld(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	root0 := [32]byte{0x10}
	rootFin := [32]byte{0xF0}
	// Confirmed is at epoch 0, current is epoch 3 → epoch 0 + 1 < 3 → revert
	currentSlot := spe * 3

	fc := &chainTestFC{
		mockForkchoiceReader: mockForkchoiceReader{
			headRoot: rootFin,
			ujc:      &forkchoicetypes.Checkpoint{Epoch: 2, Root: rootFin},
		},
		parents: map[[32]byte][32]byte{},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0:   0,
			rootFin: spe * 2,
		},
	}
	fc.mockForkchoiceReader.finalized = &forkchoicetypes.Checkpoint{Epoch: 2, Root: rootFin}

	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: root0}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)
	fcr.currentSlotHead = rootFin

	sm := NewSupportMap()

	result := fcr.getLatestConfirmed(
		context.Background(), currentSlot, sm, 320_000, forkchoicetypes.Checkpoint{}, alwaysJustified, sm, 320_000, zeroEquivScorer, zeroEquivScorer,
	)
	require.Equal(t, rootFin, result)
}

// TestGetLatestConfirmed_RevertsWhenNotCanonical tests reversion when confirmed
// root is not an ancestor of the current head.
func TestGetLatestConfirmed_RevertsWhenNotCanonical(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	root0 := [32]byte{0x10}
	rootHead := [32]byte{0x20}
	rootFin := [32]byte{0xF0}
	currentSlot := spe + 1

	fc := &chainTestFC{
		mockForkchoiceReader: mockForkchoiceReader{
			headRoot: rootHead,
			ujc:      &forkchoicetypes.Checkpoint{Epoch: 0, Root: root0},
		},
		parents: map[[32]byte][32]byte{
			rootHead: rootFin,
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0:    spe - 2,
			rootHead: spe,
			rootFin:  spe - 3,
		},
	}
	fc.mockForkchoiceReader.finalized = &forkchoicetypes.Checkpoint{Epoch: 0, Root: rootFin}

	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: root0}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)
	fcr.currentSlotHead = rootHead

	sm := NewSupportMap()

	// root0 is NOT an ancestor of rootHead (rootHead's parent is rootFin, not root0)
	result := fcr.getLatestConfirmed(
		context.Background(), currentSlot, sm, 320_000, forkchoicetypes.Checkpoint{}, alwaysJustified, sm, 320_000, zeroEquivScorer, zeroEquivScorer,
	)
	require.Equal(t, rootFin, result)
}

// TestGetLatestConfirmed_RestartsToJustified tests that at epoch start, confirmed
// jumps to the observed justified checkpoint when conditions are met.
func TestGetLatestConfirmed_RestartsToJustified(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	rootFin := [32]byte{0xF0}
	rootJust := [32]byte{0xAA} // observed justified
	rootHead := [32]byte{0x20}
	currentSlot := spe // epoch 1 start

	fc := &chainTestFC{
		mockForkchoiceReader: mockForkchoiceReader{headRoot: rootHead},
		parents: map[[32]byte][32]byte{
			rootJust: rootFin,
			rootHead: rootJust,
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			rootFin:  0,
			rootJust: spe - 5,
			rootHead: spe,
		},
		unrealizedJustifications: map[[32]byte]*forkchoicetypes.Checkpoint{
			// Head's unrealized justification matches the observed justified
			rootHead: {Epoch: 0, Root: rootJust},
		},
	}
	fc.mockForkchoiceReader.finalized = &forkchoicetypes.Checkpoint{Epoch: 0, Root: rootFin}

	// Set up FCR: confirmed=rootFin (behind), observed justified=rootJust epoch 0
	anchorCp := forkchoicetypes.Checkpoint{Epoch: 0, Root: rootFin}
	fcr := New(fc, &mockCommitteeAccessor{}, &mockBalanceAccessor{}, anchorCp)
	// Simulate epoch boundary: observed justified is rootJust at epoch 0
	fcr.currentEpochObservedJustifiedCheckpoint = forkchoicetypes.Checkpoint{Epoch: 0, Root: rootJust}
	fcr.previousSlotHead = rootHead
	fcr.currentSlotHead = rootHead

	sm := NewSupportMap()

	result := fcr.getLatestConfirmed(
		context.Background(), currentSlot, sm, 320_000, forkchoicetypes.Checkpoint{}, alwaysJustified, sm, 320_000, zeroEquivScorer, zeroEquivScorer,
	)
	// Phase 2: confirmed should jump from rootFin to rootJust
	// (rootFin slot 0 < rootJust slot 27, and epoch 0+1==1)
	// Phase 3 may try to advance further, but with no support it stays at rootJust
	require.Equal(t, rootJust, result)
}
