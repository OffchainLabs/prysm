// Package slots includes ticker and timer-related functions for Ethereum consensus.
package slots

import (
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/sirupsen/logrus"
)

// The Ticker interface defines a type which can expose a
// receive-only channel firing slot events.
type Ticker interface {
	C() <-chan primitives.Slot
	Done()
}

// SlotInterval is a wrapper that contains a slot and the interval index that
// triggered the ticker
type SlotInterval struct {
	Slot     primitives.Slot
	Interval int
}

// The IntervalTicker is similar to the Ticker interface but
// exposes also the interval along with the slot number
type IntervalTicker interface {
	C() <-chan SlotInterval
	Done()
}

// SlotTicker is a special ticker for the beacon chain block.
// The channel emits over the slot interval, and ensures that
// the ticks are in line with the genesis time. This means that
// the duration between the ticks and the genesis time are always a
// multiple of the slot duration.
// In addition, the channel returns the new slot number.
type SlotTicker struct {
	c    chan primitives.Slot
	done chan struct{}
}

// SlotIntervalTicker is similar to a slot ticker but it returns also
// the index of the interval that triggered the event
type SlotIntervalTicker struct {
	c    chan SlotInterval
	done chan struct{}
}

// C returns the ticker channel. Call Cancel afterwards to ensure
// that the goroutine exits cleanly.
func (s *SlotTicker) C() <-chan primitives.Slot {
	return s.c
}

// C returns the ticker channel. Call Cancel afterwards to ensure
// that the goroutine exits cleanly.
func (s *SlotIntervalTicker) C() <-chan SlotInterval {
	return s.c
}

// Done should be called to clean up the ticker.
func (s *SlotTicker) Done() {
	go func() {
		s.done <- struct{}{}
	}()
}

// Done should be called to clean up the ticker.
func (s *SlotIntervalTicker) Done() {
	go func() {
		s.done <- struct{}{}
	}()
}

// NewSlotTicker starts and returns a new SlotTicker instance.
// This method panics if genesis time is zero or the slot duration is not positive.
// lint:nopanic -- Communicated panic in godoc commentary.
func NewSlotTicker(genesisTime time.Time, slotDuration time.Duration) *SlotTicker {
	if genesisTime.IsZero() {
		panic("zero genesis time")
	}
	if slotDuration <= 0 {
		panic("non-positive slot duration")
	}
	ticker := &SlotTicker{
		c:    make(chan primitives.Slot),
		done: make(chan struct{}),
	}
	ticker.start(genesisTime, slotDuration, prysmTime.Since, prysmTime.Until, time.After)
	return ticker
}

func (s *SlotTicker) start(
	genesisTime time.Time,
	d time.Duration,
	since, until func(time.Time) time.Duration,
	after func(time.Duration) <-chan time.Time) {

	cfg := params.BeaconConfig()
	schedule, slotsPerEpoch := cfg.SlotSchedule, cfg.SlotsPerEpoch
	// The passed duration keeps working for callers with non-config slot times, the schedule wins when configured.
	durationAt := func(slot primitives.Slot) time.Duration {
		if len(schedule) == 0 {
			return d
		}
		return schedule.DurationAt(slot, slotsPerEpoch)
	}

	go func() {
		sinceGenesis := since(genesisTime)

		var nextTickTime time.Time
		var slot primitives.Slot
		if sinceGenesis < durationAt(0) {
			// Handle when the current time is before the genesis time.
			nextTickTime = genesisTime
			slot = 0
		} else if len(schedule) > 0 {
			slot = schedule.SlotAt(genesisTime, genesisTime.Add(sinceGenesis), slotsPerEpoch) + 1
			sg, err := schedule.SinceGenesis(slot, slotsPerEpoch)
			if err != nil {
				panic(err) // lint:nopanic -- Unreachable for a validated schedule and a present-day slot.
			}
			nextTickTime = genesisTime.Add(sg)
		} else {
			nextTick := sinceGenesis.Truncate(d) + d
			nextTickTime = genesisTime.Add(nextTick)
			slot = primitives.Slot(nextTick / d)
		}

		for {
			waitTime := until(nextTickTime)
			select {
			case <-after(waitTime):
				maybeLogSlotDurationChange(slot)
				s.c <- slot
				nextTickTime = nextTickTime.Add(durationAt(slot))
				slot++
			case <-s.done:
				return
			}
		}
	}()
}

// IntervalFunc resolves an offset from the start of the given slot, letting deadlines scale with the slot's duration.
type IntervalFunc func(primitives.Slot) time.Duration

// ComponentInterval returns an IntervalFunc for the given slot component in basis points.
func ComponentInterval(bp primitives.BP) IntervalFunc {
	return func(slot primitives.Slot) time.Duration {
		return params.BeaconConfig().SlotComponentDurationAt(bp, slot)
	}
}

// FixedInterval returns an IntervalFunc with a constant offset regardless of slot duration.
func FixedInterval(d time.Duration) IntervalFunc {
	return func(primitives.Slot) time.Duration {
		return d
	}
}

// startWithIntervals starts a ticker that emits a tick every slot at the
// prescribed intervals. The caller is responsible to make these intervals increasing and
// less than the slot duration.
func (s *SlotIntervalTicker) startWithIntervals(
	genesisTime time.Time,
	until func(time.Time) time.Duration,
	after func(time.Duration) <-chan time.Time,
	intervals []IntervalFunc) {
	go func() {
		slot := CurrentSlot(genesisTime)
		slot++
		interval := 0
		nextTickTime := UnsafeStartTime(genesisTime, slot).Add(intervals[0](slot))

		for {
			waitTime := until(nextTickTime)
			select {
			case <-after(waitTime):
				s.c <- SlotInterval{Slot: slot, Interval: interval}
				interval++
				if interval == len(intervals) {
					interval = 0
					slot++
				}
				nextTickTime = UnsafeStartTime(genesisTime, slot).Add(intervals[interval](slot))
			case <-s.done:
				return
			}
		}
	}()
}

// NewSlotTickerWithIntervalFuncs starts and returns a SlotIntervalTicker that ticks at
// per-slot offsets resolved by the given interval funcs. The caller is responsible to
// keep the resolved intervals increasing and less than the slot duration.
// This method will panic if genesis time is zero or intervals is 0 length.
// lint:nopanic -- Communicated panic in godoc commentary.
func NewSlotTickerWithIntervalFuncs(genesisTime time.Time, intervals []IntervalFunc) *SlotIntervalTicker {
	if genesisTime.Unix() == 0 {
		panic("zero genesis time")
	}
	if len(intervals) == 0 {
		panic("at least one interval has to be entered")
	}
	ticker := &SlotIntervalTicker{
		c:    make(chan SlotInterval),
		done: make(chan struct{}),
	}
	ticker.startWithIntervals(genesisTime, prysmTime.Until, time.After, intervals)
	return ticker
}

func (s *SlotTicker) startWithOffsetFunc(
	genesisTime time.Time,
	offset IntervalFunc,
	since, until func(time.Time) time.Duration,
	after func(time.Duration) <-chan time.Time) {
	go func() {
		var slot primitives.Slot
		if sinceGenesis := since(genesisTime); sinceGenesis > 0 {
			slot = At(genesisTime, genesisTime.Add(sinceGenesis))
		}
		// Skip the current slot when its offset tick has already passed.
		if until(UnsafeStartTime(genesisTime, slot).Add(offset(slot))) <= 0 {
			slot++
		}

		for {
			nextTickTime := UnsafeStartTime(genesisTime, slot).Add(offset(slot))
			select {
			case <-after(until(nextTickTime)):
				s.c <- slot
				slot++
			case <-s.done:
				return
			}
		}
	}()
}

// NewSlotTickerWithOffsetFunc starts and returns a SlotTicker that ticks once per slot at a
// per-slot offset resolved by the given func. The caller is responsible to keep the resolved
// offset smaller than the slot duration.
// This method will panic if genesis time is zero.
// lint:nopanic -- Communicated panic in godoc commentary.
func NewSlotTickerWithOffsetFunc(genesisTime time.Time, offset IntervalFunc) *SlotTicker {
	if genesisTime.Unix() == 0 {
		panic("zero genesis time")
	}
	ticker := &SlotTicker{
		c:    make(chan primitives.Slot),
		done: make(chan struct{}),
	}
	ticker.startWithOffsetFunc(genesisTime, offset, prysmTime.Since, prysmTime.Until, time.After)
	return ticker
}

// Deduplicates across the many tickers in a process so each boundary is announced once.
var lastLoggedDurationChange atomic.Uint64

func maybeLogSlotDurationChange(slot primitives.Slot) {
	if slot == 0 {
		return
	}
	cfg := params.BeaconConfig()
	if len(cfg.SlotSchedule) == 0 {
		return
	}
	prev, cur := cfg.SlotDurationAt(slot-1), cfg.SlotDurationAt(slot)
	if prev == cur {
		return
	}
	last := lastLoggedDurationChange.Load()
	if last == uint64(slot) || !lastLoggedDurationChange.CompareAndSwap(last, uint64(slot)) {
		return
	}
	log.WithFields(logrus.Fields{
		"slot":             slot,
		"epoch":            ToEpoch(slot),
		"previousDuration": prev,
		"newDuration":      cur,
	}).Info("Slot duration changed")
}
