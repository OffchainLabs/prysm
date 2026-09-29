package electra_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common"
)

func TestVerifyBlockDepositLength(t *testing.T) {
	oneDeposit := []*ethpb.Deposit{
		{
			Data: &ethpb.Deposit_Data{
				PublicKey:             []byte{1, 2, 3},
				Amount:                1000,
				WithdrawalCredentials: make([]byte, common.AddressLength),
				Signature:             []byte{4, 5, 6},
			},
		},
	}
	unsetStartIndex := params.BeaconConfig().UnsetDepositRequestsStartIndex

	// From Fulu onward the start index is never set, so every post-Fulu case below leaves the Eth1
	// bridge unfinished: the Electra limit would have asked for a deposit there, which is what
	// makes each case fail if the Fulu branch is removed. Gloas is covered alongside Fulu because
	// the gate is keyed on `>= version.Fulu`.
	for _, tt := range []struct {
		name             string
		newState         func(testing.TB) state.BeaconState
		newBlock         func() any
		eth1DepositIndex uint64
		startIndex       uint64
		deposits         []*ethpb.Deposit
		wantErr          string
	}{
		{
			name:             "electra accepts an empty body once eth1_deposit_index reached the limit",
			newState:         func(t testing.TB) state.BeaconState { s, _ := util.DeterministicGenesisStateElectra(t, 64); return s },
			newBlock:         func() any { return util.NewBeaconBlockElectra() },
			eth1DepositIndex: 64,
			startIndex:       unsetStartIndex,
		},
		{
			name:             "electra requires the outstanding Eth1 bridge deposits",
			newState:         func(t testing.TB) state.BeaconState { s, _ := util.DeterministicGenesisStateElectra(t, 64); return s },
			newBlock:         func() any { return util.NewBeaconBlockElectra() },
			eth1DepositIndex: 0,
			startIndex:       1,
			wantErr:          "incorrect outstanding deposits in block body",
		},
		{
			name:             "electra rejects an Eth1 bridge deposit once the bridge is drained",
			newState:         func(t testing.TB) state.BeaconState { s, _ := util.DeterministicGenesisStateElectra(t, 64); return s },
			newBlock:         func() any { return util.NewBeaconBlockElectra() },
			eth1DepositIndex: 1,
			startIndex:       1,
			deposits:         oneDeposit,
			wantErr:          "incorrect outstanding deposits in block body",
		},
		{
			name: "fulu accepts an empty body while the Eth1 bridge is unfinished",
			newState: func(t testing.TB) state.BeaconState {
				s, _ := util.DeterministicGenesisStateFulu(t, 64)
				return s
			},
			newBlock:         func() any { return util.NewBeaconBlockFulu() },
			eth1DepositIndex: 0,
			startIndex:       unsetStartIndex,
		},
		{
			name: "fulu rejects an Eth1 bridge deposit",
			newState: func(t testing.TB) state.BeaconState {
				s, _ := util.DeterministicGenesisStateFulu(t, 64)
				return s
			},
			newBlock:         func() any { return util.NewBeaconBlockFulu() },
			eth1DepositIndex: 0,
			startIndex:       unsetStartIndex,
			deposits:         oneDeposit,
			wantErr:          "eth1 bridge deposits are not allowed from Fulu",
		},
		{
			name: "gloas accepts an empty body while the Eth1 bridge is unfinished",
			newState: func(t testing.TB) state.BeaconState {
				s, _ := util.DeterministicGenesisStateGloas(t, 64)
				return s
			},
			newBlock:         func() any { return util.NewBeaconBlockGloas() },
			eth1DepositIndex: 0,
			startIndex:       unsetStartIndex,
		},
		{
			name: "gloas rejects an Eth1 bridge deposit",
			newState: func(t testing.TB) state.BeaconState {
				s, _ := util.DeterministicGenesisStateGloas(t, 64)
				return s
			},
			newBlock:         func() any { return util.NewBeaconBlockGloas() },
			eth1DepositIndex: 0,
			startIndex:       unsetStartIndex,
			deposits:         oneDeposit,
			wantErr:          "eth1 bridge deposits are not allowed from Fulu",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.newState(t)
			require.NoError(t, s.SetEth1DepositIndex(tt.eth1DepositIndex))
			require.NoError(t, s.SetDepositRequestsStartIndex(tt.startIndex))
			sb, err := consensusblocks.NewSignedBeaconBlock(tt.newBlock())
			require.NoError(t, err)
			sb.SetDeposits(tt.deposits)

			err = electra.VerifyBlockDepositLength(sb.Block().Body(), s)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, tt.wantErr, err)
		})
	}
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
	err := electra.ProcessEpoch(t.Context(), st)
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
