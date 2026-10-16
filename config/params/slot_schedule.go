package params

import (
	"math"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

// SlotScheduleEntry is one slot duration era, starting at the given epoch.
type SlotScheduleEntry struct {
	Epoch              primitives.Epoch
	SlotDurationMillis uint64
}

// SlotSchedule lists slot duration eras in increasing epoch order, starting at the genesis epoch.
type SlotSchedule []SlotScheduleEntry

const (
	// maxMs bounds millisecond totals so their time.Duration conversion cannot overflow int64.
	maxMs = uint64(math.MaxInt64 / int64(time.Millisecond))

	// https://github.com/ethereum/consensus-specs/blob/master/specs/heze/beacon-chain.md#time-parameters
	slotDurationGranularityMillis = 1000
)

// SlotComponent names an intra-slot deadline.
type SlotComponent uint8

const (
	ProposerReorgCutoff SlotComponent = iota
	AttestationDue
	AggregateDue
	SyncMessageDue
	ContributionDue
	PayloadDue
	PayloadAttestationDue
)

// SinceGenesis returns the duration from genesis to the start of the given slot.
// https://github.com/ethereum/consensus-specs/blob/master/specs/heze/beacon-chain.md#modified-compute_time_at_slot_ms
func (s SlotSchedule) SinceGenesis(slot primitives.Slot, slotsPerEpoch primitives.Slot) (time.Duration, error) {
	totalMs := uint64(0)
	end := slot
	for i, entry := range slices.Backward(s) {
		start := s.startSlot(i, slotsPerEpoch)
		if start >= end {
			continue
		}

		ms, err := (end - start).SafeMul(entry.SlotDurationMillis)
		if err != nil || uint64(ms) > maxMs-totalMs {
			return 0, errors.Errorf("slot %d is in the far distant future", slot)
		}

		totalMs += uint64(ms)
		end = start
	}

	return time.Duration(totalMs) * time.Millisecond, nil
}

// SlotAt returns the slot at the given time.
// https://github.com/ethereum/consensus-specs/blob/master/specs/heze/beacon-chain.md#modified-compute_slot_at_time_ms
func (s SlotSchedule) SlotAt(genesis, tm time.Time, slotsPerEpoch primitives.Slot) primitives.Slot {
	if tm.Before(genesis) {
		return 0
	}

	return s.slotAfter(tm.Sub(genesis), slotsPerEpoch)
}

// durationAt returns the duration of the given slot, which is the slot duration of the era containing it.
// The spec takes an epoch instead of a slot, the result is the same for every slot of an epoch.
// https://github.com/ethereum/consensus-specs/blob/master/specs/heze/beacon-chain.md#new-get_slot_duration_ms
func (s SlotSchedule) durationAt(slot primitives.Slot, slotsPerEpoch primitives.Slot) time.Duration {
	return time.Duration(s.entryFor(slot, slotsPerEpoch).SlotDurationMillis) * time.Millisecond
}

// entryFor returns the entry of the era containing the given slot.
func (s SlotSchedule) entryFor(slot primitives.Slot, slotsPerEpoch primitives.Slot) SlotScheduleEntry {
	return s.entryForEpoch(primitives.Epoch(slot / slotsPerEpoch))
}

// entryForEpoch returns the entry of the era containing the given epoch, which is the last entry starting at or before it.
func (s SlotSchedule) entryForEpoch(epoch primitives.Epoch) SlotScheduleEntry {
	for _, entry := range slices.Backward(s) {
		if entry.Epoch <= epoch {
			return entry
		}
	}

	return s[0]
}

// startSlot assumes the config passed validateSlotDurations, which bounds epoch*slotsPerEpoch.
func (s SlotSchedule) startSlot(i int, slotsPerEpoch primitives.Slot) primitives.Slot {
	return primitives.Slot(uint64(s[i].Epoch) * uint64(slotsPerEpoch))
}

// slotAfter returns the slot in progress the given duration after genesis.
func (s SlotSchedule) slotAfter(elapsed time.Duration, slotsPerEpoch primitives.Slot) primitives.Slot {
	for i, entry := range slices.Backward(s) {
		forkSlot := s.startSlot(i, slotsPerEpoch)
		forkTime, err := s.SinceGenesis(forkSlot, slotsPerEpoch)

		// An era start too far to represent as a duration is necessarily after elapsed.
		if err != nil || elapsed < forkTime {
			continue
		}

		duration := time.Duration(entry.SlotDurationMillis) * time.Millisecond
		return forkSlot + primitives.Slot((elapsed-forkTime)/duration)
	}

	return 0
}

// SlotDurations returns the slot duration eras derived from the fork configuration, skipping unscheduled forks.
// https://github.com/ethereum/consensus-specs/blob/master/specs/heze/beacon-chain.md#new-get_slot_durations
func (b *BeaconChainConfig) SlotDurations() SlotSchedule {
	schedule := SlotSchedule{{Epoch: b.GenesisEpoch, SlotDurationMillis: b.SlotDurationMillis()}}
	if b.HezeForkEpoch != b.FarFutureEpoch {
		schedule = append(schedule, SlotScheduleEntry{Epoch: b.HezeForkEpoch, SlotDurationMillis: b.SlotDurationMillisHeze})
	}

	return schedule
}

// HasSlotDurationChange reports whether a fork changing the slot duration is scheduled.
func (b *BeaconChainConfig) HasSlotDurationChange() bool {
	return len(b.SlotDurations()) > 1
}

// SinceGenesis returns the duration from genesis to the start of the given slot.
func (b *BeaconChainConfig) SinceGenesis(slot primitives.Slot) (time.Duration, error) {
	return b.SlotDurations().SinceGenesis(slot, b.SlotsPerEpoch)
}

// SlotDurationAt returns the duration of the given slot.
func (b *BeaconChainConfig) SlotDurationAt(slot primitives.Slot) time.Duration {
	return b.SlotDurations().durationAt(slot, b.SlotsPerEpoch)
}

// SlotDurationMillisAtEpoch returns the slot duration in effect at the given epoch, in milliseconds.
// https://github.com/ethereum/consensus-specs/blob/master/specs/heze/beacon-chain.md#new-get_slot_duration_ms
func (b *BeaconChainConfig) SlotDurationMillisAtEpoch(epoch primitives.Epoch) uint64 {
	return b.SlotDurations().entryForEpoch(epoch).SlotDurationMillis
}

// SlotFractionAt returns the given basis-point fraction of the slot's duration.
func (b *BeaconChainConfig) SlotFractionAt(bp primitives.BP, slot primitives.Slot) time.Duration {
	ms := uint64(bp) * uint64(b.SlotDurationAt(slot)/time.Millisecond) / uint64(BasisPoints)
	return time.Duration(ms) * time.Millisecond
}

// SlotComponentDurationAt returns the offset of the given deadline into the given slot.
// From Heze, the basis points apply to SLOT_DURATION_MS_HEZE, which is the duration of every Heze slot.
// https://github.com/ethereum/consensus-specs/blob/master/specs/heze/fork-choice.md#modified-get_slot_component_duration_ms
func (b *BeaconChainConfig) SlotComponentDurationAt(c SlotComponent, slot primitives.Slot) time.Duration {
	return b.SlotFractionAt(b.slotComponentBPS(c, slot), slot)
}

// BlobDataRetentionStartEpoch returns the earliest epoch of the data column retention window at the given epoch.
// Before Heze, the window is the given number of epochs. From Heze, it keeps MIN_BLOB_DATA_RETENTION_MS of wall-clock
// time across slot duration changes, or the given number of epochs at the genesis slot duration if that is longer.
// https://github.com/ethereum/consensus-specs/blob/master/specs/heze/p2p-interface.md#new-compute_blob_data_retention_start_epoch
func (b *BeaconChainConfig) BlobDataRetentionStartEpoch(current, epochs primitives.Epoch) primitives.Epoch {
	fallback := primitives.Epoch(0)
	if current > epochs {
		fallback = current - epochs
	}

	if current < b.HezeForkEpoch {
		return fallback
	}

	s, spe := b.SlotDurations(), b.SlotsPerEpoch

	// If the window cannot be expressed as a duration, it is counted in epochs.
	genesisMs := b.SlotDurationMillisAtEpoch(b.GenesisEpoch)
	if uint64(epochs) > maxMs/uint64(spe)/genesisMs || b.MinBlobDataRetentionMillis > maxMs {
		return fallback
	}
	windowMs := max(b.MinBlobDataRetentionMillis, uint64(epochs)*uint64(spe)*genesisMs)
	window := time.Duration(windowMs) * time.Millisecond

	currentStart, err := s.SinceGenesis(primitives.Slot(current)*spe, spe)
	if err != nil {
		return fallback
	}

	if currentStart < window {
		return 0
	}

	return primitives.Epoch(s.slotAfter(currentStart-window, spe) / spe)
}

// validateSlotDurations checks the slot durations derived from the fork configuration.
func (b *BeaconChainConfig) validateSlotDurations() error {
	if b.SlotsPerEpoch == 0 {
		return errors.New("slots per epoch must be positive")
	}

	for _, e := range b.SlotDurations()[1:] {
		if e.SlotDurationMillis == 0 || e.SlotDurationMillis%slotDurationGranularityMillis != 0 {
			return errors.Errorf("slot duration %dms at epoch %d must be a positive multiple of %d", e.SlotDurationMillis, e.Epoch, slotDurationGranularityMillis)
		}

		if uint64(e.Epoch) > math.MaxUint64/uint64(b.SlotsPerEpoch) {
			return errors.Errorf("slot duration change at epoch %d overflows slot arithmetic", e.Epoch)
		}
	}

	return nil
}

func (b *BeaconChainConfig) slotComponentBPS(c SlotComponent, slot primitives.Slot) primitives.BP {
	postGloas := primitives.Epoch(slot.DivSlot(b.SlotsPerEpoch)) >= b.GloasForkEpoch
	switch c {
	case ProposerReorgCutoff:
		return b.ProposerReorgCutoffBPS
	case AttestationDue:
		if postGloas {
			return b.AttestationDueBPSGloas
		}
		return b.AttestationDueBPS
	case AggregateDue:
		if postGloas {
			return b.AggregateDueBPSGloas
		}
		return b.AggregateDueBPS
	case SyncMessageDue:
		if postGloas {
			return b.SyncMessageDueBPSGloas
		}
		return b.SyncMessageDueBPS
	case ContributionDue:
		if postGloas {
			return b.ContributionDueBPSGloas
		}
		return b.ContributionDueBPS
	case PayloadDue:
		return b.PayloadDueBPS
	case PayloadAttestationDue:
		return b.PayloadAttestationDueBPS
	default:
		return 0
	}
}
