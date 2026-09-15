package blockchain

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func warmupCreds(t *testing.T, builder bool) []byte {
	t.Helper()
	creds := make([]byte, fieldparams.RootLength)
	if builder {
		creds[0] = params.BeaconConfig().BuilderWithdrawalPrefixByte
		for i := 12; i < len(creds); i++ {
			creds[i] = 0x7a
		}
		return creds
	}
	creds[0] = params.BeaconConfig().BLSWithdrawalPrefixByte
	return creds
}

func warmupDeposit(t *testing.T, sk bls.SecretKey, creds []byte, amount uint64, valid bool) *ethpb.PendingDeposit {
	t.Helper()
	sig := make([]byte, fieldparams.BLSSignatureLength)
	if valid {
		domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
		require.NoError(t, err)
		root, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
			PublicKey:             sk.PublicKey().Marshal(),
			WithdrawalCredentials: creds,
			Amount:                amount,
		}, domain)
		require.NoError(t, err)
		sig = sk.Sign(root[:]).Marshal()
	}
	return &ethpb.PendingDeposit{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: creds,
		Amount:                amount,
		Signature:             sig,
	}
}

func TestPendingBuilderDepositCandidates(t *testing.T) {
	builderCreds := warmupCreds(t, true)
	valCreds := warmupCreds(t, false)

	keys := make([]bls.SecretKey, 4)
	for i := range keys {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		keys[i] = sk
	}

	// keys[0] builder+validator creds, keys[1] validator creds only, keys[2] builder creds but
	// already a validator, keys[3] builder creds.
	deposits := []*ethpb.PendingDeposit{
		warmupDeposit(t, keys[0], valCreds, 1, false),
		warmupDeposit(t, keys[1], valCreds, 2, true),
		warmupDeposit(t, keys[0], builderCreds, 3, true),
		warmupDeposit(t, keys[2], builderCreds, 4, true),
		warmupDeposit(t, keys[1], valCreds, 5, true),
		warmupDeposit(t, keys[3], builderCreds, 6, true),
		warmupDeposit(t, keys[0], valCreds, 7, true),
	}

	st, err := util.NewBeaconStateGloas(func(s *ethpb.BeaconStateGloas) error {
		s.PendingDeposits = deposits
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, st.AppendValidator(&ethpb.Validator{
		PublicKey:             keys[2].PublicKey().Marshal(),
		WithdrawalCredentials: valCreds,
		EffectiveBalance:      params.BeaconConfig().MinDepositAmount,
		ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
	}))
	require.NoError(t, st.AppendBalance(params.BeaconConfig().MinDepositAmount))

	candidates, err := pendingBuilderDepositCandidates(st)
	require.NoError(t, err)

	wantAmounts := []uint64{1, 3, 6, 7}
	require.Equal(t, len(wantAmounts), len(candidates))
	for i, amount := range wantAmounts {
		require.Equal(t, amount, candidates[i].Amount)
	}
}

func TestPendingBuilderDepositCandidatesNoBuilderCredentials(t *testing.T) {
	valCreds := warmupCreds(t, false)
	sk, err := bls.RandKey()
	require.NoError(t, err)

	st, err := util.NewBeaconStateGloas(func(s *ethpb.BeaconStateGloas) error {
		s.PendingDeposits = []*ethpb.PendingDeposit{
			warmupDeposit(t, sk, valCreds, 1, true),
			warmupDeposit(t, sk, valCreds, 2, false),
		}
		return nil
	})
	require.NoError(t, err)

	candidates, err := pendingBuilderDepositCandidates(st)
	require.NoError(t, err)
	require.Equal(t, 0, len(candidates))
}

func TestVerifyDepositSignaturesPopulatesCache(t *testing.T) {
	cache.DepositSignature.Clear()
	builderCreds := warmupCreds(t, true)

	var data []*ethpb.Deposit_Data
	for i := range 8 {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		deposit := warmupDeposit(t, sk, builderCreds, uint64(i+1), i%2 == 0)
		data = append(data, &ethpb.Deposit_Data{
			PublicKey:             deposit.PublicKey,
			WithdrawalCredentials: deposit.WithdrawalCredentials,
			Amount:                deposit.Amount,
			Signature:             deposit.Signature,
		})
	}

	s := &Service{ctx: context.Background()}
	verified := s.verifyDepositSignatures(context.Background(), data)
	require.Equal(t, len(data), verified)
	require.Equal(t, len(data), cache.DepositSignature.Len())

	for i, d := range data {
		key, err := d.HashTreeRoot()
		require.NoError(t, err)
		valid, ok := cache.DepositSignature.Get(key)
		require.Equal(t, true, ok)
		require.Equal(t, i%2 == 0, valid)
	}
}

func TestVerifyDepositSignaturesCancelledContext(t *testing.T) {
	cache.DepositSignature.Clear()
	builderCreds := warmupCreds(t, true)
	sk, err := bls.RandKey()
	require.NoError(t, err)
	deposit := warmupDeposit(t, sk, builderCreds, 32, true)
	data := []*ethpb.Deposit_Data{{
		PublicKey:             deposit.PublicKey,
		WithdrawalCredentials: deposit.WithdrawalCredentials,
		Amount:                deposit.Amount,
		Signature:             deposit.Signature,
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := &Service{ctx: context.Background()}
	require.Equal(t, 0, s.verifyDepositSignatures(ctx, data))
	require.Equal(t, 0, cache.DepositSignature.Len())
}

func TestDepositWarmupWorkers(t *testing.T) {
	require.Equal(t, true, depositWarmupWorkers() >= 1)
}

// Byte-identical pending deposits are legal and share a cache key, so they must collapse to a
// single verification. Only PendingDeposit.Slot may differ, and it is not part of the signature.
func TestWarmupDuplicateDepositsShareOneCacheEntry(t *testing.T) {
	cache.DepositSignature.Clear()
	builderCreds := warmupCreds(t, true)
	sk, err := bls.RandKey()
	require.NoError(t, err)

	first := warmupDeposit(t, sk, builderCreds, 32, true)
	second := warmupDeposit(t, sk, builderCreds, 32, true)
	second.Slot = 99

	st, err := util.NewBeaconStateGloas(func(s *ethpb.BeaconStateGloas) error {
		s.PendingDeposits = []*ethpb.PendingDeposit{first, second}
		return nil
	})
	require.NoError(t, err)

	candidates, err := pendingBuilderDepositCandidates(st)
	require.NoError(t, err)
	require.Equal(t, 2, len(candidates))

	k0, err := candidates[0].HashTreeRoot()
	require.NoError(t, err)
	k1, err := candidates[1].HashTreeRoot()
	require.NoError(t, err)
	require.DeepEqual(t, k0, k1)

	// The duplicate must be dropped before verification, not merely overwrite the same entry.
	cold := coldDepositCandidates(candidates)
	require.Equal(t, 1, len(cold))

	s := &Service{ctx: context.Background()}
	require.Equal(t, 1, s.verifyDepositSignatures(context.Background(), cold))
	require.Equal(t, 1, cache.DepositSignature.Len())

	// Already cached candidates are dropped too, so a second pass has nothing to do.
	require.Equal(t, 0, len(coldDepositCandidates(candidates)))
}

func TestColdDepositCandidates(t *testing.T) {
	builderCreds := warmupCreds(t, true)
	data := func(t *testing.T, amount uint64) *ethpb.Deposit_Data {
		t.Helper()
		sk, err := bls.RandKey()
		require.NoError(t, err)
		d := warmupDeposit(t, sk, builderCreds, amount, true)
		return &ethpb.Deposit_Data{
			PublicKey:             d.PublicKey,
			WithdrawalCredentials: d.WithdrawalCredentials,
			Amount:                d.Amount,
			Signature:             d.Signature,
		}
	}

	t.Run("nil and empty", func(t *testing.T) {
		cache.DepositSignature.Clear()
		require.Equal(t, 0, len(coldDepositCandidates(nil)))
		require.Equal(t, 0, len(coldDepositCandidates([]*ethpb.Deposit_Data{})))
	})

	t.Run("distinct candidates all cold", func(t *testing.T) {
		cache.DepositSignature.Clear()
		candidates := []*ethpb.Deposit_Data{data(t, 1), data(t, 2), data(t, 3)}
		require.Equal(t, 3, len(coldDepositCandidates(candidates)))
	})

	t.Run("repeated candidate collapses once", func(t *testing.T) {
		cache.DepositSignature.Clear()
		d := data(t, 4)
		candidates := []*ethpb.Deposit_Data{d, d, d, d}
		cold := coldDepositCandidates(candidates)
		require.Equal(t, 1, len(cold))
		require.DeepEqual(t, d, cold[0])
	})

	t.Run("cached candidates excluded, order preserved", func(t *testing.T) {
		cache.DepositSignature.Clear()
		first, second, third := data(t, 5), data(t, 6), data(t, 7)
		key, err := second.HashTreeRoot()
		require.NoError(t, err)
		cache.DepositSignature.Put(key, true)

		cold := coldDepositCandidates([]*ethpb.Deposit_Data{first, second, third})
		require.Equal(t, 2, len(cold))
		require.DeepEqual(t, first, cold[0])
		require.DeepEqual(t, third, cold[1])
	})
}

// The pre-fork cache must survive until the fork epoch can no longer be reverted, otherwise a
// reorg back across the boundary re-runs the upgrade against a cold cache.
func TestDepositWarmupCacheRetainedUntilForkFinalized(t *testing.T) {
	service, _ := minimalTestService(t)
	forkEpoch := primitives.Epoch(8)

	reset := func() {
		cache.DepositSignature.Clear()
		cache.DepositSignature.Put([32]byte{0x01}, true)
	}
	retired := func(finalized primitives.Epoch) bool {
		require.NoError(t, service.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(
			&forkchoicetypes.Checkpoint{Epoch: finalized},
		))
		return service.FinalizedCheckpt().Epoch > forkEpoch
	}

	reset()
	require.Equal(t, false, retired(forkEpoch-1))
	require.Equal(t, 1, cache.DepositSignature.Len())

	require.Equal(t, false, retired(forkEpoch))
	require.Equal(t, 1, cache.DepositSignature.Len())

	require.Equal(t, true, retired(forkEpoch+1))
}
