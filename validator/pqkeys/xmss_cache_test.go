package pqkeys

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
	leanxmss "github.com/nalepae/go-leanxmss"
)

// The BLS secret key of the devnet-0 mnemonic's key 0, as in Lighthouse's xmss_cache tests, and its record in the
// published XMSS tree cache: bytes 0 to 14,695 of the cache Lighthouse's pq-genesis README points to (SHA-256
// 2250ce36f2047783a1227332f87e396b89df6ed0a19b4421caee77303fcf357c).
const (
	devnetZeroSecretKey = "0x0dce41fa73ae9f6bdfd51df4d422d75eee174553dba5fd450c4437e4ed3fc903"
	devnetZeroPubkey    = "0xd7792e263b94cf0b7a9f70d8374c6d34a600b4247057d4079f810caff711c632"
	devnetZeroRecord    = "testdata/devnet0_index0.xmss"
)

func devnetZero(t *testing.T) (bls.SecretKey, []byte) {
	secretKey, err := bls.SecretKeyFromBytes(hexutil.MustDecode(devnetZeroSecretKey))
	require.NoError(t, err)

	record, err := os.ReadFile(devnetZeroRecord)
	require.NoError(t, err)
	require.Equal(t, xmssRecordSize, len(record))

	return secretKey, record
}

// writeXMSSCache writes a published tree cache whose record at index 0 is `record`, then `extra` records of zeros.
func writeXMSSCache(t *testing.T, record []byte, extra int) string {
	path := filepath.Join(t.TempDir(), "cache.xmss")
	content := append(append([]byte(nil), record...), make([]byte, extra*xmssRecordSize)...)
	require.NoError(t, os.WriteFile(path, content, 0o600))

	return path
}

// devnetZeroPubkeys lists the XMSS public keys of a validator registry whose validator 0 is the devnet-0 key 0.
func devnetZeroPubkeys(_ context.Context, count int) ([][fieldparams.XMSSPubkeyLength]byte, error) {
	pubkeys := [][fieldparams.XMSSPubkeyLength]byte{[fieldparams.XMSSPubkeyLength]byte(hexutil.MustDecode(devnetZeroPubkey))}
	return pubkeys[:min(count, len(pubkeys))], nil
}

func TestIndexPublishedKeys(t *testing.T) {
	secretKey, _ := devnetZero(t)
	param := [xmssPublicParamSize]byte(hexutil.MustDecode(devnetZeroPubkey)[xmssPubkeySize-xmssPublicParamSize:])
	require.Equal(t, param, leanxmss.SeedPublicParam(xmssSeed(secretKey.Marshal())))

	zero := [fieldparams.XMSSPubkeyLength]byte(hexutil.MustDecode(devnetZeroPubkey))
	other := zero
	other[0] ^= 1

	t.Run("indexes by public parameter", func(t *testing.T) {
		keys := indexPublishedKeys([][fieldparams.XMSSPubkeyLength]byte{{}, zero})
		require.Equal(t, 2, len(keys))
		require.Equal(t, uint32(1), keys[param].index)
		require.DeepEqual(t, zero[:], keys[param].pubkey.Bytes())
	})

	t.Run("leaves out shared public parameters", func(t *testing.T) {
		keys := indexPublishedKeys([][fieldparams.XMSSPubkeyLength]byte{zero, other, zero})
		require.Equal(t, 0, len(keys))
	})

	t.Run("leaves out the validators without a record", func(t *testing.T) {
		pubkeys := make([][fieldparams.XMSSPubkeyLength]byte, maxXMSSCacheRecords+1)
		pubkeys[maxXMSSCacheRecords] = zero
		require.Equal(t, 0, len(indexPublishedKeys(pubkeys)))
	})
}

func TestXMSSCacheSource(t *testing.T) {
	_, record := devnetZero(t)

	t.Run("locations", func(t *testing.T) {
		for location, expectedErr := range map[string]string{
			"":                        "empty",
			"ftp://example.com/cache": "neither http, https nor file",
			"file://":                 "without a path",
			"relative/cache.xmss":     "",
			"https://example.com/c":   "",
		} {
			_, err := NewXMSSCacheSource(location)
			if expectedErr == "" {
				require.NoError(t, err, location)
				continue
			}

			require.ErrorContains(t, expectedErr, err, location)
		}
	})

	t.Run("file", func(t *testing.T) {
		path := writeXMSSCache(t, record, 1)
		for _, location := range []string{path, "file://" + path} {
			source, err := NewXMSSCacheSource(location)
			require.NoError(t, err)

			got, err := source.Record(t.Context(), 0)
			require.NoError(t, err)
			require.DeepEqual(t, record, got)

			got, err = source.Record(t.Context(), 1)
			require.NoError(t, err)
			require.DeepEqual(t, make([]byte, xmssRecordSize), got)

			_, err = source.Record(t.Context(), 2)
			require.ErrorContains(t, "missing record", err)
		}

		truncated := filepath.Join(t.TempDir(), "truncated.xmss")
		require.NoError(t, os.WriteFile(truncated, record[:xmssRecordSize-1], 0o600))
		source, err := NewXMSSCacheSource(truncated)
		require.NoError(t, err)
		_, err = source.Record(t.Context(), 0)
		require.ErrorContains(t, "want a multiple", err)
	})

	t.Run("HTTP range requests", func(t *testing.T) {
		content := append(append([]byte(nil), make([]byte, xmssRecordSize)...), record...)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "identity", r.Header.Get("Accept-Encoding"))
			http.ServeContent(w, r, "cache.xmss", time.Time{}, bytes.NewReader(content))
		}))
		defer server.Close()

		source, err := NewXMSSCacheSource(server.URL)
		require.NoError(t, err)

		got, err := source.Record(t.Context(), 1)
		require.NoError(t, err)
		require.DeepEqual(t, record, got)

		_, err = source.Record(t.Context(), 2)
		require.ErrorContains(t, "status 416", err)

		_, err = source.Record(t.Context(), maxXMSSCacheRecords)
		require.ErrorContains(t, "at most", err)
	})

	t.Run("HTTP server ignoring ranges", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write(record)
			require.NoError(t, err)
		}))
		defer server.Close()

		source, err := NewXMSSCacheSource(server.URL)
		require.NoError(t, err)
		_, err = source.Record(t.Context(), 0)
		require.ErrorContains(t, "status 200", err)
	})

	t.Run("HTTP wrong Content-Range", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", xmssRecordSize-1, 2*xmssRecordSize))
			w.WriteHeader(http.StatusPartialContent)
			_, err := w.Write(record)
			require.NoError(t, err)
		}))
		defer server.Close()

		source, err := NewXMSSCacheSource(server.URL)
		require.NoError(t, err)
		_, err = source.Record(t.Context(), 1)
		require.ErrorContains(t, "Content-Range", err)
	})
}

func TestXMSSLoader(t *testing.T) {
	secretKey, record := devnetZero(t)
	blsKey := blsPubkey(secretKey.PublicKey().Marshal())
	dir := t.TempDir()
	cachePath := filepath.Join(dir, fmt.Sprintf("%#x.cache", blsKey))

	derive := func(t *testing.T, loader *XMSSLoader) *Keys {
		keys, err := loader.DeriveAll(t.Context(), []bls.SecretKey{secretKey, secretKey})
		require.NoError(t, err)
		require.Equal(t, 1, len(keys))
		require.DeepEqual(t, hexutil.MustDecode(devnetZeroPubkey), keys[blsKey].XMSS.PublicKey().Bytes())

		return keys[blsKey]
	}

	source, err := NewXMSSCacheSource(writeXMSSCache(t, record, 0))
	require.NoError(t, err)

	// From the published tree cache, which holds bottom trees 0 and 24,576: no tree to build.
	keys := derive(t, NewXMSSLoader(dir, source, devnetZeroPubkeys))
	require.DeepEqual(t, record, keys.XMSS.TreeCache())
	require.NoError(t, keys.PersistXMSS())

	content, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	require.Equal(t, len(xmssCacheMagic)+xmssPubkeySize+xmssRecordSize+xmssCacheTagSize, len(content))

	// Unchanged trees are not written again.
	require.NoError(t, os.Remove(cachePath))
	require.NoError(t, keys.PersistXMSS())
	_, err = os.Stat(cachePath)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, os.WriteFile(cachePath, content, 0o600))

	// From disk, without the published tree cache.
	keys = derive(t, NewXMSSLoader(dir, nil, nil))
	require.DeepEqual(t, record, keys.XMSS.TreeCache())

	// Another seed cannot authenticate the tree cache on disk.
	other := NewXMSSLoader(dir, nil, nil).newDiskCache(blsKey, [32]byte{1})
	_, err = other.read([32]byte{1})
	require.ErrorContains(t, "authentication failed", err)

	// A tampered tree cache on disk falls back to the published one.
	tampered := append([]byte(nil), content...)
	tampered[len(xmssCacheMagic)+xmssPubkeySize+10] ^= 1
	require.NoError(t, os.WriteFile(cachePath, tampered, 0o600))
	keys = derive(t, NewXMSSLoader(dir, source, devnetZeroPubkeys))
	require.DeepEqual(t, record, keys.XMSS.TreeCache())

	// A published record that does not match the key falls back to building it, without bottom trees.
	require.NoError(t, os.Remove(cachePath))
	wrong := append([]byte(nil), record...)
	wrong[0] ^= 1
	wrongSource, err := NewXMSSCacheSource(writeXMSSCache(t, wrong, 0))
	require.NoError(t, err)
	keys = derive(t, NewXMSSLoader(dir, wrongSource, devnetZeroPubkeys))
	require.DeepEqual(t, record[:treeCacheTopSize], keys.XMSS.TreeCache())

	// A registry that cannot be listed falls back to building the key, and is listed again on the next load.
	calls := 0
	failing := func(context.Context, int) ([][fieldparams.XMSSPubkeyLength]byte, error) {
		calls++
		return nil, errors.New("beacon node unreachable")
	}

	loader := NewXMSSLoader(dir, source, failing)
	keys = derive(t, loader)
	require.DeepEqual(t, record[:treeCacheTopSize], keys.XMSS.TreeCache())

	derive(t, loader)
	require.Equal(t, 2, calls)

	// A registry is listed once, and not at all when every key is on disk.
	calls = 0
	counting := func(ctx context.Context, count int) ([][fieldparams.XMSSPubkeyLength]byte, error) {
		calls++
		return devnetZeroPubkeys(ctx, count)
	}

	loader = NewXMSSLoader(dir, source, counting)
	keys = derive(t, loader)
	require.DeepEqual(t, record, keys.XMSS.TreeCache())
	require.NoError(t, keys.PersistXMSS())

	derive(t, loader)
	derive(t, NewXMSSLoader(dir, source, counting))
	require.Equal(t, 1, calls)
}
