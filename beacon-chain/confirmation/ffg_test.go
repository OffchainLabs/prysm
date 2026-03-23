package confirmation

import (
	"context"
	"testing"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// ffgTestFC extends safetyTestFC with AncestorRoot for checkpoint derivation.
type ffgTestFC struct {
	safetyTestFC
	// ancestors maps (root, slot) → ancestor root for AncestorRoot queries.
	ancestors map[[32]byte]map[primitives.Slot][32]byte
}

func (m *ffgTestFC) AncestorRoot(_ context.Context, root [32]byte, slot primitives.Slot) ([32]byte, error) {
	if slotMap, ok := m.ancestors[root]; ok {
		if r, ok := slotMap[slot]; ok {
			return r, nil
		}
	}
	// Walk parents to find ancestor at slot.
	r := root
	for {
		s, err := m.Slot(r)
		if err != nil {
			return [32]byte{}, err
		}
		if s <= slot {
			return r, nil
		}
		p, err := m.ParentRoot(r)
		if err != nil || p == ([32]byte{}) {
			return [32]byte{}, ErrUnknownRoot
		}
		r = p
	}
}

// TestGetCurrentTargetScore tests that only validators whose implied checkpoint
// matches the current target are counted.
//
// Setup (32 slots/epoch):
//   - root0(slot 0) → root1(slot 1) → root2(slot 33)
//   - AncestorRoot(root2, 32) → root1 (last block at or before slot 32)
//   - Current target: epoch 1, root = root1
//   - Validator 0: votes for root1 (slot 1) → epoch 0, checkpoint at slot 0 = root0 → epoch 0 ≠ 1
//   - Validator 1: votes for root2 (slot 33) → epoch 1, checkpoint at slot 32 = root1 → matches!
//   - Validator 2: votes for root1 (slot 1) → epoch 0, no match
func TestGetCurrentTargetScore(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	root2 := [32]byte{2}

	fc := &ffgTestFC{
		safetyTestFC: safetyTestFC{
			testForkchoiceReader: testForkchoiceReader{
				parents: map[[32]byte][32]byte{
					root1: root0,
					root2: root1,
				},
			},
			slotsByRoot: map[[32]byte]primitives.Slot{
				root0: 0,
				root1: 1,
				root2: spe + 1, // slot 33
			},
		},
	}

	votes := []forkchoicetypes.VoteData{
		{Root: root1, Slot: 1},
		{Root: root2, Slot: spe + 1},
		{Root: root1, Slot: 1},
	}
	ffg := &FFGStateInfo{
		TotalActiveBalance: 300,
		Balances:           []uint64{100, 100, 100},
	}

	// Current target: epoch 1, root = root1 (ancestor of root2 at slot 32).
	currentTarget := forkchoicetypes.Checkpoint{Epoch: primitives.Epoch(1), Root: root1}

	score := GetCurrentTargetScore(context.Background(), fc, ffg, votes, nil, currentTarget)

	// Validator 1 votes for root2 at epoch 1 → checkpoint at slot 32 = root1 → matches
	// Validator 0 and 2 vote at epoch 0 → doesn't match target epoch 1
	require.Equal(t, uint64(100), score)
}

// TestGetCurrentTargetScore_Equivocating tests that equivocating validators are excluded.
func TestGetCurrentTargetScore_Equivocating(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}

	fc := &ffgTestFC{
		safetyTestFC: safetyTestFC{
			testForkchoiceReader: testForkchoiceReader{
				parents: map[[32]byte][32]byte{
					root1: root0,
				},
			},
			slotsByRoot: map[[32]byte]primitives.Slot{
				root0: 0,
				root1: 1,
			},
		},
	}

	votes := []forkchoicetypes.VoteData{
		{Root: root1, Slot: 1},
		{Root: root1, Slot: 1},
	}
	ffg := &FFGStateInfo{
		TotalActiveBalance: 200,
		Balances:           []uint64{100, 100},
	}
	equivocating := map[primitives.ValidatorIndex]bool{1: true}
	target := forkchoicetypes.Checkpoint{Epoch: 0, Root: root0}

	score := GetCurrentTargetScore(context.Background(), fc, ffg, votes, equivocating, target)
	// Validator 1 excluded, only validator 0 counts
	require.Equal(t, uint64(100), score)
}

// TestComputeHonestFFGSupport_CurrentSlotZero tests that ComputeHonestFFGSupport
// returns 0 when currentSlot is 0, avoiding unsigned underflow on currentSlot-1.
func TestComputeHonestFFGSupport_CurrentSlotZero(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}

	fc := &ffgTestFC{
		safetyTestFC: safetyTestFC{
			testForkchoiceReader: testForkchoiceReader{
				parents: map[[32]byte][32]byte{
					root1: root0,
				},
			},
			slotsByRoot: map[[32]byte]primitives.Slot{
				root0: 0,
				root1: 0,
			},
		},
	}

	votes := []forkchoicetypes.VoteData{
		{Root: root1, Slot: 0},
	}
	ffg := &FFGStateInfo{
		TotalActiveBalance: 320_000,
		Balances:           []uint64{320_000},
	}
	target := forkchoicetypes.Checkpoint{Epoch: 0, Root: root0}

	result := ComputeHonestFFGSupport(context.Background(), fc, ffg, votes, nil, target, 0, zeroEquivScorer)
	require.Equal(t, uint64(0), result)
}

// TestWillCurrentTargetBeJustified tests the 2/3 supermajority check.
func TestWillCurrentTargetBeJustified(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}

	fc := &ffgTestFC{
		safetyTestFC: safetyTestFC{
			testForkchoiceReader: testForkchoiceReader{
				parents: map[[32]byte][32]byte{
					root1: root0,
				},
			},
			slotsByRoot: map[[32]byte]primitives.Slot{
				root0: 0,
				root1: 1,
			},
		},
	}

	target := forkchoicetypes.Checkpoint{Epoch: 0, Root: root0}

	// Early in epoch (slot 2 of 32): most weight is remaining → honest future
	// votes dominate → should pass even with small actual support.
	votes := []forkchoicetypes.VoteData{
		{Root: root1, Slot: 1},
		{Root: root1, Slot: 1},
		{Root: root1, Slot: 1},
	}
	ffg := &FFGStateInfo{
		TotalActiveBalance: 300,
		Balances:           []uint64{100, 100, 100},
	}
	honest := func() (uint64, uint64) {
		return ComputeHonestFFGSupport(context.Background(), fc, ffg, votes, nil, target, 2, zeroEquivScorer), ffg.TotalActiveBalance
	}
	result := WillCurrentTargetBeJustified(honest)
	require.Equal(t, true, result)

	// Late in epoch (last slot): almost all weight has been seen.
	// With only 300 of target score out of 32000 total → fails.
	ffgLate := &FFGStateInfo{
		TotalActiveBalance: 32000,
		Balances:           []uint64{100, 100, 100},
	}
	lastSlot := spe - 1
	honestLate := func() (uint64, uint64) {
		return ComputeHonestFFGSupport(context.Background(), fc, ffgLate, votes, nil, target, lastSlot, zeroEquivScorer), ffgLate.TotalActiveBalance
	}
	result = WillCurrentTargetBeJustified(honestLate)
	require.Equal(t, false, result)
}

// TestWillNoConflictingCheckpointBeJustified_TargetIsUnrealized tests the
// short-circuit when current target == unrealized justified.
func TestWillNoConflictingCheckpointBeJustified_TargetIsUnrealized(t *testing.T) {
	target := forkchoicetypes.Checkpoint{Epoch: 5, Root: [32]byte{5}}
	unrealized := forkchoicetypes.Checkpoint{Epoch: 5, Root: [32]byte{5}}

	// Should return true immediately without computing support.
	honest := func() (uint64, uint64) {
		t.Fatal("honest FFG support should not be computed")
		return 0, 0
	}
	result := WillNoConflictingCheckpointBeJustified(honest, target, unrealized)
	require.Equal(t, true, result)
}
