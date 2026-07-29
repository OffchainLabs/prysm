package params

import (
	"math"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

// SlotScheduleEntry sets the slot duration starting at the given epoch, per EIP-7782's SLOT_SCHEDULE.
type SlotScheduleEntry struct {
	Epoch              primitives.Epoch `yaml:"EPOCH" json:"EPOCH"`
	SlotDurationMillis uint64           `yaml:"SLOT_DURATION_MS" json:"SLOT_DURATION_MS"`
}

type SlotSchedule []SlotScheduleEntry

func (s SlotSchedule) Validate(slotsPerEpoch primitives.Slot) error {
	if len(s) == 0 {
		return errors.New("empty slot schedule")
	}
	if s[0].Epoch != 0 {
		return errors.New("slot schedule must start at epoch 0")
	}
	for i, e := range s {
		if e.SlotDurationMillis == 0 {
			return errors.Errorf("slot schedule entry at epoch %d has zero duration", e.Epoch)
		}
		if i > 0 && e.Epoch <= s[i-1].Epoch {
			return errors.Errorf("slot schedule epochs must be strictly increasing, got epoch %d after %d", e.Epoch, s[i-1].Epoch)
		}
		if uint64(e.Epoch) > math.MaxUint64/uint64(slotsPerEpoch) {
			return errors.Errorf("slot schedule epoch %d overflows slot arithmetic", e.Epoch)
		}
	}
	return nil
}

// startSlot assumes the schedule passed Validate, which bounds epoch*slotsPerEpoch.
func (s SlotSchedule) startSlot(i int, slotsPerEpoch primitives.Slot) primitives.Slot {
	return primitives.Slot(uint64(s[i].Epoch) * uint64(slotsPerEpoch))
}

func (s SlotSchedule) entryFor(slot primitives.Slot, slotsPerEpoch primitives.Slot) SlotScheduleEntry {
	for i := len(s) - 1; i >= 0; i-- {
		if s.startSlot(i, slotsPerEpoch) <= slot {
			return s[i]
		}
	}
	return s[0]
}

func (s SlotSchedule) DurationAt(slot primitives.Slot, slotsPerEpoch primitives.Slot) time.Duration {
	return time.Duration(s.entryFor(slot, slotsPerEpoch).SlotDurationMillis) * time.Millisecond
}

// maxMs bounds millisecond totals so their time.Duration conversion cannot overflow int64.
const maxMs = uint64(math.MaxInt64 / int64(time.Millisecond))

// SinceGenesis returns the duration from genesis to the start of the given slot.
func (s SlotSchedule) SinceGenesis(slot primitives.Slot, slotsPerEpoch primitives.Slot) (time.Duration, error) {
	var totalMs uint64
	for i, e := range s {
		start := s.startSlot(i, slotsPerEpoch)
		if i == len(s)-1 || s.startSlot(i+1, slotsPerEpoch) > slot {
			delta, err := slot.SafeSub(uint64(start))
			if err != nil {
				return 0, errors.Wrapf(err, "slot %d precedes schedule entry at epoch %d", slot, e.Epoch)
			}
			ms, err := delta.SafeMul(e.SlotDurationMillis)
			if err != nil || uint64(ms) > maxMs-totalMs {
				return 0, errors.Errorf("slot %d is in the far distant future", slot)
			}
			return time.Duration(totalMs+uint64(ms)) * time.Millisecond, nil
		}
		span, err := (s.startSlot(i+1, slotsPerEpoch) - start).SafeMul(e.SlotDurationMillis)
		if err != nil || uint64(span) > maxMs-totalMs {
			return 0, errors.Errorf("slot schedule entry at epoch %d is too far in the future", s[i+1].Epoch)
		}
		totalMs += uint64(span)
	}
	return 0, errors.New("empty slot schedule")
}

func (s SlotSchedule) SlotAt(genesis, tm time.Time, slotsPerEpoch primitives.Slot) primitives.Slot {
	if tm.Before(genesis) {
		return 0
	}
	remaining := tm.Sub(genesis)
	for i, e := range s {
		d := time.Duration(e.SlotDurationMillis) * time.Millisecond
		if i < len(s)-1 {
			spanSlots := uint64(s.startSlot(i+1, slotsPerEpoch) - s.startSlot(i, slotsPerEpoch))
			// A span too large to represent as a duration necessarily contains any remaining time.
			if spanSlots <= uint64(math.MaxInt64)/uint64(d) {
				span := time.Duration(spanSlots) * d
				if remaining >= span {
					remaining -= span
					continue
				}
			}
		}
		return s.startSlot(i, slotsPerEpoch) + primitives.Slot(remaining/d)
	}
	return 0
}
