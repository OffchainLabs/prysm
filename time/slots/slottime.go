package slots

import (
	"fmt"
	"math"
	"math/bits"
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

// startFromTime returns the slot start in terms of genesis time.Time
func startFromTime(genesis time.Time, slot primitives.Slot) time.Time {
	cfg := params.BeaconConfig()
	epoch := ToEpoch(slot)

	var duration time.Duration

	if epoch >= cfg.FuluForkEpoch {
		upgradeSlot := primitives.Slot(cfg.FuluForkEpoch) * cfg.SlotsPerEpoch
		adjustedSlot := slot.SubSlot(upgradeSlot)

		duration += time.Duration(adjustedSlot.Mul(cfg.DeprecatedSecondsPerSlotXYZ)) * time.Second
		duration += time.Duration(upgradeSlot.Mul(cfg.DeprecatedSecondsPerSlot)) * time.Second
	} else {
		duration += time.Duration(slot.Mul(cfg.DeprecatedSecondsPerSlot)) * time.Second
	}

	return genesis.Add(duration)
}

// StartTime returns the start time in terms of its unix epoch
// value.
func StartTime(genesis uint64, slot primitives.Slot) time.Time {
	genesisTime := time.Unix(int64(genesis), 0) // lint:ignore uintcast -- Genesis timestamp will not exceed int64 in your lifetime.
	return startFromTime(genesisTime, slot)
}

// SinceGenesis returns the number of slots since
// the provided genesis time.
func SinceGenesis(genesis time.Time) primitives.Slot {
	now := prysmTime.Now()
	if genesis.After(now) {
		return 0
	}

	cfg := params.BeaconConfig()
	elapsedSeconds := uint64(now.Sub(genesis).Seconds())

	upgradeSlot := primitives.Slot(cfg.FuluForkEpoch) * cfg.SlotsPerEpoch
	upgradeTime := uint64(upgradeSlot) * cfg.DeprecatedSecondsPerSlot

	if elapsedSeconds <= upgradeTime {
		return primitives.Slot(elapsedSeconds / cfg.DeprecatedSecondsPerSlot)
	}

	postUpgradeElapsed := elapsedSeconds - upgradeTime
	postUpgradeSlots := primitives.Slot(postUpgradeElapsed / cfg.DeprecatedSecondsPerSlotXYZ)

	return upgradeSlot + postUpgradeSlots
}

// EpochsSinceGenesis returns the number of epochs since
// the provided genesis time.
func EpochsSinceGenesis(genesis time.Time) primitives.Epoch {
	return primitives.Epoch(SinceGenesis(genesis) / params.BeaconConfig().SlotsPerEpoch)
}

// DivideSlotBy divides the SECONDS_PER_SLOT configuration
// parameter by a specified number. It returns a value of time.Duration
// in milliseconds, useful for dividing values such as 1 second into
// millisecond-based durations.
func DivideSlotBy(timesPerSlot int64, slot primitives.Slot) time.Duration {
	if ToEpoch(slot) >= params.BeaconConfig().FuluForkEpoch {
		return time.Duration(int64(params.BeaconConfig().DeprecatedSecondsPerSlotXYZ*1000)/timesPerSlot) * time.Millisecond
	}
	return time.Duration(int64(params.BeaconConfig().DeprecatedSecondsPerSlot*1000)/timesPerSlot) * time.Millisecond
}

// MultiplySlotBy multiplies the SECONDS_PER_SLOT configuration
// parameter by a specified number. It returns a value of time.Duration
// in millisecond-based durations.
func MultiplySlotBy(times int64, slot primitives.Slot) time.Duration {
	if ToEpoch(slot) >= params.BeaconConfig().FuluForkEpoch {
		return time.Duration(int64(params.BeaconConfig().DeprecatedSecondsPerSlotXYZ)*times) * time.Second
	}
	return time.Duration(int64(params.BeaconConfig().DeprecatedSecondsPerSlot)*times) * time.Second
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
func VerifyTime(genesisTime uint64, slot primitives.Slot, timeTolerance time.Duration) error {
	slotTime, err := ToTime(genesisTime, slot)
	if err != nil {
		return err
	}

	// Defensive check to ensure unreasonable slots are rejected
	// straight away.
	if err := ValidateClock(slot, genesisTime); err != nil {
		return err
	}

	currentTime := prysmTime.Now()
	diff := slotTime.Sub(currentTime)

	if diff > timeTolerance {
		return fmt.Errorf("could not process slot from the future, slot time %s > current time %s", slotTime, currentTime)
	}
	return nil
}

func ToTime(genesisTimeSec uint64, slot primitives.Slot) (time.Time, error) {
	cfg := params.BeaconConfig()
	upgradeSlot := primitives.Slot(cfg.FuluForkEpoch) * cfg.SlotsPerEpoch

	var timeSinceGenesis primitives.Slot
	var err error

	if slot < upgradeSlot {
		timeSinceGenesis, err = slot.SafeMul(cfg.DeprecatedSecondsPerSlot)
		if err != nil {
			return time.Time{}, fmt.Errorf("slot (%d) is in the far distant future: %w", slot, err)
		}
	} else {
		adjustedSlot := slot - upgradeSlot

		preUpgradeTime, err := upgradeSlot.SafeMul(cfg.DeprecatedSecondsPerSlot)
		if err != nil {
			return time.Time{}, fmt.Errorf("slot (%d) pre-upgrade mul overflow: %w", slot, err)
		}

		postUpgradeTime, err := adjustedSlot.SafeMul(cfg.DeprecatedSecondsPerSlotXYZ)
		if err != nil {
			return time.Time{}, fmt.Errorf("slot (%d) post-upgrade mul overflow: %w", slot, err)
		}

		timeSinceGenesis, err = preUpgradeTime.SafeAddSlot(postUpgradeTime)
		if err != nil {
			return time.Time{}, fmt.Errorf("slot (%d) addition overflow: %w", slot, err)
		}
	}

	absoluteTime, err := timeSinceGenesis.SafeAdd(genesisTimeSec)
	if err != nil {
		return time.Time{}, fmt.Errorf("slot (%d) genesis time addition overflow: %w", slot, err)
	}

	if bits.Len64(uint64(absoluteTime)) >= 63 {
		return time.Time{}, fmt.Errorf("slot (%d) resulting timestamp overflows int64: %d", slot, absoluteTime)
	}

	return time.Unix(0, 0).Add(time.Duration(absoluteTime) * time.Second), nil
}

// BeginsAt computes the timestamp where the given slot begins, relative to the genesis timestamp.
func BeginsAt(slot primitives.Slot, genesis time.Time) time.Time {
	cfg := params.BeaconConfig()
	upgradeSlot := primitives.Slot(cfg.FuluForkEpoch) * cfg.SlotsPerEpoch

	var seconds uint64
	if slot < upgradeSlot {
		seconds = uint64(slot) * cfg.DeprecatedSecondsPerSlot
	} else {
		preUpgrade := uint64(upgradeSlot) * cfg.DeprecatedSecondsPerSlot
		postUpgrade := uint64(slot-upgradeSlot) * cfg.DeprecatedSecondsPerSlotXYZ
		seconds = preUpgrade + postUpgrade
	}

	return genesis.Add(time.Duration(seconds) * time.Second)
}

// Since computes the number of time slots that have occurred since the given timestamp.
func Since(time time.Time) primitives.Slot {
	return CurrentSlot(uint64(time.Unix()))
}

func CurrentSlot(genesisTimeSec uint64) primitives.Slot {
	now := uint64(prysmTime.Now().Unix())
	if now < genesisTimeSec {
		return 0
	}

	cfg := params.BeaconConfig()
	elapsed := now - genesisTimeSec

	upgradeSlot := primitives.Slot(cfg.FuluForkEpoch) * cfg.SlotsPerEpoch
	upgradeTime := uint64(upgradeSlot) * cfg.DeprecatedSecondsPerSlot

	if elapsed <= upgradeTime {
		return primitives.Slot(elapsed / cfg.DeprecatedSecondsPerSlot)
	}

	postUpgradeElapsed := elapsed - upgradeTime
	postUpgradeSlots := primitives.Slot(postUpgradeElapsed / cfg.DeprecatedSecondsPerSlotXYZ)

	return upgradeSlot + postUpgradeSlots
}

// Duration computes the span of time between two instants, represented as Slots.
func Duration(start, end time.Time) primitives.Slot {
	if end.Before(start) {
		return 0
	}

	cfg := params.BeaconConfig()
	elapsed := uint64(end.Sub(start).Seconds())

	upgradeSlot := primitives.Slot(cfg.FuluForkEpoch) * cfg.SlotsPerEpoch
	upgradeTime := uint64(upgradeSlot) * cfg.DeprecatedSecondsPerSlot

	if elapsed <= upgradeTime {
		return primitives.Slot(elapsed / cfg.DeprecatedSecondsPerSlot)
	}

	postUpgradeElapsed := elapsed - upgradeTime
	postUpgradeSlots := primitives.Slot(postUpgradeElapsed / cfg.DeprecatedSecondsPerSlotXYZ)

	return upgradeSlot + postUpgradeSlots
}

// ValidateClock validates a provided slot against the local
// clock to ensure slots that are unreasonable are returned with
// an error.
func ValidateClock(slot primitives.Slot, genesisTimeSec uint64) error {
	maxPossibleSlot := CurrentSlot(genesisTimeSec).Add(MaxSlotBuffer)
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
	startTime := uint64((slot - slot.ModSlot(slots)).Mul(params.BeaconConfig().DeprecatedSecondsPerSlot)) // Can ignore this as it's used before Pectra. See EIP-6110
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

// SecondsSinceSlotStart returns the number of seconds elapsed since the
// given slot start time
func SecondsSinceSlotStart(s primitives.Slot, genesisTime, timeStamp uint64) (uint64, error) {
	cfg := params.BeaconConfig()
	upgradeSlot := primitives.Slot(cfg.FuluForkEpoch) * cfg.SlotsPerEpoch

	var slotStart uint64
	if s < upgradeSlot {
		slotStart = genesisTime + uint64(s)*cfg.DeprecatedSecondsPerSlot
	} else {
		preUpgrade := uint64(upgradeSlot) * cfg.DeprecatedSecondsPerSlot
		postUpgrade := uint64(s-upgradeSlot) * cfg.DeprecatedSecondsPerSlotXYZ
		slotStart = genesisTime + preUpgrade + postUpgrade
	}

	if timeStamp < slotStart {
		return 0, fmt.Errorf("could not compute seconds since slot %d start: invalid timestamp, got %d < want %d", s, timeStamp, slotStart)
	}

	return timeStamp - slotStart, nil
}

// TimeIntoSlot returns the time duration elapsed between the current time and
// the start of the current slot
func TimeIntoSlot(genesisTime uint64) time.Duration {
	return time.Since(StartTime(genesisTime, CurrentSlot(genesisTime)))
}

// WithinVotingWindow returns whether the current time is within the voting window
// (eg. 4 seconds on mainnet) of the current slot.
func WithinVotingWindow(genesisTime uint64, slot primitives.Slot) bool {
	e := ToEpoch(slot)
	if e >= params.BeaconConfig().FuluForkEpoch {
		votingWindow := params.BeaconConfig().DeprecatedSecondsPerSlotXYZ / params.BeaconConfig().IntervalsPerSlot
		return time.Since(StartTime(genesisTime, slot)) < time.Duration(votingWindow)*time.Second
	}
	votingWindow := params.BeaconConfig().DeprecatedSecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	return time.Since(StartTime(genesisTime, slot)) < time.Duration(votingWindow)*time.Second
}

// MaxSafeEpoch gives the largest epoch value that can be safely converted to a slot.
func MaxSafeEpoch() primitives.Epoch {
	return primitives.Epoch(math.MaxUint64 / uint64(params.BeaconConfig().SlotsPerEpoch))
}

// SecondsUntilNextEpochStart returns how many seconds until the next Epoch start from the current time and slot
func SecondsUntilNextEpochStart(genesisTimeSec uint64) (uint64, error) {
	currentSlot := CurrentSlot(genesisTimeSec)
	firstSlotOfNextEpoch, err := EpochStart(ToEpoch(currentSlot) + 1)
	if err != nil {
		return 0, err
	}
	nextEpochStartTime, err := ToTime(genesisTimeSec, firstSlotOfNextEpoch)
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

func CurrentSecondsPerSlot(genesisTimeSec uint64) uint64 {
	now := uint64(prysmTime.Now().Unix())
	if now < genesisTimeSec {
		return 0
	}

	cfg := params.BeaconConfig()
	elapsed := now - genesisTimeSec

	upgradeSlot := primitives.Slot(cfg.FuluForkEpoch) * cfg.SlotsPerEpoch
	upgradeTime := uint64(upgradeSlot) * cfg.DeprecatedSecondsPerSlot

	if elapsed <= upgradeTime {
		return cfg.DeprecatedSecondsPerSlot
	}
	return cfg.DeprecatedSecondsPerSlotXYZ
}
