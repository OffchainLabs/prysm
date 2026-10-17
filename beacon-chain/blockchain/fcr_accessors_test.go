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

func TestBalanceInfoByCheckpoint(t *testing.T) {
	t.Run("skipped boundary uses the checkpoint epoch", func(t *testing.T) {
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
		info, err := acc.BalanceInfoByCheckpoint(ctx, forkchoicetypes.Checkpoint{Epoch: 1, Root: root})
		require.NoError(t, err)

		require.Equal(t, st.NumValidators(), len(info.Balances))
		require.Equal(t, cfg.MaxEffectiveBalance, info.Balances[newIdx])
		require.Equal(t, uint64(st.NumValidators())*cfg.MaxEffectiveBalance, info.TotalActiveBalance)
	})

	t.Run("second lookup hits the cache", func(t *testing.T) {
		helpers.ClearCache()
		service, tr := minimalTestService(t)
		st, _ := util.DeterministicGenesisState(t, 64)
		blk := util.NewBeaconBlock()
		util.SaveBlock(t, tr.ctx, tr.db, blk)
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, tr.db.SaveState(tr.ctx, st, root))
		acc := &fcrBalanceAccessor{s: service}
		cp := forkchoicetypes.Checkpoint{Epoch: 0, Root: root}

		first, err := acc.BalanceInfoByCheckpoint(tr.ctx, cp)
		require.NoError(t, err)
		second, err := acc.BalanceInfoByCheckpoint(tr.ctx, cp)
		require.NoError(t, err)
		// Equal compares the pointers, so this proves the second lookup did not recompute.
		require.Equal(t, first, second)
	})

	t.Run("failed lookup leaves no entry", func(t *testing.T) {
		helpers.ClearCache()
		service, tr := minimalTestService(t)

		st, _ := util.DeterministicGenesisState(t, 64)
		blk := util.NewBeaconBlock()
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)

		acc := &fcrBalanceAccessor{s: service}
		cp := forkchoicetypes.Checkpoint{Epoch: 0, Root: root}

		// Unknown root: a failed prewarm-time lookup must not poison the cache.
		_, err = acc.BalanceInfoByCheckpoint(tr.ctx, cp)
		require.ErrorContains(t, "could not get state for checkpoint root", err)
		require.Equal(t, 0, len(acc.byCheckpoint))

		// The state becomes available later, the slot-start lookup still computes it.
		util.SaveBlock(t, tr.ctx, tr.db, blk)
		require.NoError(t, tr.db.SaveState(tr.ctx, st, root))
		got, err := acc.BalanceInfoByCheckpoint(tr.ctx, cp)
		require.NoError(t, err)
		require.Equal(t, st.NumValidators(), len(got.Balances))
	})
}
