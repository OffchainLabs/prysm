package confirmation

import (
	"context"
	"testing"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// testForkchoiceReader extends the mock to support parent chain walking for TotalSupport.
type testForkchoiceReader struct {
	mockForkchoiceReader
	parents map[[32]byte][32]byte // root → parent root
}

func (m *testForkchoiceReader) ParentRoot(root [32]byte) ([32]byte, error) {
	p, ok := m.parents[root]
	if !ok {
		return [32]byte{}, nil
	}
	return p, nil
}

// addTestSupport registers a fresh synthetic validator voting for root in the given slot.
func addTestSupport(sm *SupportMap, slot primitives.Slot, root [32]byte, balance uint64) {
	idx := primitives.ValidatorIndex(len(sm.balances))
	sm.balances = append(sm.balances, balance)
	if sm.slotRootVoters[slot] == nil {
		sm.slotRootVoters[slot] = make(map[[32]byte][]primitives.ValidatorIndex)
	}
	sm.slotRootVoters[slot][root] = append(sm.slotRootVoters[slot][root], idx)
}

// rebuildTotalSupportFromSlotRoot rebuilds totalSupport from the registered voters by walking the ancestor chain.
func rebuildTotalSupportFromSlotRoot(sm *SupportMap, fc ForkchoiceReader) {
	sm.totalSupport = make(map[[32]byte]uint64)
	globalVotes := make(map[[32]byte]uint64)
	for _, rootMap := range sm.slotRootVoters {
		for root, voters := range rootMap {
			for _, idx := range voters {
				globalVotes[root] += sm.balances[idx]
			}
		}
	}
	for root, bal := range globalVotes {
		if bal == 0 {
			continue
		}
		r := root
		for {
			sm.totalSupport[r] += bal
			parent, err := fc.ParentRoot(r)
			if err != nil || parent == ([32]byte{}) {
				break
			}
			r = parent
		}
	}
}

// testCommitteeAccessor returns fixed committee assignments.
type testCommitteeAccessor struct {
	committees map[primitives.Slot][]primitives.ValidatorIndex
}

func (m *testCommitteeAccessor) Committee(_ context.Context, slot primitives.Slot) ([]primitives.ValidatorIndex, error) {
	return m.committees[slot], nil
}

// TestSupportMap_Build tests the full build path including ancestor-based TotalSupport.
//
// Tree: root0 → root1 → root2 → root3
// Slot:   0       1       2       3
//
// Committees:
//
//	slot 1: validators [0, 1]
//	slot 2: validators [2, 3]
//	slot 3: validator  [4]
//
// Votes:
//
//	validator 0: votes for root1 (slot 1)
//	validator 1: votes for root2 (slot 2)
//	validator 2: votes for root3 (slot 3)
//	validator 3: votes for root1 (slot 1)
//	validator 4: votes for root3 (slot 3)
//
// Balances: [100, 200, 300, 400, 500]
func TestSupportMap_Build(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{1}
	root2 := [32]byte{2}
	root3 := [32]byte{3}

	fc := &testForkchoiceReader{
		parents: map[[32]byte][32]byte{
			root1: root0,
			root2: root1,
			root3: root2,
		},
	}
	committees := &testCommitteeAccessor{
		committees: map[primitives.Slot][]primitives.ValidatorIndex{
			1: {0, 1},
			2: {2, 3},
			3: {4},
		},
	}
	votes := []forkchoicetypes.VoteData{
		{Root: root1, Slot: 1},
		{Root: root2, Slot: 2},
		{Root: root3, Slot: 3},
		{Root: root1, Slot: 1},
		{Root: root3, Slot: 3},
	}
	balances := []uint64{100, 200, 300, 400, 500}

	sm := NewSupportMap()
	err := sm.Build(context.Background(), votes, balances, committees, nil, 4, fc)
	require.NoError(t, err)

	// --- BlockSupportBetweenSlots (direct votes) ---
	// Slot 1: validator 0 (100) votes root1, validator 1 (200) votes root2
	require.Equal(t, uint64(100), sm.BlockSupportBetweenSlots(root1, 1, 1))
	require.Equal(t, uint64(200), sm.BlockSupportBetweenSlots(root2, 1, 1))

	// Slot 2: validator 2 (300) votes root3, validator 3 (400) votes root1
	require.Equal(t, uint64(400), sm.BlockSupportBetweenSlots(root1, 2, 2))
	require.Equal(t, uint64(300), sm.BlockSupportBetweenSlots(root3, 2, 2))

	// Slot 3: validator 4 (500) votes root3
	require.Equal(t, uint64(500), sm.BlockSupportBetweenSlots(root3, 3, 3))

	// Range: slots 1-2 for root1 = 100 + 400 = 500
	require.Equal(t, uint64(500), sm.BlockSupportBetweenSlots(root1, 1, 2))

	// Range: slots 2-3 for root3 = 300 + 500 = 800
	require.Equal(t, uint64(800), sm.BlockSupportBetweenSlots(root3, 2, 3))

	// No support for root0 in any slot
	require.Equal(t, uint64(0), sm.BlockSupportBetweenSlots(root0, 1, 3))

	// --- AttestationScore (ancestor-based LMD) ---
	// Votes: root1 gets 100+400=500, root2 gets 200, root3 gets 300+500=800
	// Ancestor credit:
	//   root3 (800) credits root2, root1, root0
	//   root2 (200) credits root1, root0
	//   root1 (500) credits root0
	// TotalSupport:
	//   root3 = 800
	//   root2 = 200 + 800 = 1000
	//   root1 = 500 + 200 + 800 = 1500
	//   root0 = 500 + 200 + 800 = 1500
	require.Equal(t, uint64(800), sm.AttestationScore(root3))
	require.Equal(t, uint64(1000), sm.AttestationScore(root2))
	require.Equal(t, uint64(1500), sm.AttestationScore(root1))
	require.Equal(t, uint64(1500), sm.AttestationScore(root0))
}

// TestSupportMap_Equivocation tests that equivocating validators are excluded
// from the support maps.
func TestSupportMap_Equivocation(t *testing.T) {
	root1 := [32]byte{1}

	fc := &testForkchoiceReader{
		parents: map[[32]byte][32]byte{},
	}
	committees := &testCommitteeAccessor{
		committees: map[primitives.Slot][]primitives.ValidatorIndex{
			1: {0, 1, 2},
		},
	}
	votes := []forkchoicetypes.VoteData{
		{Root: root1, Slot: 1},
		{Root: root1, Slot: 1},
		{Root: root1, Slot: 1},
	}
	balances := []uint64{100, 200, 300}
	equivocating := map[primitives.ValidatorIndex]bool{
		1: true, // validator 1 is equivocating
	}

	sm := NewSupportMap()
	err := sm.Build(context.Background(), votes, balances, committees, equivocating, 2, fc)
	require.NoError(t, err)

	// Validator 1 (200) is equivocating: excluded from support
	require.Equal(t, uint64(400), sm.BlockSupportBetweenSlots(root1, 1, 1)) // 100 + 300
	require.Equal(t, uint64(400), sm.AttestationScore(root1))
}

// TestSupportMap_EmptySlots tests that querying slots with no committees returns 0.
func TestSupportMap_EmptySlots(t *testing.T) {
	sm := NewSupportMap()
	require.Equal(t, uint64(0), sm.BlockSupportBetweenSlots([32]byte{1}, 5, 10))
	require.Equal(t, uint64(0), sm.AttestationScore([32]byte{1}))
}
