package hdiff

import (
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
)

// legacyBalanceDiff preserves the previous allocation pattern and encoding as a reference.
func legacyBalanceDiff(source, target state.ReadOnlyBeaconState) ([]byte, error) {
	sBalances, tBalances := source.Balances(), target.Balances()
	if len(tBalances) < len(sBalances) {
		return nil, errors.New("target balances shorter than source")
	}
	diffs := make([]int64, len(tBalances))
	for i, s := range sBalances {
		if tBalances[i] >= s {
			diffs[i] = int64(tBalances[i] - s)
		} else {
			diffs[i] = -int64(s - tBalances[i])
		}
	}
	for i, t := range tBalances[len(sBalances):] {
		diffs[i+len(sBalances)] = int64(t) // lint:ignore uintcast
	}
	data := make([]byte, 0, 8+len(diffs)*8)
	data = binary.LittleEndian.AppendUint64(data, uint64(len(diffs)))
	for _, d := range diffs {
		data = binary.LittleEndian.AppendUint64(data, uint64(d))
	}
	return snappy.Encode(nil, data), nil
}

type balanceReadTrackingState struct {
	state.ReadOnlyBeaconState
	count int
	reads []primitives.ValidatorIndex
	err   error
}

func (s *balanceReadTrackingState) Balances() []uint64 {
	panic("balance diff must not materialize the balances slice")
}

func (s *balanceReadTrackingState) BalancesLength() int { return s.count }

func (s *balanceReadTrackingState) BalanceAtIndex(i primitives.ValidatorIndex) (uint64, error) {
	s.reads = append(s.reads, i)
	if s.err != nil {
		return 0, s.err
	}
	return s.ReadOnlyBeaconState.BalanceAtIndex(i)
}

type balanceBatchTrackingState struct {
	*balanceReadTrackingState
	starts []primitives.ValidatorIndex
	sizes  []int
	failAt primitives.ValidatorIndex
}

func (s *balanceBatchTrackingState) BalanceAtIndex(primitives.ValidatorIndex) (uint64, error) {
	panic("native balance diff must use batch reads")
}

func (s *balanceBatchTrackingState) ReadBalancesAt(start primitives.ValidatorIndex, dst []uint64) error {
	s.starts = append(s.starts, start)
	s.sizes = append(s.sizes, len(dst))
	if s.err != nil && start == s.failAt {
		return s.err
	}
	return s.ReadOnlyBeaconState.(balanceRangeReader).ReadBalancesAt(start, dst)
}

func balanceState(tb testing.TB, balances []uint64) state.BeaconState {
	tb.Helper()
	s, err := state_native.InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{Balances: slices.Clone(balances)})
	require.NoError(tb, err)
	return s
}

func TestBalanceDiffEncoding(t *testing.T) {
	for _, tt := range []struct {
		name   string
		source []uint64
		target []uint64
	}{
		{name: "empty"},
		{name: "unchanged", source: []uint64{0, 32e9, math.MaxUint64}, target: []uint64{0, 32e9, math.MaxUint64}},
		{name: "increases and decreases", source: []uint64{32e9, 32e9, 32e9}, target: []uint64{32e9 + 1000, 32e9 - 500, 0}},
		{name: "appended balances", source: []uint64{32e9}, target: []uint64{32e9 - 1, 0, 32e9, math.MaxUint64}},
		{name: "empty source", target: []uint64{0, 32e9, math.MaxInt64, 1 << 63, math.MaxUint64}},
		{
			name:   "integer boundaries",
			source: []uint64{0, 0, 0, math.MaxUint64, 1 << 63, math.MaxInt64, math.MaxUint64},
			target: []uint64{math.MaxInt64, 1 << 63, math.MaxUint64, 0, 0, math.MaxUint64, math.MaxInt64},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source, target := balanceState(t, tt.source), balanceState(t, tt.target)
			s := &balanceReadTrackingState{ReadOnlyBeaconState: source, count: len(tt.source)}
			d := &balanceReadTrackingState{ReadOnlyBeaconState: target, count: len(tt.target)}
			got, err := diffToBalances(s, d)
			require.NoError(t, err)
			want, err := legacyBalanceDiff(source, target)
			require.NoError(t, err)
			require.DeepEqual(t, want, got)
			require.Equal(t, len(tt.source), len(s.reads))
			require.Equal(t, len(tt.target), len(d.reads))
			for i, index := range s.reads {
				require.Equal(t, primitives.ValidatorIndex(i), index)
			}
			for i, index := range d.reads {
				require.Equal(t, primitives.ValidatorIndex(i), index)
			}
			raw, err := snappy.Decode(nil, got)
			require.NoError(t, err)
			require.Equal(t, 8+8*len(tt.target), len(raw))
			require.Equal(t, uint64(len(tt.target)), binary.LittleEndian.Uint64(raw))
			diffs, err := newBalancesDiff(got)
			require.NoError(t, err)
			patched, err := applyBalancesDiff(source.Copy(), diffs)
			require.NoError(t, err)
			require.DeepEqual(t, target.Balances(), patched.Balances())
			require.Equal(t, true, slices.Equal(tt.source, source.Balances()))
			require.Equal(t, true, slices.Equal(tt.target, target.Balances()))
		})
	}
	t.Run("same state", func(t *testing.T) {
		source := balanceState(t, []uint64{0, 32e9, math.MaxUint64})
		got, err := diffToBalances(source, source)
		require.NoError(t, err)
		diffs, err := newBalancesDiff(got)
		require.NoError(t, err)
		require.DeepEqual(t, []int64{0, 0, 0}, diffs)
	})
}

func TestBalanceDiffErrors(t *testing.T) {
	readErr := errors.New("balance read failed")
	for _, tt := range []struct {
		name      string
		source    int
		target    int
		sourceErr error
		targetErr error
		want      string
		wantReads int
	}{
		{name: "shorter target", source: 2, target: 1, want: "invalid balances lengths"},
		{name: "negative source", source: -1, target: 0, want: "invalid balances lengths"},
		{name: "negative target", source: 0, target: -1, want: "invalid balances lengths"},
		{name: "overflow", target: (math.MaxInt-8)/8 + 1, want: "overflows int"},
		{name: "source error", source: 1, target: 1, sourceErr: readErr, want: "source balance at index 0", wantReads: 2},
		{name: "target error", source: 1, target: 1, targetErr: readErr, want: "target balance at index 0", wantReads: 1},
		{name: "appended target error", target: 1, targetErr: readErr, want: "target balance at index 0", wantReads: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := balanceState(t, []uint64{1})
			source := &balanceReadTrackingState{ReadOnlyBeaconState: base, count: tt.source, err: tt.sourceErr}
			target := &balanceReadTrackingState{ReadOnlyBeaconState: base, count: tt.target, err: tt.targetErr}
			got, err := diffToBalances(source, target)
			require.ErrorContains(t, tt.want, err)
			require.IsNil(t, got)
			require.Equal(t, tt.wantReads, len(source.reads)+len(target.reads))
			if tt.sourceErr != nil || tt.targetErr != nil {
				require.ErrorIs(t, err, readErr)
			}
		})
	}
}

func TestBalanceDiffBatches(t *testing.T) {
	for _, targetCount := range []int{0, 1, balanceDiffBatchSize - 1, balanceDiffBatchSize, balanceDiffBatchSize + 1, 3*balanceDiffBatchSize + 17} {
		for _, sourceCount := range []int{0, 1, balanceDiffBatchSize - 1, balanceDiffBatchSize, balanceDiffBatchSize + 1, targetCount} {
			if sourceCount > targetCount {
				continue
			}
			t.Run(fmt.Sprintf("source=%d/target=%d", sourceCount, targetCount), func(t *testing.T) {
				balances := make([]uint64, targetCount)
				for i := range balances {
					balances[i] = uint64(i+1) * 32e9
				}
				source, target := balanceState(t, balances[:sourceCount]), balanceState(t, balances)
				s := &balanceBatchTrackingState{balanceReadTrackingState: &balanceReadTrackingState{ReadOnlyBeaconState: source, count: sourceCount}}
				d := &balanceBatchTrackingState{balanceReadTrackingState: &balanceReadTrackingState{ReadOnlyBeaconState: target, count: targetCount}}
				got, err := diffToBalances(s, d)
				require.NoError(t, err)
				want, err := legacyBalanceDiff(source, target)
				require.NoError(t, err)
				require.DeepEqual(t, want, got)
				for _, reader := range []*balanceBatchTrackingState{s, d} {
					require.Equal(t, (reader.count+balanceDiffBatchSize-1)/balanceDiffBatchSize, len(reader.starts))
					for i, start := range reader.starts {
						require.Equal(t, primitives.ValidatorIndex(i*balanceDiffBatchSize), start)
						require.Equal(t, min(reader.count-i*balanceDiffBatchSize, balanceDiffBatchSize), reader.sizes[i])
					}
				}
			})
		}
	}
	t.Run("later batch read failures", func(t *testing.T) {
		for _, failSource := range []bool{false, true} {
			t.Run(fmt.Sprintf("source=%t", failSource), func(t *testing.T) {
				base := balanceState(t, make([]uint64, balanceDiffBatchSize+1))
				s := &balanceBatchTrackingState{balanceReadTrackingState: &balanceReadTrackingState{ReadOnlyBeaconState: base, count: base.BalancesLength()}}
				d := &balanceBatchTrackingState{balanceReadTrackingState: &balanceReadTrackingState{ReadOnlyBeaconState: base, count: base.BalancesLength()}}
				failing := d
				if failSource {
					failing = s
				}
				failing.err = errors.New("batch read failed")
				failing.failAt = balanceDiffBatchSize
				got, err := diffToBalances(s, d)
				require.ErrorIs(t, err, failing.err)
				require.ErrorContains(t, fmt.Sprintf("index %d", balanceDiffBatchSize), err)
				require.IsNil(t, got)
			})
		}
	})
}

func TestDiffStreamsBalances(t *testing.T) {
	source, _ := util.DeterministicGenesisStateElectra(t, 8)
	target := source.Copy()
	require.NoError(t, target.SetSlot(source.Slot()+1))
	require.NoError(t, target.UpdateBalancesAtIndex(0, 32e9+123))
	require.NoError(t, target.UpdateBalancesAtIndex(1, 32e9-456))
	sourceBefore, err := source.MarshalSSZ()
	require.NoError(t, err)
	targetBefore, err := target.MarshalSSZ()
	require.NoError(t, err)
	s := &balanceReadTrackingState{ReadOnlyBeaconState: source, count: source.BalancesLength()}
	d := &balanceReadTrackingState{ReadOnlyBeaconState: target, count: target.BalancesLength()}
	diff, err := Diff(s, d)
	require.NoError(t, err)
	want, err := legacyBalanceDiff(source, target)
	require.NoError(t, err)
	require.DeepEqual(t, want, diff.BalancesDiff)
	decoded, err := newHdiff(diff)
	require.NoError(t, err)
	reencoded, err := decoded.serialize()
	require.NoError(t, err)
	require.DeepEqual(t, diff, reencoded)
	result, err := ApplyDiff(t.Context(), source.Copy(), diff)
	require.NoError(t, err)
	resultBytes, err := result.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, targetBefore, resultBytes)
	sourceAfter, err := source.MarshalSSZ()
	require.NoError(t, err)
	targetAfter, err := target.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, sourceBefore, sourceAfter)
	require.DeepEqual(t, targetBefore, targetAfter)

	t.Run("read error propagates", func(t *testing.T) {
		readErr := errors.New("balance read failed")
		d.err = readErr
		got, err := Diff(s, d)
		require.ErrorIs(t, err, readErr)
		require.DeepEqual(t, HdiffBytes{}, got)
	})
}

var balanceDiffSink []byte

func BenchmarkBalanceDiffEncoding(b *testing.B) {
	for _, size := range []int{10_000, 1_000_000} {
		b.Run(fmt.Sprintf("balances=%d", size), func(b *testing.B) {
			balances := make([]uint64, size)
			for i := range balances {
				balances[i] = 32e9
			}
			source := balanceState(b, balances)
			target := source.Copy()
			for i := range balances {
				balances[i] += uint64(i % 1000)
				require.NoError(b, target.UpdateBalancesAtIndex(primitives.ValidatorIndex(i), balances[i]))
			}
			flatTarget := balanceState(b, balances)
			for i := range balances {
				balances[i] = 32e9
			}
			flatSource := balanceState(b, balances)
			for _, layout := range []struct {
				name   string
				source state.ReadOnlyBeaconState
				target state.ReadOnlyBeaconState
			}{
				{name: "shared_modified", source: source, target: target},
				{name: "independent", source: flatSource, target: flatTarget},
				{name: "unchanged", source: flatSource, target: flatSource},
			} {
				b.Run(layout.name, func(b *testing.B) {
					benchmarkBalanceDiffPair(b, layout.source, layout.target)
				})
			}
		})
	}
}

func benchmarkBalanceDiffPair(b *testing.B, source, target state.ReadOnlyBeaconState) {
	for _, impl := range []struct {
		name string
		diff func(state.ReadOnlyBeaconState, state.ReadOnlyBeaconState) ([]byte, error)
	}{
		{name: "legacy", diff: legacyBalanceDiff},
		{name: "streaming", diff: diffToBalances},
	} {
		b.Run(impl.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				balanceDiffSink, err = impl.diff(source, target)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
