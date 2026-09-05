package slots

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	logTest "github.com/sirupsen/logrus/hooks/test"
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

		offsetTicker := NewSlotTickerWithOffsetFunc(genesisTime, FixedInterval(offset))
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

		intervalTicker := NewSlotTickerWithIntervalFuncs(genesisTime, []IntervalFunc{FixedInterval(intervals[0]), FixedInterval(intervals[1])})
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
	intervals := make([]IntervalFunc, 0)
	panicCall := func() {
		NewSlotTickerWithIntervalFuncs(genesisTime, intervals)
	}
	require.Panics(t, panicCall, "zero genesis time")
	genesisTime = time.Now()
	require.Panics(t, panicCall, "at least one interval has to be entered")
	intervals = []IntervalFunc{FixedInterval(offset), FixedInterval(2 * offset)}
	require.NotPanics(t, panicCall)
}

func TestSlotTickerFollowsSlotSchedule(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig().Copy()
	cfg.SlotSchedule = params.SlotSchedule{
		{Epoch: 0, SlotDurationMillis: 12000},
		{Epoch: 1, SlotDurationMillis: 6000},
	}
	params.OverrideBeaconConfig(cfg)

	synctest.Test(t, func(t *testing.T) {
		genesisTime := time.Now()
		ticker := NewSlotTicker(genesisTime, cfg.SlotDuration())
		defer ticker.Done()

		lastSlotOfEpoch0 := primitives.Slot(cfg.SlotsPerEpoch) - 1
		for want := primitives.Slot(0); want <= lastSlotOfEpoch0+3; want++ {
			require.Equal(t, want, <-ticker.C())
			switch want {
			case lastSlotOfEpoch0:
				require.Equal(t, 31*12*time.Second, time.Since(genesisTime))
			case lastSlotOfEpoch0 + 1:
				require.Equal(t, 32*12*time.Second, time.Since(genesisTime))
			case lastSlotOfEpoch0 + 2:
				require.Equal(t, 32*12*time.Second+6*time.Second, time.Since(genesisTime))
			case lastSlotOfEpoch0 + 3:
				require.Equal(t, 32*12*time.Second+12*time.Second, time.Since(genesisTime))
			}
		}
	})
}

func TestSlotTickerMidScheduleStart(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig().Copy()
	cfg.SlotSchedule = params.SlotSchedule{
		{Epoch: 0, SlotDurationMillis: 12000},
		{Epoch: 1, SlotDurationMillis: 6000},
	}
	params.OverrideBeaconConfig(cfg)

	synctest.Test(t, func(t *testing.T) {
		genesisTime := time.Now()
		// Start the ticker two slots into epoch 1, mid slot.
		time.Sleep(32*12*time.Second + 2*6*time.Second + 3*time.Second)
		ticker := NewSlotTicker(genesisTime, cfg.SlotDuration())
		defer ticker.Done()

		require.Equal(t, primitives.Slot(35), <-ticker.C())
		require.Equal(t, 32*12*time.Second+3*6*time.Second, time.Since(genesisTime))
	})
}

func TestSlotTickerWithOffsetFuncFollowsSlotSchedule(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig().Copy()
	cfg.SlotSchedule = params.SlotSchedule{
		{Epoch: 0, SlotDurationMillis: 12000},
		{Epoch: 1, SlotDurationMillis: 6000},
	}
	params.OverrideBeaconConfig(cfg)

	synctest.Test(t, func(t *testing.T) {
		genesisTime := time.Now()
		// 3333 bps resolves to 3999ms of a 12s slot and 1999ms of a 6s slot.
		ticker := NewSlotTickerWithOffsetFunc(genesisTime, ComponentInterval(3333))
		defer ticker.Done()

		lastSlotOfEpoch0 := primitives.Slot(cfg.SlotsPerEpoch) - 1
		for want := primitives.Slot(0); want <= lastSlotOfEpoch0+2; want++ {
			require.Equal(t, want, <-ticker.C())
			switch want {
			case primitives.Slot(0):
				require.Equal(t, 3999*time.Millisecond, time.Since(genesisTime))
			case lastSlotOfEpoch0:
				require.Equal(t, 31*12*time.Second+3999*time.Millisecond, time.Since(genesisTime))
			case lastSlotOfEpoch0 + 1:
				require.Equal(t, 32*12*time.Second+1999*time.Millisecond, time.Since(genesisTime))
			case lastSlotOfEpoch0 + 2:
				require.Equal(t, 32*12*time.Second+6*time.Second+1999*time.Millisecond, time.Since(genesisTime))
			}
		}
	})
}

func TestSlotTickerWithOffsetFuncMidSlotStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		genesisTime := time.Now()
		offset := params.BeaconConfig().SlotDuration() / 3

		// Started past slot 0's offset tick, the first tick must be slot 1's.
		time.Sleep(offset + time.Second)
		ticker := NewSlotTickerWithOffsetFunc(genesisTime, FixedInterval(offset))
		defer ticker.Done()

		require.Equal(t, primitives.Slot(1), <-ticker.C())
		require.Equal(t, params.BeaconConfig().SlotDuration()+offset, time.Since(genesisTime))
	})
}

func TestSlotDurationChangeLoggedOnce(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig().Copy()
	cfg.SlotSchedule = params.SlotSchedule{
		{Epoch: 0, SlotDurationMillis: 12000},
		{Epoch: 1, SlotDurationMillis: 6000},
	}
	params.OverrideBeaconConfig(cfg)
	lastLoggedDurationChange.Store(0)
	hook := logTest.NewGlobal()

	synctest.Test(t, func(t *testing.T) {
		genesisTime := time.Now()
		ticker := NewSlotTicker(genesisTime, cfg.SlotDuration())
		defer ticker.Done()
		// Two tickers must announce the boundary once.
		other := NewSlotTicker(genesisTime, cfg.SlotDuration())
		defer other.Done()

		boundary := primitives.Slot(cfg.SlotsPerEpoch)
		for slot := primitives.Slot(0); slot <= boundary+1; slot++ {
			require.Equal(t, slot, <-ticker.C())
			require.Equal(t, slot, <-other.C())
		}
	})

	count := 0
	for _, e := range hook.AllEntries() {
		if e.Message == "Slot duration changed" {
			count++
		}
	}
	require.Equal(t, 1, count)
}
