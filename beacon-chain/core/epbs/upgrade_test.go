package epbs_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/epbs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

func TestUpgradeToEip7732(t *testing.T) {
	st, _ := util.DeterministicGenesisStateElectra(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, st.SetHistoricalRoots([][]byte{{1}}))

	preForkState := st.Copy()
	mSt, err := epbs.UpgradeToEIP7732(st)
	require.NoError(t, err)

	require.Equal(t, preForkState.GenesisTime(), mSt.GenesisTime())
	require.DeepSSZEqual(t, preForkState.GenesisValidatorsRoot(), mSt.GenesisValidatorsRoot())
	require.Equal(t, preForkState.Slot(), mSt.Slot())
	require.DeepSSZEqual(t, preForkState.LatestBlockHeader(), mSt.LatestBlockHeader())
	require.DeepSSZEqual(t, preForkState.BlockRoots(), mSt.BlockRoots())
	require.DeepSSZEqual(t, preForkState.StateRoots(), mSt.StateRoots())
	require.DeepSSZEqual(t, preForkState.Validators()[2:], mSt.Validators()[2:])
	require.DeepSSZEqual(t, preForkState.Balances()[2:], mSt.Balances()[2:])
	require.DeepSSZEqual(t, preForkState.Eth1Data(), mSt.Eth1Data())
	require.DeepSSZEqual(t, preForkState.Eth1DataVotes(), mSt.Eth1DataVotes())
	require.DeepSSZEqual(t, preForkState.Eth1DepositIndex(), mSt.Eth1DepositIndex())
	require.DeepSSZEqual(t, preForkState.RandaoMixes(), mSt.RandaoMixes())
	require.DeepSSZEqual(t, preForkState.Slashings(), mSt.Slashings())
	require.DeepSSZEqual(t, preForkState.JustificationBits(), mSt.JustificationBits())
	require.DeepSSZEqual(t, preForkState.PreviousJustifiedCheckpoint(), mSt.PreviousJustifiedCheckpoint())
	require.DeepSSZEqual(t, preForkState.CurrentJustifiedCheckpoint(), mSt.CurrentJustifiedCheckpoint())
	require.DeepSSZEqual(t, preForkState.FinalizedCheckpoint(), mSt.FinalizedCheckpoint())

	require.Equal(t, len(preForkState.Validators()), len(mSt.Validators()))

	numValidators := mSt.NumValidators()
	p, err := mSt.PreviousEpochParticipation()
	require.NoError(t, err)
	require.DeepSSZEqual(t, make([]byte, numValidators), p)
	p, err = mSt.CurrentEpochParticipation()
	require.NoError(t, err)
	require.DeepSSZEqual(t, make([]byte, numValidators), p)
	s, err := mSt.InactivityScores()
	require.NoError(t, err)
	require.DeepSSZEqual(t, make([]uint64, numValidators), s)

	hr1, err := preForkState.HistoricalRoots()
	require.NoError(t, err)
	hr2, err := mSt.HistoricalRoots()
	require.NoError(t, err)
	require.DeepEqual(t, hr1, hr2)

	f := mSt.Fork()
	require.DeepSSZEqual(t, &ethpb.Fork{
		PreviousVersion: st.Fork().CurrentVersion,
		CurrentVersion:  params.BeaconConfig().EPBSForkVersion,
		Epoch:           time.CurrentEpoch(st),
	}, f)
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

	nwi, err := mSt.NextWithdrawalIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(0), nwi)

	lwvi, err := mSt.NextWithdrawalValidatorIndex()
	require.NoError(t, err)
	require.Equal(t, primitives.ValidatorIndex(0), lwvi)

	summaries, err := mSt.HistoricalSummaries()
	require.NoError(t, err)
	require.Equal(t, 0, len(summaries))

	startIndex, err := mSt.DepositRequestsStartIndex()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().UnsetDepositRequestsStartIndex, startIndex)

	balance, err := mSt.DepositBalanceToConsume()
	require.NoError(t, err)
	require.Equal(t, primitives.Gwei(0), balance)

	tab, err := helpers.TotalActiveBalance(mSt)
	require.NoError(t, err)

	ebtc, err := mSt.ExitBalanceToConsume()
	require.NoError(t, err)
	require.Equal(t, helpers.ActivationExitChurnLimit(primitives.Gwei(tab)), ebtc)

	cbtc, err := mSt.ConsolidationBalanceToConsume()
	require.NoError(t, err)
	require.Equal(t, helpers.ConsolidationChurnLimit(primitives.Gwei(tab)), cbtc)

	earliestConsolidationEpoch, err := mSt.EarliestConsolidationEpoch()
	require.NoError(t, err)
	require.Equal(t, helpers.ActivationExitEpoch(slots.ToEpoch(preForkState.Slot())), earliestConsolidationEpoch)

	// EIP-7732 checks.
	h, err := mSt.LatestExecutionPayloadHeaderEPBS()
	require.NoError(t, err)
	require.DeepEqual(t, &enginev1.ExecutionPayloadHeaderEPBS{
		ParentBlockHash:        make([]byte, 32),
		ParentBlockRoot:        make([]byte, 32),
		BlockHash:              make([]byte, 32),
		BlobKzgCommitmentsRoot: make([]byte, 32),
	}, h)
	lwr, err := mSt.LastWithdrawalsRoot()
	require.NoError(t, err)
	require.DeepEqual(t, lwr, make([]byte, 32))
	lbh, err := mSt.LatestBlockHash()
	require.NoError(t, err)
	lh, err := preForkState.LatestExecutionPayloadHeader()
	require.NoError(t, err)
	require.DeepEqual(t, lbh, lh.BlockHash())
	slot, err := mSt.LatestFullSlot()
	require.NoError(t, err)
	require.Equal(t, slot, preForkState.Slot())
}
