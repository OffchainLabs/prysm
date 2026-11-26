package client

import (
	"context"
	"errors"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
	dbTest "github.com/OffchainLabs/prysm/v7/validator/db/testing"
	"github.com/OffchainLabs/prysm/v7/validator/pqkeys"
	"github.com/OffchainLabs/prysm/v7/validator/xmsssigner"
	"go.uber.org/mock/gomock"
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

			keys, err := newPQKeys(t.Context(), tc.vrfStateURL, "", "", nil)
			if tc.expectedErr != "" {
				require.ErrorContains(t, tc.expectedErr, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectKeys, keys != nil)
		})
	}
}

func TestRegistryXMSSPubkeys(t *testing.T) {
	xmssKey := func(b byte) [fieldparams.XMSSPubkeyLength]byte {
		var key [fieldparams.XMSSPubkeyLength]byte
		key[0], key[fieldparams.XMSSPubkeyLength-1] = b, b

		return key
	}

	// container returns the validator `index` whose public key is the XMSS key of `index`, as `size` bytes.
	container := func(index, size int) *ethpb.Validators_ValidatorContainer {
		key := xmssKey(byte(index))
		pubkey := append(make([]byte, size-fieldparams.XMSSPubkeyLength), key[:]...)

		return &ethpb.Validators_ValidatorContainer{
			Index:     primitives.ValidatorIndex(index),
			Validator: &ethpb.Validator{PublicKey: pubkey},
		}
	}

	t.Run("pages and key sizes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		chainClient := validatormock.NewMockChainClient(ctrl)
		chainClient.EXPECT().Validators(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, request *ethpb.ListValidatorsRequest) (*ethpb.Validators, error) {
				require.Equal(t, int32(3), request.PageSize)
				if request.PageToken == "" {
					return &ethpb.Validators{
						ValidatorList: []*ethpb.Validators_ValidatorContainer{container(0, fieldparams.XMSSPubkeyLength), container(1, fieldparams.BLSPubkeyLength)},
						NextPageToken: "1",
					}, nil
				}

				require.Equal(t, "1", request.PageToken)
				return &ethpb.Validators{
					ValidatorList: []*ethpb.Validators_ValidatorContainer{container(2, fieldparams.XMSSPubkeyLength), container(3, fieldparams.XMSSPubkeyLength)},
				}, nil
			}).Times(2)

		pubkeys, err := registryXMSSPubkeys(chainClient)(t.Context(), 3)
		require.NoError(t, err)
		require.DeepEqual(t, [][fieldparams.XMSSPubkeyLength]byte{xmssKey(0), xmssKey(1), xmssKey(2)}, pubkeys)
	})

	t.Run("registry smaller than the count", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		chainClient := validatormock.NewMockChainClient(ctrl)
		chainClient.EXPECT().Validators(gomock.Any(), gomock.Any()).Return(&ethpb.Validators{
			ValidatorList: []*ethpb.Validators_ValidatorContainer{container(0, fieldparams.XMSSPubkeyLength)},
		}, nil)

		pubkeys, err := registryXMSSPubkeys(chainClient)(t.Context(), 10)
		require.NoError(t, err)
		require.DeepEqual(t, [][fieldparams.XMSSPubkeyLength]byte{xmssKey(0)}, pubkeys)
	})

	for _, tc := range []struct {
		name        string
		validators  *ethpb.Validators
		err         error
		expectedErr string
	}{
		{
			name:        "beacon node error",
			err:         errors.New("unreachable"),
			expectedErr: "unreachable",
		},
		{
			name:        "missing index",
			validators:  &ethpb.Validators{ValidatorList: []*ethpb.Validators_ValidatorContainer{container(1, fieldparams.XMSSPubkeyLength)}},
			expectedErr: "validator at index 0 missing",
		},
		{
			name:        "public key of another size",
			validators:  &ethpb.Validators{ValidatorList: []*ethpb.Validators_ValidatorContainer{{Validator: &ethpb.Validator{PublicKey: make([]byte, fieldparams.BLSPubkeyLength-1)}}}},
			expectedErr: "public key of 47 bytes",
		},
		{
			name: "48 bytes without the zero prefix",
			validators: &ethpb.Validators{ValidatorList: []*ethpb.Validators_ValidatorContainer{{
				Validator: &ethpb.Validator{PublicKey: append([]byte{1}, make([]byte, fieldparams.BLSPubkeyLength-1)...)},
			}}},
			expectedErr: "is not an XMSS public key",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			chainClient := validatormock.NewMockChainClient(ctrl)
			chainClient.EXPECT().Validators(gomock.Any(), gomock.Any()).Return(tc.validators, tc.err)

			_, err := registryXMSSPubkeys(chainClient)(t.Context(), 2)
			require.ErrorContains(t, tc.expectedErr, err)
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
		db := dbTest.SetupDB(t, t.TempDir(), nil, false)
		require.NoError(t, db.SaveGenesisValidatorsRoot(t.Context(), make([]byte, fieldparams.RootLength)))
		v := &validator{km: km, db: db, pqKeys: pqkeys.NewRegistry(t.Context(), nil)}

		require.NoError(t, v.loadPQKeys(t.Context()))

		keys, ok := v.pqKeys.Keys(pair.pub)
		require.Equal(t, true, ok)

		xmssKey, ok := v.pqKeys.XMSSPubkey(pair.pub)
		require.Equal(t, true, ok)
		require.DeepEqual(t, keys.XMSS.PublicKey().Bytes(), xmssKey[:])

		// The XMSS signer can sign with the key.
		require.DeepEqual(t, [][fieldparams.XMSSPubkeyLength]byte{xmssKey}, v.pqSigner.PublicKeys())

		// Loading again does not add the key twice.
		require.NoError(t, v.loadPQKeys(t.Context()))
		require.Equal(t, 1, len(v.pqSigner.PublicKeys()))
	})

	t.Run("no genesis validators root", func(t *testing.T) {
		km := &pqTestKeymanager{mockKeymanager: newMockKeymanager(t, randKeypair(t))}
		v := &validator{km: km, db: dbTest.SetupDB(t, t.TempDir(), nil, false), pqKeys: pqkeys.NewRegistry(t.Context(), nil)}

		err := v.loadPQKeys(t.Context())
		require.ErrorContains(t, "genesis validators root", err)
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

func TestXMSSLeavesToPrepare(t *testing.T) {
	const treeLeaves = uint64(1) << xmssBottomTreeHeight

	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	epochsPerSlotTree := primitives.Epoch(treeLeaves / xmsssigner.LeavesPerSlot / slotsPerEpoch)

	// The slot leaves do not start at the start of a bottom tree: find the first epoch starting a slot tree.
	firstSlotLeaf, err := xmsssigner.SlotLeafIndex(0, 0)
	require.NoError(t, err)

	firstTreeSlot := (treeLeaves - uint64(firstSlotLeaf)%treeLeaves) % treeLeaves / xmsssigner.LeavesPerSlot
	require.Equal(t, uint64(0), firstTreeSlot%slotsPerEpoch)
	firstTreeEpoch := primitives.Epoch(firstTreeSlot / slotsPerEpoch)

	// slotLeaves returns the first and the last slot leaves of `epoch`.
	slotLeaves := func(t *testing.T, epoch primitives.Epoch) []uint32 {
		first, err := xmsssigner.SlotLeafIndex(primitives.Slot(uint64(epoch)*slotsPerEpoch), 0)
		require.NoError(t, err)

		last, err := xmsssigner.SlotLeafIndex(primitives.Slot((uint64(epoch)+1)*slotsPerEpoch-1), signingprotection.SlotMessage(xmsssigner.LeavesPerSlot-1))
		require.NoError(t, err)

		require.Equal(t, xmssBottomTree(first), xmssBottomTree(last))

		return []uint32{first, last}
	}

	t.Run("same trees", func(t *testing.T) {
		for _, epoch := range []primitives.Epoch{0, 1, firstTreeEpoch, firstTreeEpoch + epochsPerSlotTree - 2} {
			leaves, err := xmssLeavesToPrepare(epoch, epoch+1)
			require.NoError(t, err)
			require.Equal(t, 0, len(leaves))
		}
	})

	t.Run("next slot tree", func(t *testing.T) {
		for _, epoch := range []primitives.Epoch{firstTreeEpoch - 1, firstTreeEpoch + epochsPerSlotTree - 1} {
			leaves, err := xmssLeavesToPrepare(epoch, epoch+1)
			require.NoError(t, err)
			require.DeepEqual(t, slotLeaves(t, epoch+1)[:1], leaves)
		}
	})

	t.Run("next attestation tree", func(t *testing.T) {
		// The attestation leaf of epoch 2^15 is 2^16: the first leaf of a bottom tree.
		const epoch = primitives.Epoch(1<<15 - 1)

		attestation, err := xmsssigner.AttestationLeafIndex(epoch + 1)
		require.NoError(t, err)
		require.Equal(t, uint32(treeLeaves), attestation)

		want := []uint32{attestation}
		if (epoch+1-firstTreeEpoch)%epochsPerSlotTree == 0 {
			want = append(want, slotLeaves(t, epoch+1)[0])
		}

		leaves, err := xmssLeavesToPrepare(epoch, epoch+1)
		require.NoError(t, err)
		require.DeepEqual(t, want, leaves)
	})

	t.Run("beyond the leaves", func(t *testing.T) {
		_, err := xmssLeavesToPrepare(primitives.Epoch(xmsssigner.AttestationLeafCount-1), primitives.Epoch(xmsssigner.AttestationLeafCount))
		require.ErrorContains(t, "beyond", err)
	})
}
