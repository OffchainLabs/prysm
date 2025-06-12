// Package slots includes ticker and timer-related functions for Ethereum consensus.
package slots

import (
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
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
	c        chan primitives.Slot
	done     chan struct{}
	schedule *params.SlotSchedule
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
// This method panics if genesis time is zero.
// lint:nopanic -- Communicated panic in godoc commentary.
func NewSlotTicker(genesisTime time.Time, schedule *params.SlotSchedule) *SlotTicker {
	if genesisTime.IsZero() {
		panic("zero genesis time")
	}
	ticker := &SlotTicker{
		c:        make(chan primitives.Slot),
		done:     make(chan struct{}),
		schedule: schedule,
	}
	ticker.start(genesisTime, prysmTime.Since, prysmTime.Until, time.After)
	return ticker
}

// NewSlotTickerWithOffset starts and returns a SlotTicker instance that allows a offset of time from genesis,
// entering a offset greater than secondsPerSlot is not allowed.
// This method will panic if genesis time is zero or the offset is less than seconds per slot.
// lint:nopanic -- Communicated panic in godoc commentary.
func NewSlotTickerWithOffset(genesisTime time.Time, offset time.Duration, schedule *params.SlotSchedule) *SlotTicker {
	if genesisTime.Unix() == 0 {
		panic("zero genesis time")
	}
	if offset > schedule.CurrentSlotDuration(genesisTime) { // TODO: Handle schedule validation. It should check that all durations are greater than this value.
		panic("invalid ticker offset")
	}
	ticker := &SlotTicker{
		c:        make(chan primitives.Slot),
		done:     make(chan struct{}),
		schedule: schedule,
	}
	ticker.start(genesisTime.Add(offset), prysmTime.Since, prysmTime.Until, time.After)
	return ticker
}

func (s *SlotTicker) start(
	genesisTime time.Time,
	since, until func(time.Time) time.Duration,
	after func(time.Duration) <-chan time.Time) {

	go func() {
		sinceGenesis := since(genesisTime)

		var nextTickTime time.Time
		var slot primitives.Slot
		if sinceGenesis < s.schedule.SlotDuration(0) {
			// Handle when the current time is before the genesis time.
			nextTickTime = genesisTime
			slot = 0
		} else {
			d := s.schedule.SlotDuration(s.schedule.CurrentSlot(genesisTime))
			nextTick := sinceGenesis.Truncate(d) + d
			nextTickTime = genesisTime.Add(nextTick)
			slot = s.schedule.SlotAt(genesisTime, nextTickTime)
		}

		for {
			waitTime := until(nextTickTime)
			select {
			case <-after(waitTime):
				s.c <- slot
				slot++
				nextSlotDuration := s.schedule.SlotDuration(slot)
				nextTickTime = nextTickTime.Add(nextSlotDuration)
			case <-s.done:
				return
			}
		}
	}()
}

// startWithIntervals starts a ticker that emits a tick every slot at the
// prescribed intervals. The caller is responsible to make these intervals increasing and
// less than secondsPerSlot
// DEPRECATED: Does not support slot time schedule.
// TODO(preston): Update for slot time schedule.
func (s *SlotIntervalTicker) startWithIntervals(
	genesisTime time.Time,
	until func(time.Time) time.Duration,
	after func(time.Duration) <-chan time.Time,
	intervals []time.Duration) {
	go func() {
		slot := CurrentSlot(genesisTime)
		slot++
		interval := 0
		st, err := StartTime(genesisTime, slot)
		if err != nil {
			// TODO(preston): Handle.
			panic(err) // lint:nopanic -- DEBUG
		}
		nextTickTime := st.Add(intervals[0])

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
				st, err := StartTime(genesisTime, slot)
				if err != nil {
					// TODO(preston): Handle.
					panic(err) // lint:nopanic -- DEBUG
				}
				nextTickTime = st.Add(intervals[0])
			case <-s.done:
				return
			}
		}
	}()
}

// NewSlotTickerWithIntervals starts and returns a SlotTicker instance that allows
// several offsets of time from genesis,
// Caller is responsible to input the intervals in increasing order and none bigger or equal than
// SecondsPerSlot
// This method will panic if genesis time is zero, intervals is 0 length, or offsets are invalid.
// lint:nopanic -- Communicated panic in godoc commentary.
func NewSlotTickerWithIntervals(genesisTime time.Time, intervals []time.Duration) *SlotIntervalTicker {
	if genesisTime.Unix() == 0 {
		panic("zero genesis time")
	}
	if len(intervals) == 0 {
		panic("at least one interval has to be entered")
	}
	// TODO(preston): The one or more of the callers of this function expect 12 second slot times. This method will need to be reworked
	// properly account for the use cases where the caller wants an event at some fraction of a slot. I.e. half way, two thirds, etc.

	// For validation purposes, use the slot duration from slot 0
	// The ticker is primarily designed for the first slot duration
	schedule := params.BeaconConfig().SlotSchedule
	slotDuration := schedule.SlotDuration(0)

	// Validate intervals are increasing and not too large before adjustment
	lastOffset := time.Duration(0)
	for _, offset := range intervals {
		if offset < lastOffset {
			panic("invalid decreasing offsets")
		}
		if offset >= slotDuration {
			panic("invalid ticker offset")
		}
		lastOffset = offset
	}

	// All intervals are already validated to be within bounds
	ticker := &SlotIntervalTicker{
		c:    make(chan SlotInterval),
		done: make(chan struct{}),
	}
	ticker.startWithIntervals(genesisTime, prysmTime.Until, time.After, intervals)
	return ticker
}
