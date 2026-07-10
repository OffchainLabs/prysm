package blockchain

import (
	"runtime"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestGoroutineCounter(t *testing.T) {
	t.Run("average is zero on a fresh counter", func(t *testing.T) {
		g := &goroutineCounter{}
		require.Equal(t, 0, g.average())
	})

	t.Run("average returns the cached value", func(t *testing.T) {
		g := &goroutineCounter{}
		g.avg = 42
		require.Equal(t, 42, g.average())
	})

	t.Run("sample updates the cached average to the mean of the window", func(t *testing.T) {
		g := &goroutineCounter{}
		// Pre-populate the ring buffer with known values across all 10 slots;
		// the next sample will overwrite one and recompute the average.
		for i := range goroutineSampleWindow {
			g.samples[i] = 100
		}
		idx := primitives.Slot(3)
		g.sample(idx)

		// All but one slot hold 100; that one slot holds runtime.NumGoroutine().
		nowCount := g.samples[uint64(idx)%goroutineSampleWindow]
		expected := (100*(goroutineSampleWindow-1) + nowCount) / goroutineSampleWindow
		require.Equal(t, expected, g.avg)
		require.Equal(t, expected, g.average())
	})

	t.Run("average is biased low before the window is full", func(t *testing.T) {
		g := &goroutineCounter{}
		g.sample(primitives.Slot(0))
		// 9 slots still zero, so the average is strictly below the single sampled value.
		require.Equal(t, true, g.avg < g.samples[0])
	})

	t.Run("sample writes at slot modulo window", func(t *testing.T) {
		g := &goroutineCounter{}
		slot := primitives.Slot(7)

		before := runtime.NumGoroutine()
		g.sample(slot)
		after := runtime.NumGoroutine()

		idx := uint64(slot) % goroutineSampleWindow
		got := g.samples[idx]
		require.Equal(t, true, got >= before-1 && got <= after+1)

		for i, v := range g.samples {
			if uint64(i) == idx {
				continue
			}
			require.Equal(t, 0, v)
		}
	})

	t.Run("sample overwrites the same index after a full window", func(t *testing.T) {
		g := &goroutineCounter{}
		g.sample(primitives.Slot(2))
		require.Equal(t, true, g.samples[2] > 0)

		g.samples[2] = 1 // poison the slot to detect overwrite
		g.sample(primitives.Slot(2 + goroutineSampleWindow))
		require.Equal(t, false, g.samples[2] == 1)
	})
}
