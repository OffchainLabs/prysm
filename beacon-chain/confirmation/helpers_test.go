package confirmation

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestIsFullValidatorSetCovered(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)

	// Full epoch covered: slot 0 to slot 31 (32 slots = 1 epoch)
	require.Equal(t, true, IsFullValidatorSetCovered(0, spe-1))

	// Full epoch covered: slot 5 to slot 36 (covers all of epoch 1: slots 32-36+)
	// Actually need slots 5..36 to cover a full epoch. Epoch 0 is 0-31, epoch 1 is 32-63.
	// Slots 5..36 covers epoch 0 slots 5-31 (27 slots) + epoch 1 slots 32-36 (5 slots).
	// start_full_epoch = epoch_at(5 + 31) = epoch_at(36) = 1
	// end_full_epoch = epoch_at(37) = 1
	// 1 < 1 = false. Not covered.
	require.Equal(t, false, IsFullValidatorSetCovered(5, 36))

	// Slots 5..63 covers full epoch 1 (32-63)
	// start_full_epoch = epoch_at(5 + 31) = epoch_at(36) = 1
	// end_full_epoch = epoch_at(64) = 2
	// 1 < 2 = true
	require.Equal(t, true, IsFullValidatorSetCovered(5, spe*2-1))

	// Single slot: never covers
	require.Equal(t, false, IsFullValidatorSetCovered(0, 0))

	// Almost full epoch: 31 slots
	require.Equal(t, false, IsFullValidatorSetCovered(0, spe-2))
}

func TestAdjustCommitteeWeightEstimate(t *testing.T) {
	// 1000 -> ceil(1000/1000) * 1005 = 1005
	require.Equal(t, uint64(1005), AdjustCommitteeWeightEstimate(1000))

	// 32000 -> ceil(32000/1000) * 1005 = 32160
	require.Equal(t, uint64(32160), AdjustCommitteeWeightEstimate(32000))

	// 0 -> 0
	require.Equal(t, uint64(0), AdjustCommitteeWeightEstimate(0))

	// 999 -> ceil(999/1000) * 1005 = 1005 (spec rounds the estimate up)
	require.Equal(t, uint64(1005), AdjustCommitteeWeightEstimate(999))
}

func TestEstimateCommitteeWeightBetweenSlots(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	totalBalance := uint64(320_000) // 10000 per slot with 32 slots/epoch
	committeeWeight := totalBalance / uint64(spe)

	// start > end: returns 0
	require.Equal(t, uint64(0), EstimateCommitteeWeightBetweenSlots(totalBalance, 5, 3))

	// Full epoch: returns total active balance
	require.Equal(t, totalBalance, EstimateCommitteeWeightBetweenSlots(totalBalance, 0, spe-1))

	// Same epoch, single slot
	require.Equal(t, committeeWeight, EstimateCommitteeWeightBetweenSlots(totalBalance, 0, 0))

	// Same epoch, 3 slots
	require.Equal(t, committeeWeight*3, EstimateCommitteeWeightBetweenSlots(totalBalance, 1, 3))

	// Same epoch, full epoch (all 32 slots of epoch 0)
	// This hits is_full_validator_set_covered = true, returns total
	require.Equal(t, totalBalance, EstimateCommitteeWeightBetweenSlots(totalBalance, 0, spe-1))

	// Cross-epoch but not full: last 2 slots of epoch 0 + first 2 of epoch 1
	// startSlot=30, endSlot=33 (with spe=32)
	// committeeWeight = 320000 / 32 = 10000
	// numSlotsInEndEpoch = SinceEpochStarts(33) + 1 = 2
	// remainingSlotsInEndEpoch = 32 - 2 = 30
	// numSlotsInStartEpoch = 32 - SinceEpochStarts(30) = 32 - 30 = 2
	// startEpochWeight = 10000 * 2 = 20000
	// endEpochWeight = 10000 * 2 = 20000
	// startEpochWeightProRated = 20000 / 32 * 30 = 18750
	// adjust(18750 + 20000) = adjust(38750) = ceil(38750/1000) * 1005 = 39 * 1005 = 39195
	startSlot := spe - 2
	endSlot := spe + 1
	result := EstimateCommitteeWeightBetweenSlots(totalBalance, startSlot, endSlot)
	require.Equal(t, uint64(39195), result)

	// Cross-epoch: last slot of epoch 0 + first 5 of epoch 1
	// startSlot=31, endSlot=36 (with spe=32)
	// numSlotsInEndEpoch = SinceEpochStarts(36) + 1 = 4 + 1 = 5
	// remainingSlotsInEndEpoch = 32 - 5 = 27
	// numSlotsInStartEpoch = 32 - SinceEpochStarts(31) = 32 - 31 = 1
	// startEpochWeight = 10000 * 1 = 10000
	// endEpochWeight = 10000 * 5 = 50000
	// startEpochWeightProRated = 10000 / 32 * 27 = 312 * 27 = 8424
	// adjust(8424 + 50000) = adjust(58424) = ceil(58424/1000) * 1005 = 59 * 1005 = 59295
	result = EstimateCommitteeWeightBetweenSlots(totalBalance, spe-1, spe+4)
	require.Equal(t, uint64(59295), result)
}

func TestComputeAdversarialWeight(t *testing.T) {
	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	totalBalance := uint64(320_000)

	// Full epoch, no equivocations: max_adversarial = total * 25 / 100 = 80000
	result := ComputeAdversarialWeight(totalBalance, 0, 0, spe-1)
	require.Equal(t, uint64(80000), result)

	// With equivocation: subtract equivocation score
	result = ComputeAdversarialWeight(totalBalance, 10000, 0, spe-1)
	require.Equal(t, uint64(70000), result)

	// Equivocation exceeds adversarial: returns 0
	result = ComputeAdversarialWeight(totalBalance, 100000, 0, spe-1)
	require.Equal(t, uint64(0), result)

	// start > end: max_weight = 0, so adversarial = 0
	result = ComputeAdversarialWeight(totalBalance, 0, 5, 3)
	require.Equal(t, uint64(0), result)
}

func TestComputeProposerScore(t *testing.T) {
	spe := uint64(params.BeaconConfig().SlotsPerEpoch)
	boost := params.BeaconConfig().ProposerScoreBoost
	totalBalance := uint64(320_000)

	// committee_weight = 320000 / 32 = 10000
	// proposer_score = 10000 * 40 / 100 = 4000
	expected := (totalBalance / spe) * boost / 100
	require.Equal(t, expected, ComputeProposerScore(totalBalance))

	// Zero balance
	require.Equal(t, uint64(0), ComputeProposerScore(0))
}
