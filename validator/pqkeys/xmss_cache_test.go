package pqkeys

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
	leanxmss "github.com/nalepae/go-leanxmss"
)

const (
	// The leaves of the two bottom trees of a record: the first attestation leaf, and the first slot leaf.
	testAttestationLeaf = uint32(1 << 15)
	testSlotLeaf        = uint32(1<<15 + 1<<29 + 1<<30)

	// testValidatorIndex is the index of the validator of the vector key in the test tree caches.
	testValidatorIndex = primitives.ValidatorIndex(2)
)

// testXMSSKey returns the XMSS key of the vector key built from its seed, with the two bottom trees of a record, and
// its record. It builds them once.
var testXMSSKey = sync.OnceValues(func() (*leanxmss.PrivateKey, []byte) {
	key := leanxmss.NewKeyFromSeed(xmssSeed(hexutil.MustDecode(vectorSecretKey)))
	for _, leaf := range []uint32{testAttestationLeaf, testSlotLeaf} {
		if err := key.Prepare(leaf); err != nil {
			panic(err)
		}
	}

	return key, key.CacheBytes()
})

// testXMSSCaches returns tree caches with the record of the vector key at `testValidatorIndex`, and the fetcher of
// the validators of a state holding it at this index.
func testXMSSCaches(t *testing.T) ([]byte, XMSSPubkeysFetcher) {
	_, record := testXMSSKey()
	require.Equal(t, xmssCacheRecordSize, len(record))

	caches := make([]byte, 0, 4*xmssCacheRecordSize)
	for index := range primitives.ValidatorIndex(4) {
		if index == testValidatorIndex {
			caches = append(caches, record...)
			continue
		}

		caches = append(caches, make([]byte, xmssCacheRecordSize)...)
	}

	return caches, testXMSSPubkeys(map[primitives.ValidatorIndex]xmssPubkey{
		0:                  {0x01},
		testValidatorIndex: xmssPubkey(hexutil.MustDecode(vectorXMSSPubkey)),
	})
}

func testXMSSPubkeys(pubkeys map[primitives.ValidatorIndex]xmssPubkey) XMSSPubkeysFetcher {
	return func(context.Context) (map[primitives.ValidatorIndex]xmssPubkey, error) {
		return pubkeys, nil
	}
}

func testXMSSCacheFile(t *testing.T, caches []byte, pubkeys XMSSPubkeysFetcher) *XMSSCache {
	path := filepath.Join(t.TempDir(), "caches.xmss")
	require.NoError(t, os.WriteFile(path, caches, 0o600))

	cache, err := NewXMSSCache(path, pubkeys)
	require.NoError(t, err)

	return cache
}

// requireCachedVectorKey requires `keys` to hold the vector key only, signing as the key built from its seed.
func requireCachedVectorKey(t *testing.T, keys map[blsPubkey]*Keys) {
	require.Equal(t, 1, len(keys))

	loaded, ok := keys[blsPubkey(vectorKey(t).PublicKey().Marshal())]
	require.Equal(t, true, ok)
	requireVectorKeys(t, loaded)

	built, record := testXMSSKey()
	require.DeepEqual(t, record, loaded.XMSS.CacheBytes())

	message := [32]byte{0x42}
	for _, leaf := range []uint32{testAttestationLeaf, testSlotLeaf + 9} {
		expected, err := built.Sign(leaf, message)
		require.NoError(t, err)

		actual, err := loaded.XMSS.Sign(leaf, message)
		require.NoError(t, err)

		require.DeepEqual(t, expected.Bytes(), actual.Bytes())
		require.Equal(t, true, leanxmss.Verify(loaded.XMSS.PublicKey(), leaf, message, actual))
	}
}

func TestNewXMSSCache(t *testing.T) {
	pubkeys := testXMSSPubkeys(nil)

	_, err := NewXMSSCache("caches.xmss", nil)
	require.ErrorContains(t, "nil XMSS public keys fetcher", err)

	_, err = NewXMSSCache("", pubkeys)
	require.ErrorContains(t, "empty XMSS cache location", err)

	_, err = NewXMSSCache("ftp://example.com/caches.xmss", pubkeys)
	require.ErrorContains(t, `XMSS cache URL scheme "ftp" is neither http, https nor file`, err)

	cache, err := NewXMSSCache("file:///tmp/caches.xmss", pubkeys)
	require.NoError(t, err)
	require.Equal(t, "/tmp/caches.xmss", cache.path)

	cache, err = NewXMSSCache("https://example.com/caches.xmss", pubkeys)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/caches.xmss", cache.url)
}

func TestXMSSCache_Keys(t *testing.T) {
	caches, pubkeys := testXMSSCaches(t)
	secretKeys := []bls.SecretKey{vectorKey(t)}
	recordStart := int(testValidatorIndex) * xmssCacheRecordSize

	t.Run("file", func(t *testing.T) {
		cache := testXMSSCacheFile(t, caches, pubkeys)
		requireCachedVectorKey(t, cache.keys(t.Context(), secretKeys))
	})

	t.Run("HTTP", func(t *testing.T) {
		ranges := make(chan string, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ranges <- r.Header.Get("Range")
			http.ServeContent(w, r, "caches.xmss", time.Time{}, bytes.NewReader(caches))
		}))
		defer server.Close()

		cache, err := NewXMSSCache(server.URL+"/caches.xmss", pubkeys)
		require.NoError(t, err)
		requireCachedVectorKey(t, cache.keys(t.Context(), secretKeys))
		require.Equal(t, "bytes=29392-44087", <-ranges)
	})

	t.Run("HTTP without range support", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write(caches)
			require.NoError(t, err)
		}))
		defer server.Close()

		cache, err := NewXMSSCache(server.URL+"/caches.xmss", pubkeys)
		require.NoError(t, err)
		require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
	})

	t.Run("HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()

		cache, err := NewXMSSCache(server.URL+"/caches.xmss", pubkeys)
		require.NoError(t, err)
		require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
	})

	t.Run("HTTP short record", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "caches.xmss", time.Time{}, bytes.NewReader(caches[:recordStart+xmssCacheRecordSize-1]))
		}))
		defer server.Close()

		cache, err := NewXMSSCache(server.URL+"/caches.xmss", pubkeys)
		require.NoError(t, err)
		require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
	})

	t.Run("missing file", func(t *testing.T) {
		cache, err := NewXMSSCache(filepath.Join(t.TempDir(), "caches.xmss"), pubkeys)
		require.NoError(t, err)
		require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
	})

	t.Run("file without the record", func(t *testing.T) {
		for _, length := range []int{recordStart, recordStart + xmssCacheRecordSize - 1} {
			cache := testXMSSCacheFile(t, caches[:length], pubkeys)
			require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
		}
	})

	t.Run("corrupted record", func(t *testing.T) {
		for _, offset := range []int{0, 4096, 4096 + 4, xmssCacheRecordSize - 1} {
			corrupted := bytes.Clone(caches)
			corrupted[recordStart+offset] ^= 1

			cache := testXMSSCacheFile(t, corrupted, pubkeys)
			require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
		}
	})

	t.Run("record of another validator", func(t *testing.T) {
		cache := testXMSSCacheFile(t, caches, testXMSSPubkeys(map[primitives.ValidatorIndex]xmssPubkey{
			testValidatorIndex + 1: xmssPubkey(hexutil.MustDecode(vectorXMSSPubkey)),
		}))
		require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
	})

	t.Run("validator with another root", func(t *testing.T) {
		pubkey := xmssPubkey(hexutil.MustDecode(vectorXMSSPubkey))
		pubkey[0] ^= 1

		cache := testXMSSCacheFile(t, caches, testXMSSPubkeys(map[primitives.ValidatorIndex]xmssPubkey{testValidatorIndex: pubkey}))
		require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
	})

	t.Run("validator not in the state", func(t *testing.T) {
		cache := testXMSSCacheFile(t, caches, testXMSSPubkeys(map[primitives.ValidatorIndex]xmssPubkey{0: {0x01}}))
		require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
	})

	t.Run("shared public parameter", func(t *testing.T) {
		other := xmssPubkey(hexutil.MustDecode(vectorXMSSPubkey))
		other[0] ^= 1

		cache := testXMSSCacheFile(t, caches, testXMSSPubkeys(map[primitives.ValidatorIndex]xmssPubkey{
			testValidatorIndex: xmssPubkey(hexutil.MustDecode(vectorXMSSPubkey)),
			3:                  other,
		}))
		require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
	})

	t.Run("validators not listed", func(t *testing.T) {
		cache := testXMSSCacheFile(t, caches, func(context.Context) (map[primitives.ValidatorIndex]xmssPubkey, error) {
			return nil, errors.New("boom")
		})
		require.Equal(t, 0, len(cache.keys(t.Context(), secretKeys)))
	})
}

func TestRegistry_LoadFromXMSSCache(t *testing.T) {
	caches, pubkeys := testXMSSCaches(t)
	secretKey := vectorKey(t)
	blsKey := blsPubkey(secretKey.PublicKey().Marshal())

	newRegistry := func(cache *XMSSCache) (*Registry, *int) {
		registry := NewRegistry(t.Context(), nil)
		registry.SetXMSSCache(cache)

		derivations := 0
		registry.deriveKeys = func(ctx context.Context, secretKeys []bls.SecretKey) (map[blsPubkey]*Keys, error) {
			derivations += len(secretKeys)
			return DeriveAll(ctx, secretKeys)
		}

		return registry, &derivations
	}

	t.Run("loads the key from its cache", func(t *testing.T) {
		registry, derivations := newRegistry(testXMSSCacheFile(t, caches, pubkeys))
		require.NoError(t, registry.Load(t.Context(), []bls.SecretKey{secretKey}))
		require.Equal(t, 0, *derivations)

		keys, ok := registry.Keys(blsKey)
		require.Equal(t, true, ok)
		requireCachedVectorKey(t, map[blsPubkey]*Keys{blsKey: keys})

		xmssKey, ok := registry.XMSSPubkey(blsKey)
		require.Equal(t, true, ok)
		require.Equal(t, xmssPubkey(hexutil.MustDecode(vectorXMSSPubkey)), xmssKey)
	})

	t.Run("derives the key refused by its cache", func(t *testing.T) {
		corrupted := bytes.Clone(caches)
		corrupted[int(testValidatorIndex)*xmssCacheRecordSize] ^= 1

		registry, derivations := newRegistry(testXMSSCacheFile(t, corrupted, pubkeys))
		require.NoError(t, registry.Load(t.Context(), []bls.SecretKey{secretKey}))
		require.Equal(t, 1, *derivations)

		keys, ok := registry.Keys(blsKey)
		require.Equal(t, true, ok)
		requireVectorKeys(t, keys)
	})

	t.Run("derives the key without tree caches", func(t *testing.T) {
		cache, err := NewXMSSCache(filepath.Join(t.TempDir(), "caches.xmss"), pubkeys)
		require.NoError(t, err)

		registry, derivations := newRegistry(cache)
		require.NoError(t, registry.Load(t.Context(), []bls.SecretKey{secretKey}))
		require.Equal(t, 1, *derivations)

		keys, ok := registry.Keys(blsKey)
		require.Equal(t, true, ok)
		requireVectorKeys(t, keys)
	})
}
