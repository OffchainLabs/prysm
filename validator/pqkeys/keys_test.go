package pqkeys

import (
	"context"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Test vector of Lighthouse's `pq_ethpandaops_keys_and_vrf_tree_boundaries`.
const (
	vectorSecretKey  = "0x2892f3ba475c3640a67fe730798ad9c4eed3a6d934db85f91c41cfc837ce8834"
	vectorXMSSPubkey = "0x155a29b971da2b771fdf52c768af63fa03f78f25487e3e0ed03d012b7039d88b"
)

func vectorKey(t *testing.T) bls.SecretKey {
	secretKey, err := bls.SecretKeyFromBytes(hexutil.MustDecode(vectorSecretKey))
	require.NoError(t, err)

	return secretKey
}

func requireVectorKeys(t *testing.T, keys *Keys) {
	require.DeepEqual(t, hexutil.MustDecode(vectorXMSSPubkey), keys.XMSS.PublicKey().Bytes())
}

func TestDerive(t *testing.T) {
	t.Run("nil secret key", func(t *testing.T) {
		_, err := Derive(t.Context(), nil)
		require.ErrorContains(t, "nil secret key", err)
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := Derive(ctx, vectorKey(t))
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("matches the genesis tool and Lighthouse", func(t *testing.T) {
		keys, err := Derive(t.Context(), vectorKey(t))
		require.NoError(t, err)
		requireVectorKeys(t, keys)
	})
}

func TestDeriveAll(t *testing.T) {
	t.Run("nil secret key", func(t *testing.T) {
		_, err := DeriveAll(t.Context(), []bls.SecretKey{nil})
		require.ErrorContains(t, "nil secret key at index 0", err)
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := DeriveAll(ctx, []bls.SecretKey{vectorKey(t)})
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("no secret key", func(t *testing.T) {
		keysByPubkey, err := DeriveAll(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, 0, len(keysByPubkey))
	})

	t.Run("indexes by public key and skips duplicates", func(t *testing.T) {
		secretKey := vectorKey(t)

		keysByPubkey, err := DeriveAll(t.Context(), []bls.SecretKey{secretKey, secretKey})
		require.NoError(t, err)
		require.Equal(t, 1, len(keysByPubkey))

		keys, ok := keysByPubkey[[fieldparams.BLSPubkeyLength]byte(secretKey.PublicKey().Marshal())]
		require.Equal(t, true, ok)
		requireVectorKeys(t, keys)
	})
}
