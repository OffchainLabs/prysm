package helpers_test

import (
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestIsCurrentEpochSyncCommittee_UsingCache(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	r := [32]byte{'a'}
	require.NoError(t, err, helpers.SyncCommitteeCache().UpdatePositionsInCommittee(r, state))

	ok, err := helpers.IsCurrentPeriodSyncCommittee(state, 0)
	require.NoError(t, err)
	require.Equal(t, true, ok)
}

func TestIsCurrentEpochSyncCommittee_UsingCommittee(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	ok, err := helpers.IsCurrentPeriodSyncCommittee(state, 0)
	require.NoError(t, err)
	require.Equal(t, true, ok)
}

func TestIsCurrentEpochSyncCommittee_DoesNotExist(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	ok, err := helpers.IsCurrentPeriodSyncCommittee(state, 12390192)
	require.ErrorContains(t, "validator index 12390192 does not exist", err)
	require.Equal(t, false, ok)
}

func TestIsNextEpochSyncCommittee_UsingCache(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	r := [32]byte{'a'}
	require.NoError(t, err, helpers.SyncCommitteeCache().UpdatePositionsInCommittee(r, state))

	ok, err := helpers.IsNextPeriodSyncCommittee(state, 0)
	require.NoError(t, err)
	require.Equal(t, true, ok)
}

func TestIsNextEpochSyncCommittee_UsingCommittee(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	ok, err := helpers.IsNextPeriodSyncCommittee(state, 0)
	require.NoError(t, err)
	require.Equal(t, true, ok)
}

func TestIsNextEpochSyncCommittee_DoesNotExist(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	ok, err := helpers.IsNextPeriodSyncCommittee(state, 120391029)
	require.ErrorContains(t, "validator index 120391029 does not exist", err)
	require.Equal(t, false, ok)
}

func TestCurrentEpochSyncSubcommitteeIndices_UsingCache(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	r := [32]byte{'a'}
	require.NoError(t, err, helpers.SyncCommitteeCache().UpdatePositionsInCommittee(r, state))

	index, err := helpers.CurrentPeriodSyncSubcommitteeIndices(state, 0)
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, index)
}

func TestCurrentEpochSyncSubcommitteeIndices_UsingCommittee(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))
	root, err := helpers.SyncPeriodBoundaryRoot(state)
	require.NoError(t, err)

	// Test that cache was empty.
	_, err = helpers.SyncCommitteeCache().CurrentPeriodIndexPosition(root, 0)
	require.Equal(t, cache.ErrNonExistingSyncCommitteeKey, err)

	// Test that helper can retrieve the index given empty cache.
	index, err := helpers.CurrentPeriodSyncSubcommitteeIndices(state, 0)
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, index)

	// Test that cache was able to fill on miss.
	time.Sleep(100 * time.Millisecond)
	index, err = helpers.SyncCommitteeCache().CurrentPeriodIndexPosition(root, 0)
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, index)
}

func TestCurrentEpochSyncSubcommitteeIndices_DoesNotExist(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	index, err := helpers.CurrentPeriodSyncSubcommitteeIndices(state, 129301923)
	require.ErrorContains(t, "validator index 129301923 does not exist", err)
	require.DeepEqual(t, []primitives.CommitteeIndex(nil), index)
}

func TestNextEpochSyncSubcommitteeIndices_UsingCache(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	r := [32]byte{'a'}
	require.NoError(t, err, helpers.SyncCommitteeCache().UpdatePositionsInCommittee(r, state))

	index, err := helpers.NextPeriodSyncSubcommitteeIndices(state, 0)
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, index)
}

func TestNextEpochSyncSubcommitteeIndices_UsingCommittee(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	index, err := helpers.NextPeriodSyncSubcommitteeIndices(state, 0)
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, index)
}

func TestNextEpochSyncSubcommitteeIndices_DoesNotExist(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	index, err := helpers.NextPeriodSyncSubcommitteeIndices(state, 21093019)
	require.ErrorContains(t, "validator index 21093019 does not exist", err)
	require.DeepEqual(t, []primitives.CommitteeIndex(nil), index)
}

func TestUpdateSyncCommitteeCache_BadSlot(t *testing.T) {
	helpers.ClearCache()

	state, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot: 1,
	})
	require.NoError(t, err)
	err = helpers.UpdateSyncCommitteeCache(state)
	require.ErrorContains(t, "not at the end of the epoch to update cache", err)

	state, err = state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot: params.BeaconConfig().SlotsPerEpoch - 1,
	})
	require.NoError(t, err)
	err = helpers.UpdateSyncCommitteeCache(state)
	require.ErrorContains(t, "not at sync committee period boundary to update cache", err)
}

func TestUpdateSyncCommitteeCache_BadRoot(t *testing.T) {
	helpers.ClearCache()

	state, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot:              primitives.Slot(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*params.BeaconConfig().SlotsPerEpoch - 1,
		LatestBlockHeader: &ethpb.BeaconBlockHeader{StateRoot: params.BeaconConfig().ZeroHash[:]},
	})
	require.NoError(t, err)
	err = helpers.UpdateSyncCommitteeCache(state)
	require.ErrorContains(t, "zero hash state root can't be used to update cache", err)
}

func TestIsCurrentEpochSyncCommittee_SameBlockRoot(t *testing.T) {
	helpers.ClearCache()

	validators := make([]*ethpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &ethpb.SyncCommittee{
		AggregatePubkey: bytesutil.PadTo([]byte{}, params.BeaconConfig().BLSPubkeyLength),
	}
	for i := 0; i < len(validators); i++ {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &ethpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	for i := range blockRoots {
		blockRoots[i] = make([]byte, 32)
	}
	state, err := state_native.InitializeFromProtoAltair(&ethpb.BeaconStateAltair{
		Validators: validators,
		BlockRoots: blockRoots,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	comIdxs, err := helpers.CurrentPeriodSyncSubcommitteeIndices(state, 200)
	require.NoError(t, err)

	wantedSlot := params.BeaconConfig().EpochsPerSyncCommitteePeriod.Mul(uint64(params.BeaconConfig().SlotsPerEpoch))
	assert.NoError(t, state.SetSlot(primitives.Slot(wantedSlot)))
	syncCommittee, err = state.CurrentSyncCommittee()
	assert.NoError(t, err)
	rand.Shuffle(len(syncCommittee.Pubkeys), func(i, j int) {
		syncCommittee.Pubkeys[i], syncCommittee.Pubkeys[j] = syncCommittee.Pubkeys[j], syncCommittee.Pubkeys[i]
	})
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	newIdxs, err := helpers.CurrentPeriodSyncSubcommitteeIndices(state, 200)
	require.NoError(t, err)
	require.DeepNotEqual(t, comIdxs, newIdxs)
}
