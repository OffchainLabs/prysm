package pqkeys

import (
	"context"
	"errors"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/vrftree"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestRegistry_Load(t *testing.T) {
	t.Run("nil secret key", func(t *testing.T) {
		err := NewRegistry(t.Context(), nil).Load(t.Context(), []bls.SecretKey{nil})
		require.ErrorContains(t, "nil secret key at index 0", err)
	})

	t.Run("derivation error", func(t *testing.T) {
		registry := NewRegistry(t.Context(), nil)
		registry.deriveKeys = func(context.Context, []bls.SecretKey) (map[blsPubkey]*Keys, error) {
			return nil, errors.New("boom")
		}

		err := registry.Load(t.Context(), []bls.SecretKey{vectorKey(t)})
		require.ErrorContains(t, "boom", err)
	})

	t.Run("derives the missing keys only", func(t *testing.T) {
		registry := NewRegistry(t.Context(), nil)

		derivations := 0
		registry.deriveKeys = func(ctx context.Context, secretKeys []bls.SecretKey) (map[blsPubkey]*Keys, error) {
			derivations += len(secretKeys)
			return DeriveAll(ctx, secretKeys)
		}

		secretKey := vectorKey(t)
		require.NoError(t, registry.Load(t.Context(), []bls.SecretKey{secretKey}))
		require.NoError(t, registry.Load(t.Context(), []bls.SecretKey{secretKey}))
		require.Equal(t, 1, derivations)
	})
}

func TestRegistry_Lookups(t *testing.T) {
	registry := NewRegistry(t.Context(), nil)
	secretKey := vectorKey(t)
	require.NoError(t, registry.Load(t.Context(), []bls.SecretKey{secretKey}))

	blsKey := blsPubkey(secretKey.PublicKey().Marshal())
	xmssKey := xmssPubkey(hexutil.MustDecode(vectorXMSSPubkey))

	t.Run("keys", func(t *testing.T) {
		keys, ok := registry.Keys(blsKey)
		require.Equal(t, true, ok)
		requireVectorKeys(t, keys)

		_, ok = registry.Keys(blsPubkey{})
		require.Equal(t, false, ok)
	})

	t.Run("XMSS public key", func(t *testing.T) {
		actual, ok := registry.XMSSPubkey(blsKey)
		require.Equal(t, true, ok)
		require.Equal(t, xmssKey, actual)

		_, ok = registry.XMSSPubkey(blsPubkey{})
		require.Equal(t, false, ok)
	})

	t.Run("BLS public key", func(t *testing.T) {
		actual, ok := registry.BLSPubkey(xmssKey)
		require.Equal(t, true, ok)
		require.Equal(t, blsKey, actual)

		_, ok = registry.BLSPubkey([fieldparams.XMSSPubkeyLength]byte{})
		require.Equal(t, false, ok)
	})
}

func TestRegistry_VRF(t *testing.T) {
	secretKey := vectorKey(t)
	blsKey := blsPubkey(secretKey.PublicKey().Marshal())
	root := [32]byte{0x01}
	record := VRFRecord{Root: root, Branch: [vrftree.Depth][32]byte{{0x02}}}

	// Any tree does: the registry hands out the tree `newVRFTree` builds.
	tree, err := vrftree.New(t.Context(), secretKey)
	require.NoError(t, err)

	// newTestRegistry returns a registry holding the keys of `secretKey`, reading the VRF state from `source`,
	// and counting the trees it builds in `builds`.
	newTestRegistry := func(t *testing.T, source VRFStateSource, builds *int, buildErr error) *Registry {
		registry := NewRegistry(t.Context(), source)
		require.NoError(t, registry.Load(t.Context(), []bls.SecretKey{secretKey}))

		registry.newVRFTree = func(_ context.Context, actualKey bls.SecretKey, actualRoot [32]byte, branch [vrftree.Depth][32]byte) (*vrftree.Tree, error) {
			*builds++
			require.DeepEqual(t, secretKey.Marshal(), actualKey.Marshal())
			require.Equal(t, root, actualRoot)
			require.Equal(t, record.Branch, branch)
			return tree, buildErr
		}

		return registry
	}

	// set sets the VRF root as `SetVRFRoot` does, but builds the pending trees synchronously.
	set := func(t *testing.T, registry *Registry, pubkey blsPubkey, root [32]byte) {
		if registry.setVRFRoot(pubkey, root) {
			registry.prepareVRF(t.Context())
		}
	}

	t.Run("builds the tree once", func(t *testing.T) {
		source := &testVRFStateSource{records: map[[32]byte]VRFRecord{root: record}}
		builds := 0
		registry := newTestRegistry(t, source, &builds, nil)

		_, ok := registry.VRF(blsKey)
		require.Equal(t, false, ok)

		set(t, registry, blsKey, root)
		actual, ok := registry.VRF(blsKey)
		require.Equal(t, true, ok)
		require.Equal(t, tree, actual)

		set(t, registry, blsKey, root)
		require.Equal(t, 1, builds)
		require.Equal(t, 1, source.reads)
	})

	t.Run("unknown validator", func(t *testing.T) {
		source := &testVRFStateSource{records: map[[32]byte]VRFRecord{root: record}}
		builds := 0
		registry := newTestRegistry(t, source, &builds, nil)

		set(t, registry, blsPubkey{}, root)
		_, ok := registry.VRF(blsPubkey{})
		require.Equal(t, false, ok)
		require.Equal(t, 0, source.reads)
	})

	t.Run("no VRF state", func(t *testing.T) {
		builds := 0
		registry := newTestRegistry(t, nil, &builds, nil)

		set(t, registry, blsKey, root)
		_, ok := registry.VRF(blsKey)
		require.Equal(t, false, ok)
		require.Equal(t, 0, builds)
	})

	t.Run("root missing from the VRF state is not read again", func(t *testing.T) {
		source := &testVRFStateSource{}
		builds := 0
		registry := newTestRegistry(t, source, &builds, nil)

		set(t, registry, blsKey, root)
		set(t, registry, blsKey, root)
		_, ok := registry.VRF(blsKey)
		require.Equal(t, false, ok)
		require.Equal(t, 1, source.reads)
		require.Equal(t, 0, builds)
	})

	t.Run("tree not matching the VRF state is not built again", func(t *testing.T) {
		source := &testVRFStateSource{records: map[[32]byte]VRFRecord{root: record}}
		builds := 0
		registry := newTestRegistry(t, source, &builds, errors.New("mismatch"))

		set(t, registry, blsKey, root)
		set(t, registry, blsKey, root)
		_, ok := registry.VRF(blsKey)
		require.Equal(t, false, ok)
		require.Equal(t, 1, builds)
	})

	t.Run("VRF state not read is read again", func(t *testing.T) {
		source := &testVRFStateSource{err: errors.New("unreachable")}
		builds := 0
		registry := newTestRegistry(t, source, &builds, nil)

		set(t, registry, blsKey, root)
		_, ok := registry.VRF(blsKey)
		require.Equal(t, false, ok)

		source.err = nil
		source.records = map[[32]byte]VRFRecord{root: record}
		set(t, registry, blsKey, root)
		_, ok = registry.VRF(blsKey)
		require.Equal(t, true, ok)
		require.Equal(t, 2, source.reads)
	})

	t.Run("changed root", func(t *testing.T) {
		source := &testVRFStateSource{records: map[[32]byte]VRFRecord{root: record}}
		builds := 0
		registry := newTestRegistry(t, source, &builds, nil)

		set(t, registry, blsKey, root)
		set(t, registry, blsKey, [32]byte{0x03}) // Missing from the VRF state.
		_, ok := registry.VRF(blsKey)
		require.Equal(t, false, ok)
		require.Equal(t, 2, source.reads)
	})

	t.Run("in the background", func(t *testing.T) {
		source := &testVRFStateSource{records: map[[32]byte]VRFRecord{root: record}}
		builds := 0
		registry := newTestRegistry(t, source, &builds, nil)

		registry.SetVRFRoot(blsKey, root)
		require.Eventually(t, func() bool {
			_, ok := registry.VRF(blsKey)
			return ok
		}, 10*time.Second, 10*time.Millisecond)
	})
}

// testVRFStateSource is a VRF state holding `records`, or failing with `err`. It counts its reads.
type testVRFStateSource struct {
	records map[[32]byte]VRFRecord
	err     error
	reads   int
}

func (s *testVRFStateSource) Records(_ context.Context, roots map[[32]byte]bool) (map[[32]byte]VRFRecord, error) {
	s.reads++
	if s.err != nil {
		return nil, s.err
	}

	records := make(map[[32]byte]VRFRecord)
	for root := range roots {
		if record, ok := s.records[root]; ok {
			records[root] = record
		}
	}

	return records, nil
}
