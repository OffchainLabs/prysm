package cache

import (
	"sync"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

var testDependentRoot = [32]byte{'r'}

func testInclusionList(t testing.TB, slot primitives.Slot, idx primitives.ValidatorIndex, txs ...string) *ethpb.SignedInclusionList {
	raw := make([][]byte, len(txs))
	for i, tx := range txs {
		raw[i] = []byte(tx)
	}
	list, err := enginev1.NewProgressiveTransactionList(raw)
	require.NoError(t, err)
	return &ethpb.SignedInclusionList{
		Message:   &ethpb.InclusionList{Slot: slot, ValidatorIndex: idx, DependentRoot: testDependentRoot[:], Transactions: list},
		Signature: make([]byte, 96),
	}
}

func txStrings(txs [][]byte) []string {
	out := make([]string, len(txs))
	for i, tx := range txs {
		out[i] = string(tx)
	}
	return out
}

func TestInclusionListStore_Process(t *testing.T) {
	t.Run("identical duplicate is ignored", func(t *testing.T) {
		s := NewInclusionListStore()
		require.Equal(t, true, s.Process(testInclusionList(t, 1, 7, "a"), true))
		// A new object with the same content and another signature is the same message.
		dup := testInclusionList(t, 1, 7, "a")
		dup.Signature[0] = 1
		require.Equal(t, false, s.Process(dup, false))
		require.Equal(t, false, s.IsEquivocator(1, testDependentRoot, 7))
		_, timely, ok := s.Get(1, testDependentRoot, 7)
		require.Equal(t, true, ok)
		require.Equal(t, true, timely)
	})
	t.Run("different list marks the equivocator and keeps the first", func(t *testing.T) {
		s := NewInclusionListStore()
		first := testInclusionList(t, 1, 7, "a")
		require.Equal(t, true, s.Process(first, false))
		require.Equal(t, false, s.Process(testInclusionList(t, 1, 7, "b"), true))
		require.Equal(t, true, s.IsEquivocator(1, testDependentRoot, 7))
		got, timely, ok := s.Get(1, testDependentRoot, 7)
		require.Equal(t, true, ok)
		require.Equal(t, false, timely)
		require.Equal(t, first, got)
	})
	t.Run("other dependent root is another key", func(t *testing.T) {
		s := NewInclusionListStore()
		require.Equal(t, true, s.Process(testInclusionList(t, 1, 7, "a"), true))
		other := testInclusionList(t, 1, 7, "b")
		other.Message.DependentRoot = make([]byte, 32)
		require.Equal(t, true, s.Process(other, true))
		require.Equal(t, false, s.IsEquivocator(1, testDependentRoot, 7))
		require.Equal(t, false, s.IsEquivocator(1, [32]byte{}, 7))
	})
	t.Run("nil", func(t *testing.T) {
		s := NewInclusionListStore()
		require.Equal(t, false, s.Process(nil, true))
		require.Equal(t, false, s.Process(&ethpb.SignedInclusionList{}, true))
	})
	t.Run("missing entry", func(t *testing.T) {
		got, timely, ok := NewInclusionListStore().Get(1, testDependentRoot, 7)
		require.Equal(t, false, ok)
		require.Equal(t, false, timely)
		require.Equal(t, (*ethpb.SignedInclusionList)(nil), got)
	})
}

func TestInclusionListStore_Transactions(t *testing.T) {
	s := NewInclusionListStore()
	s.Process(testInclusionList(t, 1, 3, "c", "a"), true)
	s.Process(testInclusionList(t, 1, 1, "a", "b", "a"), true)
	s.Process(testInclusionList(t, 1, 2, "late"), false)
	s.Process(testInclusionList(t, 1, 4, "equivocated"), true)
	s.Process(testInclusionList(t, 1, 4, "other"), true)
	s.Process(testInclusionList(t, 2, 5, "next slot"), true)

	tests := []struct {
		name       string
		onlyTimely bool
		want       []string
	}{
		{name: "only timely", onlyTimely: true, want: []string{"a", "b", "c"}},
		{name: "all", onlyTimely: false, want: []string{"a", "b", "late", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.DeepEqual(t, tt.want, txStrings(s.Transactions(1, testDependentRoot, tt.onlyTimely)))
		})
	}
	t.Run("unknown key", func(t *testing.T) {
		require.Equal(t, 0, len(s.Transactions(1, [32]byte{}, false)))
	})
}

func TestInclusionListStore_Bits(t *testing.T) {
	s := NewInclusionListStore()
	s.Process(testInclusionList(t, 1, 10, "a"), true)
	s.Process(testInclusionList(t, 1, 30, "b"), false)
	s.Process(testInclusionList(t, 1, 40, "c"), true)
	s.Process(testInclusionList(t, 1, 40, "d"), true)
	// Validator 10 is at two positions, as in a committee cycled over few validators.
	committee := []primitives.ValidatorIndex{20, 10, 30, 40, 50, 10, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150}

	bits := func(positions ...uint64) bitfield.Bitvector16 {
		b := bitfield.NewBitvector16()
		for _, p := range positions {
			b.SetBitAt(p, true)
		}
		return b
	}
	t.Run("only timely", func(t *testing.T) {
		require.DeepEqual(t, bits(1, 5), s.Bits(committee, 1, testDependentRoot, true))
	})
	t.Run("all", func(t *testing.T) {
		require.DeepEqual(t, bits(1, 2, 5), s.Bits(committee, 1, testDependentRoot, false))
	})

	inclusive := []struct {
		name   string
		remote bitfield.Bitvector16
		want   bool
	}{
		{name: "equal", remote: bits(1, 5), want: true},
		{name: "remote has extra bits", remote: bits(0, 1, 5, 15), want: true},
		{name: "local bit unset in remote", remote: bits(1), want: false},
		{name: "remote empty", remote: bits(), want: false},
	}
	for _, tt := range inclusive {
		t.Run("inclusive "+tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, s.IsBitsInclusive(committee, 1, testDependentRoot, tt.remote, true))
		})
	}
	t.Run("inclusive with no local lists", func(t *testing.T) {
		require.Equal(t, true, s.IsBitsInclusive(committee, 9, testDependentRoot, bits(), true))
	})
}

func TestInclusionListStore_Prune(t *testing.T) {
	retain := primitives.Slot(params.BeaconConfig().MinSlotsForInclusionListsRequests)
	s := NewInclusionListStore()
	for slot := primitives.Slot(10); slot <= 12; slot++ {
		s.Process(testInclusionList(t, slot, 1, "a"), true)
		s.Process(testInclusionList(t, slot, 1, "b"), true)
	}
	s.Prune(11 + retain)

	_, _, ok := s.Get(10, testDependentRoot, 1)
	require.Equal(t, false, ok)
	require.Equal(t, false, s.IsEquivocator(10, testDependentRoot, 1))
	for _, slot := range []primitives.Slot{11, 12} {
		_, _, ok := s.Get(slot, testDependentRoot, 1)
		require.Equal(t, true, ok)
		require.Equal(t, true, s.IsEquivocator(slot, testDependentRoot, 1))
	}
	require.Equal(t, 2, len(s.lists))
	require.Equal(t, 2, len(s.equivocators))
}

func TestInclusionListStore_Concurrent(t *testing.T) {
	s := NewInclusionListStore()
	committee := make([]primitives.ValidatorIndex, 16)
	for i := range committee {
		committee[i] = primitives.ValidatorIndex(i)
	}
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			s.Process(testInclusionList(t, 1, primitives.ValidatorIndex(i), "a", string(rune('b'+i))), true)
		})
		wg.Go(func() {
			s.Transactions(1, testDependentRoot, true)
			s.IsBitsInclusive(committee, 1, testDependentRoot, bitfield.NewBitvector16(), true)
			s.Prune(0)
		})
	}
	wg.Wait()
	require.Equal(t, 17, len(s.Transactions(1, testDependentRoot, true)))
	require.Equal(t, uint64(16), s.Bits(committee, 1, testDependentRoot, true).Count())
}
