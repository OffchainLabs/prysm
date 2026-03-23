package confirmation

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// safetyTestFC extends testForkchoiceReader with slot lookups.
type safetyTestFC struct {
	testForkchoiceReader
	slotsByRoot map[[32]byte]primitives.Slot
	optimistic  map[[32]byte]bool
}

func (m *safetyTestFC) Slot(root [32]byte) (primitives.Slot, error) {
	s, ok := m.slotsByRoot[root]
	if !ok {
		return 0, ErrUnknownRoot
	}
	return s, nil
}

func (m *safetyTestFC) IsOptimistic(root [32]byte) (bool, error) {
	return m.optimistic[root], nil
}

// errUnknownRoot is a sentinel for test lookups.
var ErrUnknownRoot = errMissingRoot("unknown root")

type errMissingRoot string

func (e errMissingRoot) Error() string { return string(e) }

var zeroEquivScorer EquivocationScorer = func(_, _ primitives.Slot) uint64 { return 0 }

// TestIsOneConfirmed_FullParticipation tests that a block is confirmed when
// all committee members vote for it (high attestation score, low threshold).
//
// Setup:
//   - Chain: root0(slot 0) → root1(slot 1) → root2(slot 2)
//   - Current slot: 3
//   - All validators in slots 1-2 vote for root2
//   - Total active balance: 320,000 (10,000 per slot with 32 slots/epoch)
func TestIsOneConfirmed_FullParticipation(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	root2 := [32]byte{2}
	currentSlot := primitives.Slot(3)
	totalBalance := uint64(320_000)

	fc := &safetyTestFC{
		testForkchoiceReader: testForkchoiceReader{
			parents: map[[32]byte][32]byte{
				root1: root0,
				root2: root1,
			},
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: 0,
			root1: 1,
			root2: 2,
		},
		optimistic: map[[32]byte]bool{},
	}

	// Build support: committee members in slots 1-2 all voted for root2
	sm := NewSupportMap()
	addTestSupport(sm, 1, root2, 10_000)
	addTestSupport(sm, 2, root2, 10_000)
	rebuildTotalSupportFromSlotRoot(sm, fc)

	// root2 has attestation score = 20,000 (direct: 10k + 10k, all ancestors credited too)
	require.Equal(t, uint64(20_000), sm.AttestationScore(root2))

	// Threshold for root2: parent is root1 at slot 1
	// maximum_support = EstimateWeight(totalBalance, slot 2, slot 2) = 10,000
	// proposer_score = 10,000 * 40 / 100 = 4,000
	// adversarial = EstimateWeight(totalBalance, slot 2, slot 2) * 25/100 = 2,500
	// discount = 0 (no empty slots between root1 and root2)
	// threshold = (10,000 + 4,000 + 2*2,500 - 0) / 2 = 9,500
	threshold := ComputeSafetyThreshold(fc, sm, totalBalance, root2, currentSlot, zeroEquivScorer)
	require.Equal(t, uint64(9500), threshold)

	// 20,000 > 9,500 → confirmed
	require.Equal(t, true, IsOneConfirmed(fc, sm, totalBalance, root2, currentSlot, zeroEquivScorer))
}

// TestIsOneConfirmed_LowParticipation tests that a block is NOT confirmed when
// attestation support is too low.
func TestIsOneConfirmed_LowParticipation(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	root2 := [32]byte{2}
	currentSlot := primitives.Slot(3)
	totalBalance := uint64(320_000)

	fc := &safetyTestFC{
		testForkchoiceReader: testForkchoiceReader{
			parents: map[[32]byte][32]byte{
				root1: root0,
				root2: root1,
			},
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: 0,
			root1: 1,
			root2: 2,
		},
		optimistic: map[[32]byte]bool{},
	}

	// Very low support: only 1,000 out of 10,000 per slot
	sm := NewSupportMap()
	addTestSupport(sm, 2, root2, 1_000)
	rebuildTotalSupportFromSlotRoot(sm, fc)

	// 1,000 < threshold → not confirmed
	require.Equal(t, false, IsOneConfirmed(fc, sm, totalBalance, root2, currentSlot, zeroEquivScorer))
}

// TestIsOneConfirmed_EmptySlotDiscount tests that empty slots between parent
// and block reduce the safety threshold.
func TestIsOneConfirmed_EmptySlotDiscount(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	// root2 is at slot 4, parent root1 at slot 1 → slots 2, 3 are empty
	root2 := [32]byte{2}
	currentSlot := primitives.Slot(5)
	totalBalance := uint64(320_000)
	spe := params.BeaconConfig().SlotsPerEpoch
	_ = spe

	fc := &safetyTestFC{
		testForkchoiceReader: testForkchoiceReader{
			parents: map[[32]byte][32]byte{
				root1: root0,
				root2: root1,
			},
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: 0,
			root1: 1,
			root2: 4,
		},
		optimistic: map[[32]byte]bool{},
	}

	// Validators in empty slots 2-3 voted for parent (root1)
	sm := NewSupportMap()
	addTestSupport(sm, 2, root1, 10_000)
	addTestSupport(sm, 3, root1, 10_000)
	addTestSupport(sm, 4, root2, 10_000)
	rebuildTotalSupportFromSlotRoot(sm, fc)

	discount := ComputeEmptySlotSupportDiscount(fc, sm, totalBalance, root2, zeroEquivScorer)
	// parent_support in empty slots 2-3 = 20,000
	// adversarial in slots 2-3 = EstimateWeight(320k, 2, 3) * 25/100 = 20,000 * 25/100 = 5,000
	// discount = 20,000 - 5,000 = 15,000
	require.Equal(t, uint64(15000), discount)

	// Threshold with discount should be lower than without
	thresholdWithDiscount := ComputeSafetyThreshold(fc, sm, totalBalance, root2, currentSlot, zeroEquivScorer)

	// Compare: no empty slot support → higher threshold
	sm2 := NewSupportMap()
	addTestSupport(sm2, 4, root2, 10_000)
	rebuildTotalSupportFromSlotRoot(sm2, fc)
	thresholdWithout := ComputeSafetyThreshold(fc, sm2, totalBalance, root2, currentSlot, zeroEquivScorer)

	require.Equal(t, true, thresholdWithDiscount < thresholdWithout)
}

// TestIsOneConfirmed_OptimisticBlock tests that optimistic blocks are never confirmed.
func TestIsOneConfirmed_OptimisticBlock(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	root2 := [32]byte{2}
	currentSlot := primitives.Slot(3)
	totalBalance := uint64(320_000)

	fc := &safetyTestFC{
		testForkchoiceReader: testForkchoiceReader{
			parents: map[[32]byte][32]byte{
				root1: root0,
				root2: root1,
			},
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: 0,
			root1: 1,
			root2: 2,
		},
		optimistic: map[[32]byte]bool{root2: true}, // root2 is optimistic
	}

	// High support — would be confirmed if not optimistic
	sm := NewSupportMap()
	addTestSupport(sm, 1, root2, 10_000)
	addTestSupport(sm, 2, root2, 10_000)
	rebuildTotalSupportFromSlotRoot(sm, fc)

	require.Equal(t, false, IsOneConfirmed(fc, sm, totalBalance, root2, currentSlot, zeroEquivScorer))
}

// TestIsOneConfirmed_OptimisticBlock tests that currentSlot == 0 does not
// underflow the unsigned slot arithmetic and returns 0 cleanly.
func TestGetAdversarialWeight_CurrentSlotZero(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	totalBalance := uint64(320_000)

	fc := &safetyTestFC{
		testForkchoiceReader: testForkchoiceReader{
			parents: map[[32]byte][32]byte{
				root1: root0,
			},
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: 0,
			root1: 0,
		},
		optimistic: map[[32]byte]bool{},
	}

	w := GetAdversarialWeight(fc, totalBalance, root1, 0, zeroEquivScorer)
	require.Equal(t, uint64(0), w)
}

// TestComputeSafetyThreshold_CurrentSlotZero tests that ComputeSafetyThreshold
// returns 0 when currentSlot is 0, avoiding unsigned underflow on currentSlot-1.
func TestComputeSafetyThreshold_CurrentSlotZero(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	totalBalance := uint64(320_000)

	fc := &safetyTestFC{
		testForkchoiceReader: testForkchoiceReader{
			parents: map[[32]byte][32]byte{
				root1: root0,
			},
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: 0,
			root1: 0,
		},
		optimistic: map[[32]byte]bool{},
	}

	sm := NewSupportMap()
	threshold := ComputeSafetyThreshold(fc, sm, totalBalance, root1, 0, zeroEquivScorer)
	require.Equal(t, uint64(0), threshold)
}

// TestIsOneConfirmed_CurrentSlotZero tests that IsOneConfirmed does not panic
// or produce incorrect results when currentSlot is 0.
func TestIsOneConfirmed_CurrentSlotZero(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	totalBalance := uint64(320_000)

	fc := &safetyTestFC{
		testForkchoiceReader: testForkchoiceReader{
			parents: map[[32]byte][32]byte{
				root1: root0,
			},
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: 0,
			root1: 0,
		},
		optimistic: map[[32]byte]bool{},
	}

	sm := NewSupportMap()
	addTestSupport(sm, 0, root1, 300_000)
	rebuildTotalSupportFromSlotRoot(sm, fc)

	// threshold is 0, score is 300k → confirmed (no underflow panic)
	require.Equal(t, true, IsOneConfirmed(fc, sm, totalBalance, root1, 0, zeroEquivScorer))
}

// TestGetAdversarialWeight_EpochCrossing tests that adversarial weight uses
// epoch start when the block crosses an epoch boundary from its parent.
func TestGetAdversarialWeight_EpochCrossing(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	root0 := [32]byte{0x10}
	root1 := [32]byte{1} // last slot of epoch 0
	root2 := [32]byte{2} // first slot of epoch 1 — crosses boundary
	currentSlot := spe + 2
	totalBalance := uint64(320_000)

	fc := &safetyTestFC{
		testForkchoiceReader: testForkchoiceReader{
			parents: map[[32]byte][32]byte{
				root1: root0,
				root2: root1,
			},
		},
		slotsByRoot: map[[32]byte]primitives.Slot{
			root0: 0,
			root1: spe - 1, // slot 31
			root2: spe,     // slot 32 — epoch 1 start
		},
		optimistic: map[[32]byte]bool{},
	}

	w := GetAdversarialWeight(fc, totalBalance, root2, currentSlot, zeroEquivScorer)

	// Block crosses epoch boundary, so adversarial range starts at epoch 1 start (slot 32)
	// Range: [32, currentSlot-1=33], 2 slots
	// estimate = 10,000 * 2 = 20,000
	// adversarial = 20,000 * 25 / 100 = 5,000
	require.Equal(t, uint64(5000), w)

	// Compare: a block that does NOT cross epoch boundary
	root3 := [32]byte{3}
	fc.parents[root3] = root2
	fc.slotsByRoot[root3] = spe + 1 // slot 33, same epoch as parent (32)

	w2 := GetAdversarialWeight(fc, totalBalance, root3, currentSlot, zeroEquivScorer)
	// Range starts at block slot (33), so [33, 33], 1 slot
	// adversarial = 10,000 * 25 / 100 = 2,500
	require.Equal(t, uint64(2500), w2)
}
