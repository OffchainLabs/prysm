package fulu_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/fulu"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestUpgradeToFulu(t *testing.T) {
	st, _ := util.DeterministicGenesisStateElectra(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, st.SetHistoricalRoots([][]byte{{1}}))
	vals := st.Validators()
	vals[0].ActivationEpoch = params.BeaconConfig().FarFutureEpoch
	vals[1].WithdrawalCredentials = []byte{params.BeaconConfig().CompoundingWithdrawalPrefixByte}
	require.NoError(t, st.SetValidators(vals))
	bals := st.Balances()
	bals[1] = params.BeaconConfig().MinActivationBalance + 1000
	require.NoError(t, st.SetBalances(bals))

	preForkState := st.Copy()
	mSt, err := fulu.UpgradeToFulu(t.Context(), st)
	require.NoError(t, err)

	require.Equal(t, preForkState.GenesisTime(), mSt.GenesisTime())
	require.DeepSSZEqual(t, preForkState.GenesisValidatorsRoot(), mSt.GenesisValidatorsRoot())
	require.Equal(t, preForkState.Slot(), mSt.Slot())

	f := mSt.Fork()
	require.DeepSSZEqual(t, &ethpb.Fork{
		PreviousVersion: st.Fork().CurrentVersion,
		CurrentVersion:  params.BeaconConfig().FuluForkVersion,
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

	header, err := mSt.LatestExecutionPayloadHeader()
	require.NoError(t, err)
	protoHeader, ok := header.Proto().(*enginev1.ExecutionPayloadHeaderDeneb)
	require.Equal(t, true, ok)
	prevHeader, err := preForkState.LatestExecutionPayloadHeader()
	require.NoError(t, err)
	txRoot, err := prevHeader.TransactionsRoot()
	require.NoError(t, err)
	wdRoot, err := prevHeader.WithdrawalsRoot()
	require.NoError(t, err)
	wanted := &enginev1.ExecutionPayloadHeaderDeneb{
		ParentHash:       prevHeader.ParentHash(),
		FeeRecipient:     prevHeader.FeeRecipient(),
		StateRoot:        prevHeader.StateRoot(),
		ReceiptsRoot:     prevHeader.ReceiptsRoot(),
		LogsBloom:        prevHeader.LogsBloom(),
		PrevRandao:       prevHeader.PrevRandao(),
		BlockNumber:      prevHeader.BlockNumber(),
		GasLimit:         prevHeader.GasLimit(),
		GasUsed:          prevHeader.GasUsed(),
		Timestamp:        prevHeader.Timestamp(),
		ExtraData:        prevHeader.ExtraData(),
		BaseFeePerGas:    prevHeader.BaseFeePerGas(),
		BlockHash:        prevHeader.BlockHash(),
		TransactionsRoot: txRoot,
		WithdrawalsRoot:  wdRoot,
	}
	require.DeepEqual(t, wanted, protoHeader)

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
}
