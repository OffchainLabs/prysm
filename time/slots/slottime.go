package slots

import (
	"fmt"
	"math"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	mathutil "github.com/OffchainLabs/prysm/v7/math"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// errOverflow is returned when a slot calculation would overflow.
var errOverflow = errors.New("slot calculation overflows")

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
	if timesPerSlot == 0 {
		return 0
	}
	return params.BeaconConfig().SlotDuration() / time.Duration(timesPerSlot)
}

// MultiplySlotBy multiplies the SECONDS_PER_SLOT configuration
// parameter by a specified number. It returns a value of time.Duration
// in millisecond-based durations.
func MultiplySlotBy(times int64) time.Duration {
	return params.BeaconConfig().SlotDuration() * time.Duration(times)
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
		return slot, errors.Wrap(errOverflow, "epoch start")
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
		return 0, errors.Wrap(errOverflow, "epoch end")
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
	ms, err := slot.SafeMul(params.BeaconConfig().SlotDurationMillis())
	if err != nil {
		return time.Unix(0, 0), fmt.Errorf("slot (%d) is in the far distant future: %w", slot, err)
	}
	return genesis.Add(time.Duration(ms) * time.Millisecond), nil
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
	return primitives.Slot(tm.Sub(genesis) / params.BeaconConfig().SlotDuration())
}

// Duration computes the span of time between two instants, represented as Slots.
func Duration(start, end time.Time) primitives.Slot {
	if end.Before(start) {
		return 0
	}
	return primitives.Slot((end.Sub(start)) / params.BeaconConfig().SlotDuration())
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
	startTime := uint64((slot - slot.ModSlot(slots)).Mul(params.BeaconConfig().SlotDurationMillis())) / 1000
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
	limit := genesis.Add(time.Duration(uint64(s)) * params.BeaconConfig().SlotDuration())
	if timestamp.Before(limit) {
		return 0, fmt.Errorf("could not compute seconds since slot %d start: invalid timestamp, got %s < want %s", s, timestamp, limit)
	}
	return timestamp.Sub(limit), nil
}

// WithinVotingWindow returns whether the current time is within the voting window
// (eg. 4 seconds on mainnet) of the current slot.
func WithinVotingWindow(genesis time.Time, slot primitives.Slot) bool {
	votingWindow := params.BeaconConfig().SlotComponentDuration(params.BeaconConfig().AttestationDueBPS)
	return time.Since(UnsafeStartTime(genesis, slot)) < votingWindow
}

// MaxSafeEpoch gives the largest epoch value that can be safely converted to a slot.
// Note that just dividing max uint64 by slots per epoch is not sufficient,
// because the resulting slot could still be the start of an epoch that would overflow
// in the end slot computation. So we subtract 1 to ensure that the final epoch can always
// have 32 slots.
func MaxSafeEpoch() primitives.Epoch {
	return primitives.Epoch(math.MaxUint64/uint64(params.BeaconConfig().SlotsPerEpoch)) - 1
}

// SafeEpochStartOrMax returns the start slot of the given epoch if it will not overflow,
// otherwise it takes the highest epoch that won't overflow,
// and to introduce a little margin for error, returns the slot beginning the prior epoch.
func SafeEpochStartOrMax(e primitives.Epoch) primitives.Slot {
	// The max value converted to a slot can't be the start of a conceptual epoch,
	// because the first slot of that epoch would be overflow
	// so use the start slot of the epoch right before that value.
	me := MaxSafeEpoch()
	if e > me {
		return UnsafeEpochStart(me)
	}
	return UnsafeEpochStart(e)
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
