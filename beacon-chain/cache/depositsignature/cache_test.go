package depositsignature_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache/depositsignature"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestCacheVerifyBatch(t *testing.T) {
	valid := pendingDeposit(t, true)
	invalid := pendingDeposit(t, false)
	cache := depositsignature.New()

	validity, err := cache.VerifyBatch(t.Context(), []*ethpb.PendingDeposit{valid, invalid})
	require.NoError(t, err)
	require.DeepEqual(t, []bool{true, false}, validity)
	require.Equal(t, 2, cache.Len())

	got, ok := cache.Get(valid)
	require.Equal(t, true, ok)
	require.Equal(t, true, got)
	got, ok = cache.Get(invalid)
	require.Equal(t, true, ok)
	require.Equal(t, false, got)
}

func TestCacheCopyFromAndClear(t *testing.T) {
	deposit := pendingDeposit(t, true)
	source := depositsignature.New()
	source.TrackBuilderPubkey(deposit.PublicKey)
	source.MarkValidValidator(deposit.PublicKey)
	valid, err := source.Verify(deposit)
	require.NoError(t, err)
	require.Equal(t, true, valid)

	target := depositsignature.New()
	target.CopyFrom(source)
	valid, ok := target.Get(deposit)
	require.Equal(t, true, ok)
	require.Equal(t, true, valid)
	require.Equal(t, false, target.ShouldVerifyValidator(deposit.PublicKey))
	require.Equal(t, 1, target.BuilderPubkeyLen())

	target.Clear()
	require.Equal(t, 0, target.Len())
	require.Equal(t, 0, target.BuilderPubkeyLen())
	require.Equal(t, 1, source.Len())
}

func TestCacheTracksBuilderValidatorWork(t *testing.T) {
	deposit := pendingDeposit(t, true)
	cache := depositsignature.New()
	require.Equal(t, false, cache.ShouldVerifyValidator(deposit.PublicKey))

	cache.TrackBuilderPubkey(deposit.PublicKey)
	require.Equal(t, true, cache.ShouldVerifyValidator(deposit.PublicKey))
	require.Equal(t, 1, cache.BuilderPubkeyLen())

	cache.MarkValidValidator(deposit.PublicKey)
	require.Equal(t, false, cache.ShouldVerifyValidator(deposit.PublicKey))
	cache.TrackBuilderPubkey(deposit.PublicKey)
	require.Equal(t, false, cache.ShouldVerifyValidator(deposit.PublicKey))
}

func pendingDeposit(t *testing.T, valid bool) *ethpb.PendingDeposit {
	t.Helper()
	sk, err := bls.RandKey()
	require.NoError(t, err)
	withdrawalCredentials := make([]byte, fieldparams.RootLength)
	amount := uint64(32_000_000_000)
	signature := make([]byte, fieldparams.BLSSignatureLength)
	if valid {
		domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
		require.NoError(t, err)
		root, err := signing.ComputeSigningRoot(&ethpb.DepositMessage{
			PublicKey:             sk.PublicKey().Marshal(),
			WithdrawalCredentials: withdrawalCredentials,
			Amount:                amount,
		}, domain)
		require.NoError(t, err)
		signature = sk.Sign(root[:]).Marshal()
	}
	return &ethpb.PendingDeposit{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: withdrawalCredentials,
		Amount:                amount,
		Signature:             signature,
	}
}
