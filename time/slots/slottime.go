package slots

import (
	"fmt"
	"math"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	mathutil "github.com/OffchainLabs/prysm/v6/math"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// MaxSlotBuffer specifies the max buffer given to slots from
// incoming objects. (24 mins with mainnet spec)
const MaxSlotBuffer = uint64(1 << 7)

// UnsafeStartTime returns the start time in terms of its unix epoch
// value. This method could panic if the product of slot duration * slot overflows uint64.
// Deprecated: Use StartTime and handle the error.
func UnsafeStartTime(genesis time.Time, slot primitives.Slot) time.Time {
	tm, err := StartTime(genesis, slot)
	if err != nil {
		panic(err) // lint:nopanic -- The panic risk is communicated in the godoc commentary.
	}
	return tm
}

// EpochsSinceGenesis returns the number of epochs since
// the provided genesis time.
func EpochsSinceGenesis(genesis time.Time) primitives.Epoch {
	return primitives.Epoch(CurrentSlot(genesis) / params.BeaconConfig().SlotsPerEpoch)
}

// DivideSlotBy divides the SECONDS_PER_SLOT configuration
// parameter by a specified number. It returns a value of time.Duration
// in milliseconds, useful for dividing values such as 1 second into
// millisecond-based durations.
func DivideSlotBy(timesPerSlot int64) time.Duration {
	return time.Duration(int64(params.BeaconConfig().SecondsPerSlot*1000)/timesPerSlot) * time.Millisecond
}

// MultiplySlotBy multiplies the SECONDS_PER_SLOT configuration
// parameter by a specified number. It returns a value of time.Duration
// in millisecond-based durations.
func MultiplySlotBy(times int64) time.Duration {
	return time.Duration(int64(params.BeaconConfig().SecondsPerSlot)*times) * time.Second
}

// AbsoluteValueSlotDifference between two slots.
func AbsoluteValueSlotDifference(x, y primitives.Slot) uint64 {
	if x > y {
		return uint64(x.SubSlot(y))
	}
	return uint64(y.SubSlot(x))
}

// ToEpoch returns the epoch number of the input slot.
//
// Spec pseudocode definition:
//
//	def compute_epoch_at_slot(slot: Slot) -> Epoch:
//	  """
//	  Return the epoch number at ``slot``.
//	  """
//	  return Epoch(slot // SLOTS_PER_EPOCH)
func ToEpoch(slot primitives.Slot) primitives.Epoch {
	return primitives.Epoch(slot.DivSlot(params.BeaconConfig().SlotsPerEpoch))
}

// ToForkVersion translates a slot into it's corresponding version.
func ToForkVersion(slot primitives.Slot) int {
	epoch := ToEpoch(slot)
	switch {
	case epoch >= params.BeaconConfig().FuluForkEpoch:
		return version.Fulu
	case epoch >= params.BeaconConfig().ElectraForkEpoch:
		return version.Electra
	case epoch >= params.BeaconConfig().DenebForkEpoch:
		return version.Deneb
	case epoch >= params.BeaconConfig().CapellaForkEpoch:
		return version.Capella
	case epoch >= params.BeaconConfig().BellatrixForkEpoch:
		return version.Bellatrix
	case epoch >= params.BeaconConfig().AltairForkEpoch:
		return version.Altair
	default:
		return version.Phase0
	}
}

// EpochStart returns the first slot number of the
// current epoch.
//
// Spec pseudocode definition:
//
//	def compute_start_slot_at_epoch(epoch: Epoch) -> Slot:
//	  """
//	  Return the start slot of ``epoch``.
//	  """
//	  return Slot(epoch * SLOTS_PER_EPOCH)
func EpochStart(epoch primitives.Epoch) (primitives.Slot, error) {
	slot, err := params.BeaconConfig().SlotsPerEpoch.SafeMul(uint64(epoch))
	if err != nil {
		return slot, errors.Errorf("start slot calculation overflows: %v", err)
	}
	return slot, nil
}

// UnsafeEpochStart is a version of EpochStart that panics if there is an overflow. It can be safely used by code
// that first guarantees epoch <= MaxSafeEpoch.
func UnsafeEpochStart(epoch primitives.Epoch) primitives.Slot {
	es, err := EpochStart(epoch)
	if err != nil {
		panic(err) // lint:nopanic -- Unsafe is implied and communicated in the godoc commentary.
	}
	return es
}

// EpochEnd returns the last slot number of the
// current epoch.
func EpochEnd(epoch primitives.Epoch) (primitives.Slot, error) {
	if epoch == math.MaxUint64 {
		return 0, errors.New("start slot calculation overflows")
	}
	slot, err := EpochStart(epoch + 1)
	if err != nil {
		return 0, err
	}
	return slot - 1, nil
}

// IsEpochStart returns true if the given slot number is an epoch starting slot
// number.
func IsEpochStart(slot primitives.Slot) bool {
	return slot%params.BeaconConfig().SlotsPerEpoch == 0
}

// IsEpochEnd returns true if the given slot number is an epoch ending slot
// number.
func IsEpochEnd(slot primitives.Slot) bool {
	return IsEpochStart(slot + 1)
}

// SinceEpochStarts returns number of slots since the start of the epoch.
func SinceEpochStarts(slot primitives.Slot) primitives.Slot {
	return slot % params.BeaconConfig().SlotsPerEpoch
}

// VerifyTime validates the input slot is not from the future.
func VerifyTime(genesis time.Time, slot primitives.Slot, timeTolerance time.Duration) error {
	slotTime, err := StartTime(genesis, slot)
	if err != nil {
		return err
	}

	// Defensive check to ensure unreasonable slots are rejected
	// straight away.
	if err := ValidateClock(slot, genesis); err != nil {
		return err
	}

	currentTime := prysmTime.Now()
	diff := slotTime.Sub(currentTime)

	if diff > timeTolerance {
		return fmt.Errorf("could not process slot from the future, slot time %s > current time %s", slotTime, currentTime)
	}
	return nil
}

// StartTime takes the given slot and genesis time to determine the start time of the slot.
// This method returns an error if the product of the slot duration * slot overflows int64.
func StartTime(genesis time.Time, slot primitives.Slot) (time.Time, error) {
	_, err := slot.SafeMul(params.BeaconConfig().SecondsPerSlot)
	if err != nil {
		return time.Unix(0, 0), fmt.Errorf("slot (%d) is in the far distant future: %w", slot, err)
	}
	sd := time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Duration(slot)
	return genesis.Add(sd), nil
}

// CurrentSlot returns the current slot as determined by the local clock and
// provided genesis time.
func CurrentSlot(genesis time.Time) primitives.Slot {
	return At(genesis, time.Now())
}

// At returns the slot at the given time.
func At(genesis, tm time.Time) primitives.Slot {
	if tm.Before(genesis) {
		return 0
	}
	return primitives.Slot(tm.Sub(genesis) / time.Second / time.Duration(params.BeaconConfig().SecondsPerSlot))
}

// Duration computes the span of time between two instants, represented as Slots.
func Duration(start, end time.Time) primitives.Slot {
	if end.Before(start) {
		return 0
	}
	return primitives.Slot(uint64(end.Unix()-start.Unix()) / params.BeaconConfig().SecondsPerSlot)
}

// ValidateClock validates a provided slot against the local
// clock to ensure slots that are unreasonable are returned with
// an error.
func ValidateClock(slot primitives.Slot, genesis time.Time) error {
	maxPossibleSlot := CurrentSlot(genesis).Add(MaxSlotBuffer)
	// Defensive check to ensure that we only process slots up to a hard limit
	// from our local clock.
	if slot > maxPossibleSlot {
		return fmt.Errorf("slot %d > %d which exceeds max allowed value relative to the local clock", slot, maxPossibleSlot)
	}
	return nil
}

// RoundUpToNearestEpoch rounds up the provided slot value to the nearest epoch.
func RoundUpToNearestEpoch(slot primitives.Slot) primitives.Slot {
	if slot%params.BeaconConfig().SlotsPerEpoch != 0 {
		slot -= slot % params.BeaconConfig().SlotsPerEpoch
		slot += params.BeaconConfig().SlotsPerEpoch
	}
	return slot
}

// VotingPeriodStartTime returns the current voting period's start time
// depending on the provided genesis and current slot.
func VotingPeriodStartTime(genesis uint64, slot primitives.Slot) uint64 {
	slots := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerEth1VotingPeriod))
	startTime := uint64((slot - slot.ModSlot(slots)).Mul(params.BeaconConfig().SecondsPerSlot))
	return genesis + startTime
}

// PrevSlot returns previous slot, with an exception in slot 0 to prevent underflow.
func PrevSlot(slot primitives.Slot) primitives.Slot {
	if slot > 0 {
		return slot.Sub(1)
	}
	return 0
}

// SyncCommitteePeriod returns the sync committee period of input epoch `e`.
//
// Spec code:
// def compute_sync_committee_period(epoch: Epoch) -> uint64:
//
//	return epoch // EPOCHS_PER_SYNC_COMMITTEE_PERIOD
func SyncCommitteePeriod(e primitives.Epoch) uint64 {
	return uint64(e / params.BeaconConfig().EpochsPerSyncCommitteePeriod)
}

// SyncCommitteePeriodStartEpoch returns the start epoch of a sync committee period.
func SyncCommitteePeriodStartEpoch(e primitives.Epoch) (primitives.Epoch, error) {
	// Overflow is impossible here because of division of `EPOCHS_PER_SYNC_COMMITTEE_PERIOD`.
	startEpoch, err := mathutil.Mul64(SyncCommitteePeriod(e), uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod))
	if err != nil {
		return 0, err
	}
	return primitives.Epoch(startEpoch), nil
}

// SinceSlotStart returns the amount of time elapsed since the
// given slot start time. This method returns an error if the timestamp happens
// before the given slot start time.
func SinceSlotStart(s primitives.Slot, genesis time.Time, timestamp time.Time) (time.Duration, error) {
	limit := genesis.Add(time.Duration(uint64(s)*params.BeaconConfig().SecondsPerSlot) * time.Second)
	if timestamp.Before(limit) {
		return 0, fmt.Errorf("could not compute seconds since slot %d start: invalid timestamp, got %s < want %s", s, timestamp, limit)
	}
	return timestamp.Sub(limit), nil
}

// WithinVotingWindow returns whether the current time is within the voting window
// (eg. 4 seconds on mainnet) of the current slot.
func WithinVotingWindow(genesis time.Time, slot primitives.Slot) bool {
	votingWindow := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	return time.Since(UnsafeStartTime(genesis, slot)) < time.Duration(votingWindow)*time.Second
}

// MaxSafeEpoch gives the largest epoch value that can be safely converted to a slot.
func MaxSafeEpoch() primitives.Epoch {
	return primitives.Epoch(math.MaxUint64 / uint64(params.BeaconConfig().SlotsPerEpoch))
}

// SecondsUntilNextEpochStart returns how many seconds until the next Epoch start from the current time and slot
func SecondsUntilNextEpochStart(genesis time.Time) (uint64, error) {
	currentSlot := CurrentSlot(genesis)
	firstSlotOfNextEpoch, err := EpochStart(ToEpoch(currentSlot) + 1)
	if err != nil {
		return 0, err
	}
	nextEpochStartTime, err := StartTime(genesis, firstSlotOfNextEpoch)
	if err != nil {
		return 0, err
	}
	es := nextEpochStartTime.Unix()
	n := time.Now().Unix()
	waitTime := uint64(es - n)
	log.WithFields(logrus.Fields{
		"current_slot":          currentSlot,
		"next_epoch_start_slot": firstSlotOfNextEpoch,
		"is_epoch_start":        IsEpochStart(currentSlot),
	}).Debugf("%d seconds until next epoch", waitTime)
	return waitTime, nil
}

// ComponentDuration calculates the duration of a slot component in milliseconds.
func ComponentDuration(component primitives.BP) time.Duration {
	ms := (component * params.SlotBP()) / params.BasisPoints
	return time.Duration(ms) * time.Millisecond
}
