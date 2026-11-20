package client

import (
	"context"
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/pqkeys"
)

func TestNewPQKeys(t *testing.T) {
	for _, tc := range []struct {
		name        string
		forkEpoch   func(*params.BeaconChainConfig) primitives.Epoch
		vrfStateURL string
		expectKeys  bool
		expectedErr string
	}{
		{
			name:      "PQ fork not scheduled",
			forkEpoch: func(config *params.BeaconChainConfig) primitives.Epoch { return config.FarFutureEpoch },
		},
		{
			name:       "PQ fork at genesis",
			forkEpoch:  func(*params.BeaconChainConfig) primitives.Epoch { return 0 },
			expectKeys: true,
		},
		{
			name:        "PQ fork at genesis with a VRF state",
			forkEpoch:   func(*params.BeaconChainConfig) primitives.Epoch { return 0 },
			vrfStateURL: "https://example.com/vrf.bin",
			expectKeys:  true,
		},
		{
			name:        "PQ fork at genesis with an invalid VRF state URL",
			forkEpoch:   func(*params.BeaconChainConfig) primitives.Epoch { return 0 },
			vrfStateURL: "ftp://example.com/vrf.bin",
			expectedErr: "VRF state",
		},
		{
			name:        "PQ fork after genesis",
			forkEpoch:   func(*params.BeaconChainConfig) primitives.Epoch { return 10 },
			expectedErr: "only a PQ fork at genesis is supported",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			config := params.BeaconConfig().Copy()
			config.PQForkEpoch = tc.forkEpoch(config)
			params.OverrideBeaconConfig(config)

			keys, err := newPQKeys(t.Context(), tc.vrfStateURL)
			if tc.expectedErr != "" {
				require.ErrorContains(t, tc.expectedErr, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectKeys, keys != nil)
		})
	}
}

func TestValidator_LoadPQKeys(t *testing.T) {
	t.Run("not in the PQ fork", func(t *testing.T) {
		v := &validator{km: newMockKeymanager(t, randKeypair(t))}
		require.NoError(t, v.loadPQKeys(t.Context()))
	})

	t.Run("keymanager without secret keys", func(t *testing.T) {
		v := &validator{km: newMockKeymanager(t, randKeypair(t)), pqKeys: pqkeys.NewRegistry(t.Context(), nil)}
		err := v.loadPQKeys(t.Context())
		require.ErrorIs(t, err, errPQRequiresLocalKeys)
	})

	t.Run("secret keys not fetched", func(t *testing.T) {
		km := &pqTestKeymanager{mockKeymanager: newMockKeymanager(t, randKeypair(t)), err: errors.New("boom")}
		v := &validator{km: km, pqKeys: pqkeys.NewRegistry(t.Context(), nil)}
		err := v.loadPQKeys(t.Context())
		require.ErrorContains(t, "boom", err)
	})

	t.Run("derives the keys", func(t *testing.T) {
		pair := randKeypair(t)
		km := &pqTestKeymanager{mockKeymanager: newMockKeymanager(t, pair)}
		v := &validator{km: km, pqKeys: pqkeys.NewRegistry(t.Context(), nil)}

		require.NoError(t, v.loadPQKeys(t.Context()))

		keys, ok := v.pqKeys.Keys(pair.pub)
		require.Equal(t, true, ok)

		xmssKey, ok := v.pqKeys.XMSSPubkey(pair.pub)
		require.Equal(t, true, ok)
		require.DeepEqual(t, keys.XMSS.PublicKey().Bytes(), xmssKey[:])
	})
}

// pqTestKeymanager is a mock keymanager that hands out its BLS secret keys, as the local keymanager does.
type pqTestKeymanager struct {
	*mockKeymanager
	err error
}

func (m *pqTestKeymanager) FetchValidatingPrivateKeys(ctx context.Context) ([][32]byte, error) {
	if m.err != nil {
		return nil, m.err
	}

	pubkeys, err := m.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return nil, err
	}

	secretKeys := make([][32]byte, 0, len(pubkeys))
	for _, pubkey := range pubkeys {
		secretKeys = append(secretKeys, [32]byte(m.keysMap[pubkey].Marshal()))
	}

	return secretKeys, nil
}
