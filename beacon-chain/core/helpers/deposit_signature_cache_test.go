package helpers

import (
	"math/rand"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func signedPendingDeposit(t *testing.T, sk bls.SecretKey, creds []byte, amount uint64, valid bool) *ethpb.PendingDeposit {
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

func depositCreds(prefix byte) []byte {
	creds := make([]byte, fieldparams.RootLength)
	creds[0] = prefix
	return creds
}

func TestIsValidDepositSignatureUsesCache(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	creds := depositCreds(params.BeaconConfig().BLSWithdrawalPrefixByte)

	t.Run("valid signature cached", func(t *testing.T) {
		cache.DepositSignature.Clear()
		deposit := signedPendingDeposit(t, sk, creds, 32, true)
		data := &ethpb.Deposit_Data{
			PublicKey:             deposit.PublicKey,
			WithdrawalCredentials: deposit.WithdrawalCredentials,
			Amount:                deposit.Amount,
			Signature:             deposit.Signature,
		}
		valid, err := IsValidDepositSignature(data)
		require.NoError(t, err)
		require.Equal(t, true, valid)
		require.Equal(t, 1, cache.DepositSignature.Len())

		key, err := data.HashTreeRoot()
		require.NoError(t, err)
		cached, ok := cache.DepositSignature.Get(key)
		require.Equal(t, true, ok)
		require.Equal(t, true, cached)

		valid, err = IsValidDepositSignature(data)
		require.NoError(t, err)
		require.Equal(t, true, valid)
		require.Equal(t, 1, cache.DepositSignature.Len())
	})

	t.Run("invalid signature cached as false", func(t *testing.T) {
		cache.DepositSignature.Clear()
		deposit := signedPendingDeposit(t, sk, creds, 32, false)
		data := &ethpb.Deposit_Data{
			PublicKey:             deposit.PublicKey,
			WithdrawalCredentials: deposit.WithdrawalCredentials,
			Amount:                deposit.Amount,
			Signature:             deposit.Signature,
		}
		valid, err := IsValidDepositSignature(data)
		require.NoError(t, err)
		require.Equal(t, false, valid)

		key, err := data.HashTreeRoot()
		require.NoError(t, err)
		cached, ok := cache.DepositSignature.Get(key)
		require.Equal(t, true, ok)
		require.Equal(t, false, cached)
	})

	t.Run("amount is part of the key", func(t *testing.T) {
		cache.DepositSignature.Clear()
		deposit := signedPendingDeposit(t, sk, creds, 32, true)
		base := &ethpb.Deposit_Data{
			PublicKey:             deposit.PublicKey,
			WithdrawalCredentials: deposit.WithdrawalCredentials,
			Amount:                deposit.Amount,
			Signature:             deposit.Signature,
		}
		valid, err := IsValidDepositSignature(base)
		require.NoError(t, err)
		require.Equal(t, true, valid)

		tampered := &ethpb.Deposit_Data{
			PublicKey:             deposit.PublicKey,
			WithdrawalCredentials: deposit.WithdrawalCredentials,
			Amount:                deposit.Amount + 1,
			Signature:             deposit.Signature,
		}
		valid, err = IsValidDepositSignature(tampered)
		require.NoError(t, err)
		require.Equal(t, false, valid)
		require.Equal(t, 2, cache.DepositSignature.Len())
	})

	t.Run("malformed inputs are invalid not errors", func(t *testing.T) {
		cache.DepositSignature.Clear()
		cases := []struct {
			name string
			data *ethpb.Deposit_Data
		}{
			{
				name: "garbage pubkey",
				data: &ethpb.Deposit_Data{
					PublicKey:             make([]byte, fieldparams.BLSPubkeyLength),
					WithdrawalCredentials: creds,
					Amount:                32,
					Signature:             make([]byte, fieldparams.BLSSignatureLength),
				},
			},
			{
				name: "zero signature",
				data: &ethpb.Deposit_Data{
					PublicKey:             sk.PublicKey().Marshal(),
					WithdrawalCredentials: creds,
					Amount:                32,
					Signature:             make([]byte, fieldparams.BLSSignatureLength),
				},
			},
			{
				name: "infinite signature",
				data: &ethpb.Deposit_Data{
					PublicKey:             sk.PublicKey().Marshal(),
					WithdrawalCredentials: creds,
					Amount:                32,
					Signature:             common.InfiniteSignature[:],
				},
			},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				valid, err := IsValidDepositSignature(tt.data)
				require.NoError(t, err)
				require.Equal(t, false, valid)
			})
		}
	})
}

// Pins the removal of the quadratic rescan: a second HasValid call must examine nothing, even
// with the signature cache emptied underneath it.
func TestPendingValidatorIndexDoesNotRescan(t *testing.T) {
	const k = 32
	sk, err := bls.RandKey()
	require.NoError(t, err)
	creds := depositCreds(params.BeaconConfig().BLSWithdrawalPrefixByte)

	cache.DepositSignature.Clear()
	idx := NewPendingValidatorIndex()
	for i := range k {
		deposit := signedPendingDeposit(t, sk, creds, 32, false)
		deposit.Signature[0] = byte(i + 1)
		idx.Add(deposit)
	}

	has, err := idx.HasValid(sk.PublicKey().Marshal())
	require.NoError(t, err)
	require.Equal(t, false, has)
	require.Equal(t, k, cache.DepositSignature.Len())

	cache.DepositSignature.Clear()
	has, err = idx.HasValid(sk.PublicKey().Marshal())
	require.NoError(t, err)
	require.Equal(t, false, has)
	require.Equal(t, 0, cache.DepositSignature.Len())
}

func TestPendingValidatorIndexStopsAtFirstValid(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	creds := depositCreds(params.BeaconConfig().BLSWithdrawalPrefixByte)

	cache.DepositSignature.Clear()
	idx := NewPendingValidatorIndex()
	invalid := signedPendingDeposit(t, sk, creds, 32, false)
	idx.Add(invalid)
	idx.Add(signedPendingDeposit(t, sk, creds, 32, true))
	trailing := signedPendingDeposit(t, sk, creds, 64, false)
	idx.Add(trailing)

	has, err := idx.HasValid(sk.PublicKey().Marshal())
	require.NoError(t, err)
	require.Equal(t, true, has)
	// The trailing deposit is never reached, so only the first two are verified.
	require.Equal(t, 2, cache.DepositSignature.Len())

	cache.DepositSignature.Clear()
	has, err = idx.HasValid(sk.PublicKey().Marshal())
	require.NoError(t, err)
	require.Equal(t, true, has)
	require.Equal(t, 0, cache.DepositSignature.Len())
}

func TestPendingValidatorIndexUnknownPubkey(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	idx := NewPendingValidatorIndex()
	has, err := idx.HasValid(sk.PublicKey().Marshal())
	require.NoError(t, err)
	require.Equal(t, false, has)
	idx.Add(nil)
}

// Differential test against the naive spec helper on every prefix of randomized queues.
func TestPendingValidatorIndexMatchesSpecHelper(t *testing.T) {
	keys := make([]bls.SecretKey, 4)
	for i := range keys {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		keys[i] = sk
	}
	creds := depositCreds(params.BeaconConfig().BLSWithdrawalPrefixByte)
	rng := rand.New(rand.NewSource(7))

	for range 20 {
		cache.DepositSignature.Clear()
		idx := NewPendingValidatorIndex()
		accumulated := make([]*ethpb.PendingDeposit, 0, 12)
		for range 12 {
			sk := keys[rng.Intn(len(keys))]
			deposit := signedPendingDeposit(t, sk, creds, uint64(1+rng.Intn(4)), rng.Intn(3) == 0)
			deposit.Signature[fieldparams.BLSSignatureLength-1] = byte(rng.Intn(256))
			accumulated = append(accumulated, deposit)
			idx.Add(deposit)

			for _, key := range keys {
				pubkey := key.PublicKey().Marshal()
				want, err := IsPendingValidator(accumulated, pubkey)
				require.NoError(t, err)
				got, err := idx.HasValid(pubkey)
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
		}
	}
}
