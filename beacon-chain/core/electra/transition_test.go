package electra_test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/electra"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	consensusblocks "github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestVerifyOperationLengths_Electra(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		s, _ := util.DeterministicGenesisStateElectra(t, 1)
		sb, err := consensusblocks.NewSignedBeaconBlock(util.NewBeaconBlockElectra())
		require.NoError(t, err)
		require.NoError(t, electra.VerifyBlockDepositLength(sb.Block().Body(), s))
	})
	t.Run("eth1depositIndex less than eth1depositIndexLimit & number of deposits incorrect", func(t *testing.T) {
		s, _ := util.DeterministicGenesisStateElectra(t, 1)
		sb, err := consensusblocks.NewSignedBeaconBlock(util.NewBeaconBlockElectra())
		require.NoError(t, err)
		require.NoError(t, s.SetEth1DepositIndex(0))
		require.NoError(t, s.SetDepositRequestsStartIndex(1))
		err = electra.VerifyBlockDepositLength(sb.Block().Body(), s)
		require.ErrorContains(t, "incorrect outstanding deposits in block body", err)
	})
	t.Run("eth1depositIndex more than eth1depositIndexLimit & number of deposits is not 0", func(t *testing.T) {
		s, _ := util.DeterministicGenesisStateElectra(t, 1)
		sb, err := consensusblocks.NewSignedBeaconBlock(util.NewBeaconBlockElectra())
		require.NoError(t, err)
		sb.SetDeposits([]*ethpb.Deposit{
			{
				Data: &ethpb.Deposit_Data{
					PublicKey:             []byte{1, 2, 3},
					Amount:                1000,
					WithdrawalCredentials: make([]byte, common.AddressLength),
					Signature:             []byte{4, 5, 6},
				},
			},
		})
		require.NoError(t, s.SetEth1DepositIndex(1))
		require.NoError(t, s.SetDepositRequestsStartIndex(1))
		err = electra.VerifyBlockDepositLength(sb.Block().Body(), s)
		require.ErrorContains(t, "incorrect outstanding deposits in block body", err)
	})
}

func TestProcessEpoch_CanProcessElectra(t *testing.T) {
	st, _ := util.DeterministicGenesisStateElectra(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, st.SetSlot(10*params.BeaconConfig().SlotsPerEpoch))
	require.NoError(t, st.SetDepositBalanceToConsume(100))
	amountAvailForProcessing := helpers.ActivationExitChurnLimit(1_000 * 1e9)
	validators := st.Validators()
	deps := make([]*ethpb.PendingDeposit, 20)
	for i := 0; i < len(deps); i += 1 {
		deps[i] = &ethpb.PendingDeposit{
			PublicKey:             validators[i].PublicKey,
			WithdrawalCredentials: validators[i].WithdrawalCredentials,
			Amount:                uint64(amountAvailForProcessing) / 10,
			Slot:                  0,
		}
	}
	require.NoError(t, st.SetPendingDeposits(deps))
	require.NoError(t, st.SetPendingConsolidations([]*ethpb.PendingConsolidation{
		{
			SourceIndex: 2,
			TargetIndex: 3,
		},
		{
			SourceIndex: 0,
			TargetIndex: 1,
		},
	}))
	err := electra.ProcessEpoch(context.Background(), st)
	require.NoError(t, err)
	require.Equal(t, uint64(0), st.Slashings()[2], "Unexpected slashed balance")

	b := st.Balances()
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(b)))
	require.Equal(t, uint64(44799839993), b[0])

	s, err := st.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(s)))

	p, err := st.PreviousEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	p, err = st.CurrentEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	sc, err := st.CurrentSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))

	sc, err = st.NextSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))

	res, err := st.DepositBalanceToConsume()
	require.NoError(t, err)
	require.Equal(t, primitives.Gwei(100), res)

	// Half of the balance deposits should have been processed.
	remaining, err := st.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 10, len(remaining))

	num, err := st.NumPendingConsolidations()
	require.NoError(t, err)
	require.Equal(t, uint64(2), num)
}
