package slots

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/stretchr/testify/require"
)

var _ Ticker = (*SlotTicker)(nil)

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
