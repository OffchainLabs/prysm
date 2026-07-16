package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestBalanceInfoByCheckpoint_SkippedBoundaryUsesCheckpointEpoch(t *testing.T) {
	helpers.ClearCache()
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	cfg := params.BeaconConfig()

	st, _ := util.DeterministicGenesisState(t, 64)
	// Inactive in epoch 0, active in epoch 1, so it is only counted if the epoch transition ran.
	pk := make([]byte, 48)
	pk[0] = 0xff
	require.NoError(t, st.AppendValidator(&ethpb.Validator{
		PublicKey:                  pk,
		WithdrawalCredentials:      make([]byte, 32),
		ActivationEligibilityEpoch: 0,
		ActivationEpoch:            1,
		ExitEpoch:                  cfg.FarFutureEpoch,
		WithdrawableEpoch:          cfg.FarFutureEpoch,
		EffectiveBalance:           cfg.MaxEffectiveBalance,
	}))
	require.NoError(t, st.AppendBalance(cfg.MaxEffectiveBalance))
	newIdx := st.NumValidators() - 1

	// The checkpoint block sits in the last slot of epoch 0 and the epoch 1 boundary slot is skipped.
	lastSlotOfEpoch0 := cfg.SlotsPerEpoch - 1
	require.NoError(t, st.SetSlot(lastSlotOfEpoch0))
	blk := util.NewBeaconBlock()
	blk.Block.Slot = lastSlotOfEpoch0
	util.SaveBlock(t, ctx, tr.db, blk)
	root, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, tr.db.SaveState(ctx, st, root))

	acc := &fcrBalanceAccessor{s: service}
	balances, total, err := acc.BalanceInfoByCheckpoint(ctx, forkchoicetypes.Checkpoint{Epoch: 1, Root: root})
	require.NoError(t, err)

	require.Equal(t, st.NumValidators(), len(balances))
	require.Equal(t, cfg.MaxEffectiveBalance, balances[newIdx])
	require.Equal(t, uint64(st.NumValidators())*cfg.MaxEffectiveBalance, total)
}
