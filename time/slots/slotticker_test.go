package slots

import (
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/stretchr/testify/require"
)

var _ Ticker = (*SlotTicker)(nil)

// settledGoroutines waits until every other goroutine in the current bubble has
// reached a blocking operation and then reports the live goroutine count. This
// makes a leak assertion deterministic: a ticker parked on a channel send is
// settled, so it is counted, while a ticker that returned from its goroutine is
// not.
func settledGoroutines() int {
	synctest.Wait()
	return runtime.NumGoroutine()
}

// tickerTestFixtures returns the injectable clock functions used by the leak
// tests below: a ticker that only fires when the test hands it a tick.
func tickerTestFixtures(tick <-chan time.Time) (time.Time, time.Duration, func(time.Time) time.Duration, func(time.Time) time.Duration, func(time.Duration) <-chan time.Time) {
	genesisTime := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	slotDuration := 8 * time.Second
	since := func(time.Time) time.Duration { return 1 * time.Second }
	until := func(time.Time) time.Duration { return 7 * time.Second }
	after := func(time.Duration) <-chan time.Time { return tick }
	return genesisTime, slotDuration, since, until, after
}

func TestSlotTicker(t *testing.T) {
	ticker := &SlotTicker{
		c:    make(chan primitives.Slot),
		done: make(chan struct{}),
	}
	defer ticker.Done()

	var sinceDuration time.Duration
	since := func(time.Time) time.Duration {
		return sinceDuration
	}

	var untilDuration time.Duration
	until := func(time.Time) time.Duration {
		return untilDuration
	}

	var tick chan time.Time
	after := func(time.Duration) <-chan time.Time {
		return tick
	}

	genesisTime := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	slotDuration := 8 * time.Second

	// Test when the ticker starts immediately after genesis time.
	sinceDuration = 1 * time.Second
	untilDuration = 7 * time.Second
	// Make this a buffered channel to prevent a deadlock since
	// the other goroutine calls a function in this goroutine.
	tick = make(chan time.Time, 2)
	ticker.start(genesisTime, slotDuration, since, until, after)

	// Tick once.
	tick <- time.Now()
	slot := <-ticker.C()
	if slot != 0 {
		t.Fatalf("Expected %d, got %d", 0, slot)
	}

	// Tick twice.
	tick <- time.Now()
	slot = <-ticker.C()
	if slot != 1 {
		t.Fatalf("Expected %d, got %d", 1, slot)
	}

	// Tick thrice.
	tick <- time.Now()
	slot = <-ticker.C()
	if slot != 2 {
		t.Fatalf("Expected %d, got %d", 2, slot)
	}
}

func TestSlotTickerGenesis(t *testing.T) {
	ticker := &SlotTicker{
		c:    make(chan primitives.Slot),
		done: make(chan struct{}),
	}
	defer ticker.Done()

	var sinceDuration time.Duration
	since := func(time.Time) time.Duration {
		return sinceDuration
	}

	var untilDuration time.Duration
	until := func(time.Time) time.Duration {
		return untilDuration
	}

	var tick chan time.Time
	after := func(time.Duration) <-chan time.Time {
		return tick
	}

	genesisTime := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	slotDuration := 8 * time.Second

	// Test when the ticker starts before genesis time.
	sinceDuration = -1 * time.Second
	untilDuration = 1 * time.Second
	// Make this a buffered channel to prevent a deadlock since
	// the other goroutine calls a function in this goroutine.
	tick = make(chan time.Time, 2)
	ticker.start(genesisTime, slotDuration, since, until, after)

	// Tick once.
	tick <- time.Now()
	slot := <-ticker.C()
	if slot != 0 {
		t.Fatalf("Expected %d, got %d", 0, slot)
	}

	// Tick twice.
	tick <- time.Now()
	slot = <-ticker.C()
	if slot != 1 {
		t.Fatalf("Expected %d, got %d", 1, slot)
	}
}

func TestGetSlotTickerWithOffset_OK(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		genesisTime := time.Now()
		slotDuration := 4 * time.Second
		offset := slotDuration / 2

		offsetTicker := NewSlotTickerWithOffset(genesisTime, offset, slotDuration)
		defer offsetTicker.Done()
		normalTicker := NewSlotTicker(genesisTime, slotDuration)
		defer normalTicker.Done()

		firstTicked := 0
		for {
			select {
			case <-offsetTicker.C():
				if firstTicked != 1 {
					t.Fatal("Expected other ticker to tick first")
				}
				return
			case <-normalTicker.C():
				if firstTicked != 0 {
					t.Fatal("Expected normal ticker to tick first")
				}
				firstTicked = 1
			}
		}
	})
}

func TestGetSlotTickerWitIntervals(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		genesisTime := time.Now()
		offset := params.BeaconConfig().SlotDuration() / 3
		intervals := []time.Duration{offset, 2 * offset}

		intervalTicker := NewSlotTickerWithIntervals(genesisTime, intervals)
		defer intervalTicker.Done()
		normalTicker := NewSlotTicker(genesisTime, params.BeaconConfig().SlotDuration())
		defer normalTicker.Done()

		firstTicked := 0
		for {
			select {
			case <-intervalTicker.C():
				// interval ticks starts in second slot
				if firstTicked < 2 {
					t.Fatal("Expected other ticker to tick first")
				}
				return
			case <-normalTicker.C():
				if firstTicked > 1 {
					t.Fatal("Expected normal ticker to tick first")
				}
				firstTicked++
			}
		}
	})
}

func TestSlotTickerWithIntervalsInputValidation(t *testing.T) {
	var genesisTime time.Time
	offset := params.BeaconConfig().SlotDuration() / 3
	intervals := make([]time.Duration, 0)
	panicCall := func() {
		NewSlotTickerWithIntervals(genesisTime, intervals)
	}
	require.Panics(t, panicCall, "zero genesis time")
	genesisTime = time.Now()
	require.Panics(t, panicCall, "at least one interval has to be entered")
	intervals = []time.Duration{2 * offset, offset}
	require.Panics(t, panicCall, "invalid decreasing offsets")
	intervals = []time.Duration{offset, 4 * offset}
	require.Panics(t, panicCall, "invalid ticker offset")
	intervals = []time.Duration{4 * offset, offset}
	require.Panics(t, panicCall, "invalid ticker offset")
	intervals = []time.Duration{offset, 2 * offset}
	require.NotPanics(t, panicCall)
}

// A ticker that has already fired and is handing its tick to a channel nobody
// reads from must still be stoppable. The send used to live outside the select,
// so the ticker goroutine could never observe the shutdown, and Done() spawned a
// goroutine that blocked forever trying to signal it: two leaked goroutines.
func TestSlotTickerDoneUnblocksTickerStuckOnSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tick := make(chan time.Time, 1)
		genesisTime, slotDuration, since, until, after := tickerTestFixtures(tick)
		ticker := &SlotTicker{c: make(chan primitives.Slot), done: make(chan struct{})}
		ticker.start(genesisTime, slotDuration, since, until, after)

		// Fire one tick without ever reading C(). Once the goroutine settles it is
		// parked inside the send, which is the state Done() has to recover from.
		tick <- time.Now()
		before := settledGoroutines()

		ticker.Done()
		require.LessOrEqual(t, settledGoroutines(), before, "Done() did not stop a ticker stuck in its send")
	})
}

// Same hazard for the interval ticker, which duplicates the ticker loop.
func TestSlotIntervalTickerDoneUnblocksTickerStuckOnSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tick := make(chan time.Time, 1)
		until := func(time.Time) time.Duration { return 7 * time.Second }
		after := func(time.Duration) <-chan time.Time { return tick }
		intervals := []time.Duration{time.Second, 2 * time.Second}
		ticker := &SlotIntervalTicker{c: make(chan SlotInterval), done: make(chan struct{})}
		ticker.startWithIntervals(time.Now(), until, after, intervals)

		tick <- time.Now()
		before := settledGoroutines()

		ticker.Done()
		require.LessOrEqual(t, settledGoroutines(), before, "Done() did not stop a ticker stuck in its send")
	})
}

// Done() is documented as a cleanup call and the codebase's dominant idiom is
// `defer ticker.Done()`, so a second call must not leak the goroutine the old
// implementation spawned to deliver the shutdown signal.
func TestSlotTickerDoneIsIdempotent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		genesisTime, slotDuration, since, until, after := tickerTestFixtures(make(chan time.Time))
		ticker := &SlotTicker{c: make(chan primitives.Slot), done: make(chan struct{})}
		ticker.start(genesisTime, slotDuration, since, until, after)
		before := settledGoroutines()

		ticker.Done()
		ticker.Done()
		ticker.Done()

		require.LessOrEqual(t, settledGoroutines(), before, "repeated Done() leaked goroutines")
	})
}

func TestSlotIntervalTickerDoneIsIdempotent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		until := func(time.Time) time.Duration { return 7 * time.Second }
		after := func(time.Duration) <-chan time.Time { return make(chan time.Time) }
		intervals := []time.Duration{time.Second, 2 * time.Second}
		ticker := &SlotIntervalTicker{c: make(chan SlotInterval), done: make(chan struct{})}
		ticker.startWithIntervals(time.Now(), until, after, intervals)
		before := settledGoroutines()

		ticker.Done()
		ticker.Done()
		ticker.Done()

		require.LessOrEqual(t, settledGoroutines(), before, "repeated Done() leaked goroutines")
	})
}

// A ticker that was never started has no goroutine to stop, so Done must be a
// harmless no-op rather than panicking on a nil done channel. Beacon-chain
// callers rely on this: e.g. beacon-chain/slasher's StartStop test swaps in
// zero-value tickers before calling Stop().
func TestSlotTickerDoneOnZeroValueIsNoOp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ticker := &SlotTicker{}
		before := settledGoroutines()

		require.NotPanics(t, func() {
			ticker.Done()
			ticker.Done()
		})

		require.LessOrEqual(t, settledGoroutines(), before, "Done() on a zero-value ticker leaked goroutines")
	})
}

func TestSlotIntervalTickerDoneOnZeroValueIsNoOp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ticker := &SlotIntervalTicker{}
		before := settledGoroutines()

		require.NotPanics(t, func() {
			ticker.Done()
			ticker.Done()
		})

		require.LessOrEqual(t, settledGoroutines(), before, "Done() on a zero-value ticker leaked goroutines")
	})
}
