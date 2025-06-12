package params

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var ErrInvalidSlotScheduleNoGenesis = errors.New("invalid slot schedule, missing an entry for epoch 0")

type SlotSchedule []SlotScheduleEntry

// SlotScheduleEntry defines a schedule entry which adjusts the seconds per slot starting at
// the given epoch. This works similarly to the blob schedule.
type SlotScheduleEntry struct {
	Epoch        primitives.Epoch
	SlotDuration time.Duration
}

// IsValid ensures that there is at least one entry with epoch 0 and that all entries have an epoch
// with a value less than MaxSafeEpoch. It also ensures that every duration is at least 1 second.
func (s *SlotSchedule) IsValid() error {
	if s == nil || s.Length() < 1 {
		return errors.New("empty schedule")
	}
	if (*s)[0].Epoch != 0 {
		return errors.New("first entry must start with epoch 0")
	}
	
	// Check for duplicates, sorting, and minimum duration
	seen := make(map[primitives.Epoch]bool)
	var lastEpoch primitives.Epoch
	for i, entry := range *s {
		// Check for duplicate epochs
		if seen[entry.Epoch] {
			return fmt.Errorf("duplicate epoch %d in schedule", entry.Epoch)
		}
		seen[entry.Epoch] = true
		
		// Check for proper sorting (epochs must be strictly increasing)
		if i > 0 && entry.Epoch <= lastEpoch {
			return fmt.Errorf("schedule must be sorted by epoch: epoch %d appears after epoch %d", entry.Epoch, lastEpoch)
		}
		lastEpoch = entry.Epoch
		
		// Check minimum duration (must be at least 1 second)
		if entry.SlotDuration < time.Second {
			return fmt.Errorf("slot duration %v at epoch %d is less than minimum 1 second", entry.SlotDuration, entry.Epoch)
		}
	}
	
	return nil
}

func (s *SlotSchedule) CurrentSlot(genesis time.Time) primitives.Slot {
	return s.SlotAt(genesis, time.Now())
}

// SlotAt calculates the slot number at a given time using the SlotTimeSchedule.
// This is the correct implementation that accounts for variable slot durations.
// Assumes the schedule is already sorted (invariant maintained by construction).
func (s *SlotSchedule) SlotAt(genesis, tm time.Time) primitives.Slot {
	if s == nil {
		return 0
	}
	if tm.Before(genesis) {
		return 0
	}

	remaining := tm.Sub(genesis)
	for i, e := range *s {
		// Is this the last bucket? If so, return the result.
		if i == s.Length()-1 {
			return unsafeEpochStart(e.Epoch) + primitives.Slot(remaining/e.SlotDuration)
		}
		// Does remaining fit in the current bucket?
		epochDiff := (*s)[i+1].Epoch - e.Epoch
		wholeEntryDuration := time.Duration(epochDiff) * time.Duration(BeaconConfig().SlotsPerEpoch) * e.SlotDuration
		// Yes -> return StartSlot(e.Epoch) + remaining / e.SlotDuration.
		if remaining < wholeEntryDuration {
			return unsafeEpochStart(e.Epoch) + primitives.Slot(remaining/e.SlotDuration)
		}
		// No -> remove the full bucket period from remaining.
		remaining -= wholeEntryDuration
	}

	return 0 // This should never happen.
}

// CurrentSlotDuration returns the slot duration given the current slot on the schedule.
func (s *SlotSchedule) CurrentSlotDuration(genesis time.Time) time.Duration {
	return s.SlotDuration(s.CurrentSlot(genesis))
}

// SinceGenesis will return the amount of time since genesis for a given slot. May return an error
// when the slot value would cause an overflow or underflow.
// Assumes the schedule is already sorted (invariant maintained by construction).
func (s *SlotSchedule) SinceGenesis(slot primitives.Slot) (time.Duration, error) {
	if s == nil {
		return 0, errors.New("nil SlotTimeSchedule")
	}

	var tm time.Duration
	for i, e := range *s {
		if i == s.Length()-1 || unsafeEpochStart((*s)[i+1].Epoch) > slot {
			delta, err := slot.SafeSub(uint64(unsafeEpochStart(e.Epoch)))
			if err != nil {
				return 0, fmt.Errorf("failed to compute the number of slots into the epoch: %w", err)
			}
			// Using SafeMul since the slot delta could overflow the result when converted to a duration.
			dt, err := delta.SafeMul(uint64(e.SlotDuration))
			if err != nil {
				return 0, fmt.Errorf("failed to compute the number of slots into the epoch: %w", err)
			}

			return tm + time.Duration(dt), nil
		}
		delta, err := (*s)[i+1].Epoch.SafeSub(uint64(e.Epoch))
		if err != nil {
			return 0, fmt.Errorf("failed to compute the number of slots in a SlotTimeSchedule entry: %w", err)
		}

		tm += (time.Duration(primitives.Slot(delta)*BeaconConfig().SlotsPerEpoch) * e.SlotDuration)
	}

	return 0, errors.New("not implemented")
}

// This is a copy from slots.EpochStart, but avoids the circular dependency.
func epochStart(e primitives.Epoch) (primitives.Slot, error) {
	slot, err := BeaconConfig().SlotsPerEpoch.SafeMul(uint64(e))
	if err != nil {
		return slot, fmt.Errorf("start slot calculation overflows: %w", err)
	}
	return slot, nil
}

// This is a copy from slots.UnsafeEpochStart, but avoids the circular dependency.
func unsafeEpochStart(epoch primitives.Epoch) primitives.Slot {
	es, err := epochStart(epoch)
	if err != nil {
		panic(err) // lint:nopanic -- Unsafe is implied and communicated in the godoc commentary.
	}
	return es
}

// sort ensures the schedule is sorted. This is now primarily used during initialization.
// The schedule should already be sorted as an invariant, but this method exists for
// compatibility and as a safety check.
func (s *SlotSchedule) sort() {
	if s != nil && s.Length() > 1 {
		// Check if already sorted to avoid unnecessary work
		alreadySorted := true
		for i := 1; i < s.Length(); i++ {
			if (*s)[i].Epoch <= (*s)[i-1].Epoch {
				alreadySorted = false
				break
			}
		}
		
		if !alreadySorted {
			sort.Sort(s)
		}
	}

	// Validate after sorting to ensure schedule integrity.
	// Invalid schedules indicate a programming error in configuration that should be
	// caught during development/testing. We panic here to fail fast rather than
	// silently return incorrect slot calculations.
	if err := s.IsValid(); err != nil {
		panic(fmt.Sprintf("invalid SlotTimeSchedule configuration: %v", err)) // lint:nopanic -- Programming error that must be fixed during development
	}
}

// Implement sort.Interface for SlotTimeSchedule
func (s *SlotSchedule) Len() int {
	return s.Length()
}

func (s *SlotSchedule) Less(i, j int) bool {
	return (*s)[i].Epoch < (*s)[j].Epoch
}

func (s *SlotSchedule) Swap(i, j int) {
	(*s)[i], (*s)[j] = (*s)[j], (*s)[i]
}

// SlotDuration returns the amount of time in a given slot. For example, 12 seconds per slot for
// Ethereum's original slot duration.
// Assumes the schedule is already sorted (invariant maintained by construction).
func (s *SlotSchedule) SlotDuration(slot primitives.Slot) time.Duration {
	if s == nil {
		return 0
	}

	// Shortcut until a full schedule is defined.
	if s.Length() == 1 {
		return (*s)[0].SlotDuration
	}

	for i := s.Length() - 1; i >= 0; i-- {
		if BeaconConfig().SlotsPerEpoch.Mul(uint64((*s)[i].Epoch)) <= slot {
			return (*s)[i].SlotDuration
		}
	}

	// This should be unreachable for valid schedules (which must start at epoch 0)
	// but we defensively return epoch 0's duration if we somehow get here
	if s.Length() > 0 {
		log.WithField("slot", slot).Warn("SlotDuration: slot before first epoch, using epoch 0 duration")
		return (*s)[0].SlotDuration
	}

	// If we have no entries at all, this is a programming error
	log.WithField("slot", slot).Error("SlotDuration: empty schedule - this should not happen")
	return 0
}

// Length returns the number of entries in the SlotTimeSchedule.
func (s *SlotSchedule) Length() int {
	if s == nil {
		return 0
	}
	return len(*s)
}

var _ yaml.Unmarshaler = &SlotSchedule{}
var _ yaml.Marshaler = &SlotSchedule{}

type rawYamlEntry struct {
	Epoch          uint64  `yaml:"EPOCH"`
	SecondsPerSlot float64 `yaml:"SECONDS_PER_SLOT"` // seconds as float64
}

// UnmarshalYAML satisifies the yaml.Unmarshaler interface. It is necessary to represent the
// SlotDuration as a time.Duration value while the yaml file requires it to be represented as a
// numeric value with the unit of seconds.
func (s *SlotSchedule) UnmarshalYAML(n *yaml.Node) error {
	var rawEntries []rawYamlEntry
	if err := n.Decode(&rawEntries); err != nil {
		return err
	}

	entries := make([]SlotScheduleEntry, len(rawEntries))
	for i, raw := range rawEntries {
		entries[i] = SlotScheduleEntry{
			Epoch:        primitives.Epoch(raw.Epoch),
			SlotDuration: time.Duration(raw.SecondsPerSlot * float64(time.Second)),
		}
	}

	*s = entries

	// Sort the schedule to maintain the invariant that it's always sorted
	if s.Length() > 1 {
		sort.Sort(s)
	}

	// Validate the schedule during unmarshaling to catch configuration errors early
	if err := s.IsValid(); err != nil {
		return fmt.Errorf("invalid SlotTimeSchedule configuration: %v", err)
	}

	return nil
}

// MarshalYAML satisifies the yaml.Marshaler interface. It is necessary to represent the
// SlotDuration as a time.Duration value while the yaml file requires it to be represented as a
// numeric value with the unit of seconds.
func (s *SlotSchedule) MarshalYAML() (interface{}, error) {
	if s == nil {
		return nil, nil
	}
	rawEntries := make([]rawYamlEntry, s.Length())
	for i, entry := range *s {
		rawEntries[i] = rawYamlEntry{
			Epoch:          uint64(entry.Epoch),
			SecondsPerSlot: float64(entry.SlotDuration) / float64(time.Second),
		}
	}

	return rawEntries, nil
}
