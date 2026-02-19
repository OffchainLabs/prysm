package core

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestAttesterDuties(t *testing.T) {
	helpers.ClearCache()

	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err)

	s := &Service{}

	t.Run("single validator", func(t *testing.T) {
		duties, rpcErr := s.AttesterDuties(t.Context(), bs, 0, []primitives.ValidatorIndex{0})
		require.Equal(t, (*RpcError)(nil), rpcErr)
		require.Equal(t, 1, len(duties))
		duty := duties[0]
		assert.Equal(t, primitives.ValidatorIndex(0), duty.ValidatorIndex)
		assert.NotEqual(t, uint64(0), duty.CommitteeLength)
		assert.NotEqual(t, uint64(0), duty.CommitteesAtSlot)
	})

	t.Run("multiple validators", func(t *testing.T) {
		indices := []primitives.ValidatorIndex{0, 1, 2}
		duties, rpcErr := s.AttesterDuties(t.Context(), bs, 0, indices)
		require.Equal(t, (*RpcError)(nil), rpcErr)
		require.Equal(t, 3, len(duties))
	})

	t.Run("zero pubkey returns error", func(t *testing.T) {
		// Index far beyond the validator count should have a zero pubkey.
		badIndex := primitives.ValidatorIndex(depChainStart + 100)
		_, rpcErr := s.AttesterDuties(t.Context(), bs, 0, []primitives.ValidatorIndex{badIndex})
		require.NotNil(t, rpcErr)
		require.Equal(t, ErrorReason(BadRequest), rpcErr.Reason)
	})
}

func TestProposerDuties(t *testing.T) {
	helpers.ClearCache()

	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err)

	s := &Service{}

	t.Run("basic OK", func(t *testing.T) {
		duties, rpcErr := s.ProposerDuties(t.Context(), bs, 0)
		require.Equal(t, (*RpcError)(nil), rpcErr)
		// Epoch 0 has SlotsPerEpoch slots, but slot 0 is skipped for proposer, so expect SlotsPerEpoch-1 duties.
		require.Equal(t, int(params.BeaconConfig().SlotsPerEpoch-1), len(duties))
	})

	t.Run("sorted by slot", func(t *testing.T) {
		duties, rpcErr := s.ProposerDuties(t.Context(), bs, 0)
		require.Equal(t, (*RpcError)(nil), rpcErr)
		for i := 1; i < len(duties); i++ {
			assert.Equal(t, true, duties[i-1].Slot <= duties[i].Slot, "duties should be sorted by slot")
		}
	})
}

func TestSyncCommitteeDuties(t *testing.T) {
	helpers.ClearCache()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	numVals := uint64(11)
	st, _ := util.DeterministicGenesisStateAltair(t, numVals)
	vals := st.Validators()

	currCommittee := &ethpb.SyncCommittee{AggregatePubkey: make([]byte, 48)}
	for i := range 5 {
		currCommittee.Pubkeys = append(currCommittee.Pubkeys, vals[i].PublicKey)
	}
	// Add one pubkey twice to test duplicate positions.
	currCommittee.Pubkeys = append(currCommittee.Pubkeys, vals[0].PublicKey)
	require.NoError(t, st.SetCurrentSyncCommittee(currCommittee))

	nextCommittee := &ethpb.SyncCommittee{AggregatePubkey: make([]byte, 48)}
	for i := 5; i < 10; i++ {
		nextCommittee.Pubkeys = append(nextCommittee.Pubkeys, vals[i].PublicKey)
	}
	require.NoError(t, st.SetNextSyncCommittee(nextCommittee))

	s := &Service{}

	t.Run("current committee", func(t *testing.T) {
		duties, rpcErr := s.SyncCommitteeDuties(t.Context(), st, 0, 0, []primitives.ValidatorIndex{1})
		require.Equal(t, (*RpcError)(nil), rpcErr)
		require.Equal(t, 1, len(duties))
		assert.Equal(t, primitives.ValidatorIndex(1), duties[0].ValidatorIndex)
		require.Equal(t, 1, len(duties[0].ValidatorSyncCommitteeIndices))
		assert.Equal(t, uint64(1), duties[0].ValidatorSyncCommitteeIndices[0])
	})

	t.Run("validator with duplicate positions", func(t *testing.T) {
		duties, rpcErr := s.SyncCommitteeDuties(t.Context(), st, 0, 0, []primitives.ValidatorIndex{0})
		require.Equal(t, (*RpcError)(nil), rpcErr)
		require.Equal(t, 1, len(duties))
		// Validator 0 appears at index 0 and 5.
		require.Equal(t, 2, len(duties[0].ValidatorSyncCommitteeIndices))
	})

	t.Run("next committee", func(t *testing.T) {
		nextEpoch := params.BeaconConfig().EpochsPerSyncCommitteePeriod
		duties, rpcErr := s.SyncCommitteeDuties(t.Context(), st, nextEpoch, 0, []primitives.ValidatorIndex{5})
		require.Equal(t, (*RpcError)(nil), rpcErr)
		require.Equal(t, 1, len(duties))
		assert.Equal(t, primitives.ValidatorIndex(5), duties[0].ValidatorIndex)
	})

	t.Run("validator not in committee", func(t *testing.T) {
		// Validator 10 is not in either committee.
		duties, rpcErr := s.SyncCommitteeDuties(t.Context(), st, 0, 0, []primitives.ValidatorIndex{10})
		require.Equal(t, (*RpcError)(nil), rpcErr)
		require.Equal(t, 0, len(duties))
	})

	t.Run("zero pubkey returns error", func(t *testing.T) {
		badIndex := primitives.ValidatorIndex(numVals + 100)
		_, rpcErr := s.SyncCommitteeDuties(t.Context(), st, 0, 0, []primitives.ValidatorIndex{badIndex})
		require.NotNil(t, rpcErr)
		require.Equal(t, ErrorReason(BadRequest), rpcErr.Reason)
	})
}

func TestSyncCommitteeDutiesLastValidEpoch(t *testing.T) {
	t.Run("epoch 0", func(t *testing.T) {
		result := SyncCommitteeDutiesLastValidEpoch(0)
		expected := 2*params.BeaconConfig().EpochsPerSyncCommitteePeriod - 1
		assert.Equal(t, expected, result)
	})
}

func TestFindValidatorIndexInCommittee(t *testing.T) {
	committee := []primitives.ValidatorIndex{10, 20, 30}
	assert.Equal(t, uint64(0), findValidatorIndexInCommittee(committee, 10))
	assert.Equal(t, uint64(1), findValidatorIndexInCommittee(committee, 20))
	assert.Equal(t, uint64(2), findValidatorIndexInCommittee(committee, 30))
	// Not found returns 0.
	assert.Equal(t, uint64(0), findValidatorIndexInCommittee(committee, 99))
}
