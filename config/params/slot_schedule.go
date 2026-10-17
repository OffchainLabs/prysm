package params

import (
	"math"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

// SlotScheduleEntry is one slot duration era of EIP-8198's SLOT_DURATION_SCHEDULE.
type SlotScheduleEntry struct {
	Epoch                       primitives.Epoch `yaml:"EPOCH" json:"EPOCH"`
	SlotDurationMillis          uint64           `yaml:"SLOT_DURATION_MS" json:"SLOT_DURATION_MS"`
	ProposerReorgCutoffMillis   uint64           `yaml:"PROPOSER_REORG_CUTOFF_MS" json:"PROPOSER_REORG_CUTOFF_MS"`
	AttestationDueMillis        uint64           `yaml:"ATTESTATION_DUE_MS" json:"ATTESTATION_DUE_MS"`
	AggregateDueMillis          uint64           `yaml:"AGGREGATE_DUE_MS" json:"AGGREGATE_DUE_MS"`
	SyncMessageDueMillis        uint64           `yaml:"SYNC_MESSAGE_DUE_MS" json:"SYNC_MESSAGE_DUE_MS"`
	ContributionDueMillis       uint64           `yaml:"CONTRIBUTION_DUE_MS" json:"CONTRIBUTION_DUE_MS"`
	PayloadDueMillis            uint64           `yaml:"PAYLOAD_DUE_MS" json:"PAYLOAD_DUE_MS"`
	PayloadAttestationDueMillis uint64           `yaml:"PAYLOAD_ATTESTATION_DUE_MS" json:"PAYLOAD_ATTESTATION_DUE_MS"`
	InclusionListDueMillis      uint64           `yaml:"INCLUSION_LIST_DUE_MS" json:"INCLUSION_LIST_DUE_MS"`
}

type SlotSchedule []SlotScheduleEntry

// SlotComponent names an intra-slot deadline independently of how a given era expresses it.
type SlotComponent uint8

const (
	ProposerReorgCutoff SlotComponent = iota
	AttestationDue
	AggregateDue
	SyncMessageDue
	ContributionDue
	PayloadDue
	PayloadAttestationDue
	InclusionListDue
)

func (e SlotScheduleEntry) deadlineMillis(c SlotComponent) uint64 {
	switch c {
	case ProposerReorgCutoff:
		return e.ProposerReorgCutoffMillis
	case AttestationDue:
		return e.AttestationDueMillis
	case AggregateDue:
		return e.AggregateDueMillis
	case SyncMessageDue:
		return e.SyncMessageDueMillis
	case ContributionDue:
		return e.ContributionDueMillis
	case PayloadDue:
		return e.PayloadDueMillis
	case PayloadAttestationDue:
		return e.PayloadAttestationDueMillis
	case InclusionListDue:
		return e.InclusionListDueMillis
	default:
		return 0
	}
}

func (e SlotScheduleEntry) validate() error {
	if e.SlotDurationMillis == 0 {
		return errors.Errorf("slot schedule entry at epoch %d has zero duration", e.Epoch)
	}
	if e.SlotDurationMillis%1000 != 0 {
		return errors.Errorf("slot schedule entry at epoch %d has duration %dms, must be a multiple of 1000", e.Epoch, e.SlotDurationMillis)
	}
	for _, d := range []struct {
		name string
		ms   uint64
	}{
		{"PROPOSER_REORG_CUTOFF_MS", e.ProposerReorgCutoffMillis},
		{"ATTESTATION_DUE_MS", e.AttestationDueMillis},
		{"AGGREGATE_DUE_MS", e.AggregateDueMillis},
		{"SYNC_MESSAGE_DUE_MS", e.SyncMessageDueMillis},
		{"CONTRIBUTION_DUE_MS", e.ContributionDueMillis},
		{"PAYLOAD_DUE_MS", e.PayloadDueMillis},
		{"PAYLOAD_ATTESTATION_DUE_MS", e.PayloadAttestationDueMillis},
		{"INCLUSION_LIST_DUE_MS", e.InclusionListDueMillis},
	} {
		if d.ms == 0 || d.ms >= e.SlotDurationMillis {
			return errors.Errorf("slot schedule entry at epoch %d has %s=%d, must be positive and below the slot duration %d", e.Epoch, d.name, d.ms, e.SlotDurationMillis)
		}
	}
	if e.ProposerReorgCutoffMillis >= e.AttestationDueMillis || e.AttestationDueMillis >= e.AggregateDueMillis {
		return errors.Errorf("slot schedule entry at epoch %d needs proposer reorg cutoff < attestation due < aggregate due", e.Epoch)
	}
	if e.SyncMessageDueMillis >= e.ContributionDueMillis {
		return errors.Errorf("slot schedule entry at epoch %d needs sync message due < contribution due", e.Epoch)
	}
	if e.PayloadDueMillis >= e.PayloadAttestationDueMillis {
		return errors.Errorf("slot schedule entry at epoch %d needs payload due < payload attestation due", e.Epoch)
	}
	return nil
}

func (s SlotSchedule) Validate(slotsPerEpoch primitives.Slot) error {
	if len(s) == 0 {
		return errors.New("empty slot schedule")
	}
	if s[0].Epoch != 0 {
		return errors.New("slot schedule must start at epoch 0")
	}
	for i, e := range s {
		if err := e.validate(); err != nil {
			return err
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

// Duration changes must coincide with a fork so the fork digest separates the two timing domains on gossip.
func (s SlotSchedule) validateForkAlignment(b *BeaconChainConfig) error {
	for _, e := range s[1:] {
		// The spec builds on Heze, so pre-Gloas eras have no scaled churn rules to apply.
		if e.Epoch < b.GloasForkEpoch {
			return errors.Errorf("slot schedule epoch %d precedes GLOAS_FORK_EPOCH", e.Epoch)
		}
		aligned := false
		for _, fe := range b.VersionToForkEpochMap() {
			if fe == e.Epoch {
				aligned = true
				break
			}
		}
		if !aligned {
			return errors.Errorf("slot schedule epoch %d does not coincide with a fork epoch", e.Epoch)
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

func (s SlotSchedule) entryForEpoch(epoch primitives.Epoch) SlotScheduleEntry {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i].Epoch <= epoch {
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

// SlotDurationAt returns the duration of the given slot per the slot duration schedule.
func (b *BeaconChainConfig) SlotDurationAt(slot primitives.Slot) time.Duration {
	if len(b.SlotDurationSchedule) == 0 {
		return b.SlotDuration()
	}
	return b.SlotDurationSchedule.DurationAt(slot, b.SlotsPerEpoch)
}

// SlotDurationMillisAtEpoch returns the slot duration in effect at the given epoch, in milliseconds.
func (b *BeaconChainConfig) SlotDurationMillisAtEpoch(epoch primitives.Epoch) uint64 {
	if len(b.SlotDurationSchedule) == 0 {
		return b.SlotDurationMillis()
	}
	return b.SlotDurationSchedule.entryForEpoch(epoch).SlotDurationMillis
}

// SlotFractionAt returns the given basis-point fraction of the slot's duration.
func (b *BeaconChainConfig) SlotFractionAt(bp primitives.BP, slot primitives.Slot) time.Duration {
	ms := uint64(bp) * uint64(b.SlotDurationAt(slot)/time.Millisecond) / uint64(BasisPoints)
	return time.Duration(ms) * time.Millisecond
}

// Explicit deadlines apply from the first duration change, before that the *_DUE_BPS fractions stay in force.
func (b *BeaconChainConfig) SlotComponentDurationAt(c SlotComponent, slot primitives.Slot) time.Duration {
	if len(b.SlotDurationSchedule) > 1 && slot >= b.SlotDurationSchedule.startSlot(1, b.SlotsPerEpoch) {
		return time.Duration(b.SlotDurationSchedule.entryFor(slot, b.SlotsPerEpoch).deadlineMillis(c)) * time.Millisecond
	}
	return b.SlotFractionAt(b.slotComponentBPS(c, slot), slot)
}

func (b *BeaconChainConfig) slotComponentBPS(c SlotComponent, slot primitives.Slot) primitives.BP {
	gloas := primitives.Epoch(slot.DivSlot(b.SlotsPerEpoch)) >= b.GloasForkEpoch
	switch c {
	case ProposerReorgCutoff:
		return b.ProposerReorgCutoffBPS
	case AttestationDue:
		if gloas {
			return b.AttestationDueBPSGloas
		}
		return b.AttestationDueBPS
	case AggregateDue:
		if gloas {
			return b.AggregateDueBPSGloas
		}
		return b.AggregateDueBPS
	case SyncMessageDue:
		if gloas {
			return b.SyncMessageDueBPSGloas
		}
		return b.SyncMessageDueBPS
	case ContributionDue:
		if gloas {
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

// RetentionStartEpoch returns the earliest epoch of a retention window of the given epoch count, holding the window's wall-clock length across slot duration changes.
func (b *BeaconChainConfig) RetentionStartEpoch(current, epochs primitives.Epoch) primitives.Epoch {
	if len(b.SlotDurationSchedule) == 0 {
		if current < epochs {
			return 0
		}
		return current - epochs
	}
	windowMs := uint64(epochs) * uint64(b.SlotsPerEpoch) * b.SlotDurationMillis()
	currentStart, err := b.SlotDurationSchedule.SinceGenesis(primitives.Slot(current)*b.SlotsPerEpoch, b.SlotsPerEpoch)
	if err != nil || windowMs > maxMs {
		if current < epochs {
			return 0
		}
		return current - epochs
	}
	window := time.Duration(windowMs) * time.Millisecond
	if currentStart < window {
		return 0
	}
	zero := time.Unix(0, 0)
	windowStartSlot := b.SlotDurationSchedule.SlotAt(zero, zero.Add(currentStart-window), b.SlotsPerEpoch)
	return primitives.Epoch(windowStartSlot / b.SlotsPerEpoch)
}

// Heze's INCLUSION_LIST_DUE_BPS, Prysm carries no Heze configuration yet.
const inclusionListDueBPS primitives.BP = 6667

// GenesisSlotScheduleEntry is the implicit epoch 0 entry when no schedule is configured, carrying the Gloas deadlines the spec inherits.
func (b *BeaconChainConfig) GenesisSlotScheduleEntry() SlotScheduleEntry {
	ms := b.SlotDurationMillis()
	of := func(bp primitives.BP) uint64 { return uint64(bp) * ms / uint64(BasisPoints) }
	return SlotScheduleEntry{
		Epoch:                       0,
		SlotDurationMillis:          ms,
		ProposerReorgCutoffMillis:   of(b.ProposerReorgCutoffBPS),
		AttestationDueMillis:        of(b.AttestationDueBPSGloas),
		AggregateDueMillis:          of(b.AggregateDueBPSGloas),
		SyncMessageDueMillis:        of(b.SyncMessageDueBPSGloas),
		ContributionDueMillis:       of(b.ContributionDueBPSGloas),
		PayloadDueMillis:            of(b.PayloadDueBPS),
		PayloadAttestationDueMillis: of(b.PayloadAttestationDueBPS),
		InclusionListDueMillis:      of(inclusionListDueBPS),
	}
}
