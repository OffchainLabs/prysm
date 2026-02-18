package gloas_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestUpgradeToGloas(t *testing.T) {
	st, _ := util.DeterministicGenesisStateFulu(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, st.SetHistoricalRoots([][]byte{{1}}))
	vals := st.Validators()
	vals[0].ActivationEpoch = params.BeaconConfig().FarFutureEpoch
	vals[1].WithdrawalCredentials = []byte{params.BeaconConfig().CompoundingWithdrawalPrefixByte}
	require.NoError(t, st.SetValidators(vals))
	bals := st.Balances()
	bals[1] = params.BeaconConfig().MinActivationBalance + 1000
	require.NoError(t, st.SetBalances(bals))
	executionPayloadHeader, err := st.LatestExecutionPayloadHeader()
	require.NoError(t, err)
	protoHeader, ok := executionPayloadHeader.Proto().(*enginev1.ExecutionPayloadHeaderDeneb)
	require.Equal(t, true, ok)
	protoHeader.BlockHash = bytes.Repeat([]byte{7}, 32)
	newHeader, err := blocks.WrappedExecutionPayloadHeaderDeneb(protoHeader)
	require.NoError(t, err)
	require.NoError(t, st.SetLatestExecutionPayloadHeader(newHeader))

	preForkState := st.Copy()
	mSt, err := gloas.UpgradeToGloas(st)
	require.NoError(t, err)

	require.Equal(t, preForkState.GenesisTime(), mSt.GenesisTime())
	require.DeepSSZEqual(t, preForkState.GenesisValidatorsRoot(), mSt.GenesisValidatorsRoot())
	require.Equal(t, preForkState.Slot(), mSt.Slot())

	f := mSt.Fork()
	require.DeepSSZEqual(t, &ethpb.Fork{
		PreviousVersion: st.Fork().CurrentVersion,
		CurrentVersion:  params.BeaconConfig().GloasForkVersion,
		Epoch:           time.CurrentEpoch(st),
	}, f)

	require.DeepSSZEqual(t, preForkState.LatestBlockHeader(), mSt.LatestBlockHeader())
	require.DeepSSZEqual(t, preForkState.BlockRoots(), mSt.BlockRoots())
	require.DeepSSZEqual(t, preForkState.StateRoots(), mSt.StateRoots())

	hr1 := preForkState.HistoricalRoots()
	hr2 := mSt.HistoricalRoots()
	require.DeepEqual(t, hr1, hr2)

	require.DeepSSZEqual(t, preForkState.Eth1Data(), mSt.Eth1Data())
	require.DeepSSZEqual(t, preForkState.Eth1DataVotes(), mSt.Eth1DataVotes())
	require.DeepSSZEqual(t, preForkState.Eth1DepositIndex(), mSt.Eth1DepositIndex())
	require.DeepSSZEqual(t, preForkState.Validators(), mSt.Validators())
	require.DeepSSZEqual(t, preForkState.Balances(), mSt.Balances())
	require.DeepSSZEqual(t, preForkState.RandaoMixes(), mSt.RandaoMixes())
	require.DeepSSZEqual(t, preForkState.Slashings(), mSt.Slashings())

	numValidators := mSt.NumValidators()

	p, err := mSt.PreviousEpochParticipation()
	require.NoError(t, err)
	require.DeepSSZEqual(t, make([]byte, numValidators), p)

	p, err = mSt.CurrentEpochParticipation()
	require.NoError(t, err)
	require.DeepSSZEqual(t, make([]byte, numValidators), p)

	require.DeepSSZEqual(t, preForkState.JustificationBits(), mSt.JustificationBits())
	require.DeepSSZEqual(t, preForkState.PreviousJustifiedCheckpoint(), mSt.PreviousJustifiedCheckpoint())
	require.DeepSSZEqual(t, preForkState.CurrentJustifiedCheckpoint(), mSt.CurrentJustifiedCheckpoint())
	require.DeepSSZEqual(t, preForkState.FinalizedCheckpoint(), mSt.FinalizedCheckpoint())

	s, err := mSt.InactivityScores()
	require.NoError(t, err)
	require.DeepSSZEqual(t, make([]uint64, numValidators), s)

	csc, err := mSt.CurrentSyncCommittee()
	require.NoError(t, err)
	psc, err := preForkState.CurrentSyncCommittee()
	require.NoError(t, err)
	require.DeepSSZEqual(t, psc, csc)

	nsc, err := mSt.NextSyncCommittee()
	require.NoError(t, err)
	psc, err = preForkState.NextSyncCommittee()
	require.NoError(t, err)
	require.DeepSSZEqual(t, psc, nsc)

	ph, err := st.LatestExecutionPayloadHeader()
	require.NoError(t, err)
	blockHash := ph.BlockHash()
	executionPayloadBid, err := mSt.LatestExecutionPayloadBid()
	require.NoError(t, err)
	newBlockHash := executionPayloadBid.BlockHash()

	require.DeepSSZEqual(t, blockHash, newBlockHash[:])

	nwi, err := mSt.NextWithdrawalIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(0), nwi)

	lwvi, err := mSt.NextWithdrawalValidatorIndex()
	require.NoError(t, err)
	require.Equal(t, primitives.ValidatorIndex(0), lwvi)

	summaries, err := mSt.HistoricalSummaries()
	require.NoError(t, err)
	require.Equal(t, 0, len(summaries))

	preDepositRequestsStartIndex, err := preForkState.DepositRequestsStartIndex()
	require.NoError(t, err)
	postDepositRequestsStartIndex, err := mSt.DepositRequestsStartIndex()
	require.NoError(t, err)
	require.Equal(t, preDepositRequestsStartIndex, postDepositRequestsStartIndex)

	preDepositBalanceToConsume, err := preForkState.DepositBalanceToConsume()
	require.NoError(t, err)
	postDepositBalanceToConsume, err := mSt.DepositBalanceToConsume()
	require.NoError(t, err)
	require.Equal(t, preDepositBalanceToConsume, postDepositBalanceToConsume)

	preExitBalanceToConsume, err := preForkState.ExitBalanceToConsume()
	require.NoError(t, err)
	postExitBalanceToConsume, err := mSt.ExitBalanceToConsume()
	require.NoError(t, err)
	require.Equal(t, preExitBalanceToConsume, postExitBalanceToConsume)

	preEarliestExitEpoch, err := preForkState.EarliestExitEpoch()
	require.NoError(t, err)
	postEarliestExitEpoch, err := mSt.EarliestExitEpoch()
	require.NoError(t, err)
	require.Equal(t, preEarliestExitEpoch, postEarliestExitEpoch)

	preConsolidationBalanceToConsume, err := preForkState.ConsolidationBalanceToConsume()
	require.NoError(t, err)
	postConsolidationBalanceToConsume, err := mSt.ConsolidationBalanceToConsume()
	require.NoError(t, err)
	require.Equal(t, preConsolidationBalanceToConsume, postConsolidationBalanceToConsume)

	preEarliesConsolidationEoch, err := preForkState.EarliestConsolidationEpoch()
	require.NoError(t, err)
	postEarliestConsolidationEpoch, err := mSt.EarliestConsolidationEpoch()
	require.NoError(t, err)
	require.Equal(t, preEarliesConsolidationEoch, postEarliestConsolidationEpoch)

	prePendingDeposits, err := preForkState.PendingDeposits()
	require.NoError(t, err)
	postPendingDeposits, err := mSt.PendingDeposits()
	require.NoError(t, err)
	require.DeepSSZEqual(t, prePendingDeposits, postPendingDeposits)

	prePendingPartialWithdrawals, err := preForkState.PendingPartialWithdrawals()
	require.NoError(t, err)
	postPendingPartialWithdrawals, err := mSt.PendingPartialWithdrawals()
	require.NoError(t, err)
	require.DeepSSZEqual(t, prePendingPartialWithdrawals, postPendingPartialWithdrawals)

	prePendingConsolidations, err := preForkState.PendingConsolidations()
	require.NoError(t, err)
	postPendingConsolidations, err := mSt.PendingConsolidations()
	require.NoError(t, err)
	require.DeepSSZEqual(t, prePendingConsolidations, postPendingConsolidations)

	_, err = mSt.Builder(0)
	require.ErrorContains(t, "out of bounds", err)

	for i := primitives.Slot(0); i < params.BeaconConfig().SlotsPerHistoricalRoot; i++ {
		available, err := mSt.ExecutionPayloadAvailability(i)
		require.NoError(t, err)
		require.Equal(t, uint64(1), available)
	}

	bpp, err := mSt.BuilderPendingPayments()
	require.NoError(t, err)
	require.Equal(t, int(2*params.BeaconConfig().SlotsPerEpoch), len(bpp))

	nbh, err := mSt.LatestBlockHash()
	require.NoError(t, err)
	require.DeepSSZEqual(t, blockHash, nbh[:])
}
