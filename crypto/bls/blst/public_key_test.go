//go:build ((linux && amd64) || (linux && arm64) || (darwin && amd64) || (darwin && arm64) || (windows && amd64)) && !blst_disabled

package blst_test

import (
	"bytes"
	"errors"
	"math/rand"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/crypto/bls/blst"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestPublicKeyFromBytes(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		err   error
	}{
		{
			name: "Nil",
			err:  errors.New("public key must be 48 bytes"),
		},
		{
			name:  "Empty",
			input: []byte{},
			err:   errors.New("public key must be 48 bytes"),
		},
		{
			name:  "Short",
			input: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			err:   errors.New("public key must be 48 bytes"),
		},
		{
			name:  "Long",
			input: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			err:   errors.New("public key must be 48 bytes"),
		},
		{
			name:  "Bad",
			input: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			err:   errors.New("could not unmarshal bytes into public key"),
		},
		{
			name:  "Good",
			input: []byte{0xa9, 0x9a, 0x76, 0xed, 0x77, 0x96, 0xf7, 0xbe, 0x22, 0xd5, 0xb7, 0xe8, 0x5d, 0xee, 0xb7, 0xc5, 0x67, 0x7e, 0x88, 0xe5, 0x11, 0xe0, 0xb3, 0x37, 0x61, 0x8f, 0x8c, 0x4e, 0xb6, 0x13, 0x49, 0xb4, 0xbf, 0x2d, 0x15, 0x3f, 0x64, 0x9f, 0x7b, 0x53, 0x35, 0x9f, 0xe8, 0xb9, 0x4a, 0x38, 0xe4, 0x4c},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := blst.PublicKeyFromBytes(test.input)
			if test.err != nil {
				assert.NotEqual(t, nil, err, "No error returned")
				assert.ErrorContains(t, test.err.Error(), err, "Unexpected error returned")
			} else {
				assert.NoError(t, err)
				assert.DeepEqual(t, 0, bytes.Compare(res.Marshal(), test.input))
			}
		})
	}
}

func TestPublicKey_Copy(t *testing.T) {
	priv, err := blst.RandKey()
	require.NoError(t, err)
	pubkeyA := priv.PublicKey()
	pubkeyBytes := pubkeyA.Marshal()

	pubkeyB := pubkeyA.Copy()
	priv2, err := blst.RandKey()
	require.NoError(t, err)
	pubkeyB.Aggregate(priv2.PublicKey())

	require.DeepEqual(t, pubkeyA.Marshal(), pubkeyBytes, "Pubkey was mutated after copy")
}

func TestPublicKey_Aggregate(t *testing.T) {
	priv, err := blst.RandKey()
	require.NoError(t, err)
	pubkeyA := priv.PublicKey()

	pubkeyB := pubkeyA.Copy()
	priv2, err := blst.RandKey()
	require.NoError(t, err)
	resKey := pubkeyB.Aggregate(priv2.PublicKey())

	aggKey := blst.AggregateMultiplePubkeys([]common.PublicKey{priv.PublicKey(), priv2.PublicKey()})

	require.DeepEqual(t, resKey.Marshal(), aggKey.Marshal(), "Pubkey does not match up")
}

func TestSubtractPublicKey(t *testing.T) {
	keys := make([]common.PublicKey, 64)
	for i := range keys {
		priv, err := blst.RandKey()
		require.NoError(t, err)
		keys[i] = priv.PublicKey()
	}
	all := blst.AggregateMultiplePubkeys(keys)

	// check splits keys by the absent flags.
	// asserts agg(all) - agg(absent) == agg(present).
	check := func(t *testing.T, absent []bool) {
		var present, missing []common.PublicKey
		for i, k := range keys {
			if absent[i] {
				missing = append(missing, k)
			} else {
				present = append(present, k)
			}
		}
		got := blst.SubtractPublicKey(all, blst.AggregateMultiplePubkeys(missing))
		require.Equal(t, true, got.Equals(blst.AggregateMultiplePubkeys(present)))
		require.Equal(t, len(present) == 0, got.IsInfinite())
	}

	t.Run("random", func(t *testing.T) {
		r := rand.New(rand.NewSource(1))
		for range 20 {
			absent := make([]bool, len(keys))
			for i := range absent {
				absent[i] = r.Intn(2) == 0
			}
			check(t, absent)
		}
	})
	t.Run("absent empty", func(t *testing.T) {
		check(t, make([]bool, len(keys)))
	})
	t.Run("absent all but one", func(t *testing.T) {
		absent := make([]bool, len(keys))
		for i := 1; i < len(absent); i++ {
			absent[i] = true
		}
		check(t, absent)
	})
	t.Run("absent all", func(t *testing.T) {
		absent := make([]bool, len(keys))
		for i := range absent {
			absent[i] = true
		}
		check(t, absent)
	})
	t.Run("does not mutate inputs", func(t *testing.T) {
		before := all.Marshal()
		sub := keys[0].Marshal()
		blst.SubtractPublicKey(all, keys[0])
		require.DeepEqual(t, before, all.Marshal())
		require.DeepEqual(t, sub, keys[0].Marshal())
	})
}

func TestPublicKey_Aggregation_NoCorruption(t *testing.T) {
	var pubkeys []common.PublicKey
	for range 100 {
		priv, err := blst.RandKey()
		require.NoError(t, err)
		pubkey := priv.PublicKey()
		pubkeys = append(pubkeys, pubkey)
	}

	var compressedKeys [][]byte
	// Fill up the cache
	for _, pkey := range pubkeys {
		_, err := blst.PublicKeyFromBytes(pkey.Marshal())
		require.NoError(t, err)
		compressedKeys = append(compressedKeys, pkey.Marshal())
	}

	wg := new(sync.WaitGroup)

	// Aggregate different sets of keys.
	wg.Go(func() {
		_, err := blst.AggregatePublicKeys(compressedKeys)
		require.NoError(t, err)
	})

	wg.Go(func() {
		_, err := blst.AggregatePublicKeys(compressedKeys[:10])
		require.NoError(t, err)
	})

	wg.Go(func() {
		_, err := blst.AggregatePublicKeys(compressedKeys[:40])
		require.NoError(t, err)
	})

	wg.Go(func() {
		_, err := blst.AggregatePublicKeys(compressedKeys[20:60])
		require.NoError(t, err)
	})

	wg.Go(func() {
		_, err := blst.AggregatePublicKeys(compressedKeys[80:])
		require.NoError(t, err)
	})

	wg.Go(func() {
		_, err := blst.AggregatePublicKeys(compressedKeys[60:90])
		require.NoError(t, err)
	})

	wg.Go(func() {
		_, err := blst.AggregatePublicKeys(compressedKeys[40:99])
		require.NoError(t, err)
	})

	wg.Wait()

	for _, pkey := range pubkeys {
		cachedPubkey, err := blst.PublicKeyFromBytes(pkey.Marshal())
		require.NoError(t, err)
		assert.Equal(t, true, cachedPubkey.Equals(pkey))
	}
}

func TestPublicKeysEmpty(t *testing.T) {
	var pubs [][]byte
	_, err := blst.AggregatePublicKeys(pubs)
	require.ErrorContains(t, "nil or empty public keys", err)
}

func TestAggregatePubkeys_BatchedAdd(t *testing.T) {
	// pairwise folds the keys with PublicKey.Aggregate, which adds two points at a time with P1Aggregate.
	pairwise := func(pks []common.PublicKey) common.PublicKey {
		acc := pks[0].Copy()
		for _, pk := range pks[1:] {
			acc = acc.Aggregate(pk)
		}
		return acc
	}
	randKeys := func(t *testing.T, n int) []common.PublicKey {
		pks := make([]common.PublicKey, n)
		for i := range pks {
			sk, err := blst.RandKey()
			require.NoError(t, err)
			pks[i] = sk.PublicKey()
		}
		return pks
	}
	marshal := func(pks []common.PublicKey) [][]byte {
		raw := make([][]byte, len(pks))
		for i, pk := range pks {
			raw[i] = pk.Marshal()
		}
		return raw
	}
	// requireBoth checks AggregateMultiplePubkeys and AggregatePublicKeys against want.
	requireBoth := func(t *testing.T, pks []common.PublicKey, want common.PublicKey) {
		require.Equal(t, true, want.Equals(blst.AggregateMultiplePubkeys(pks)))
		got, err := blst.AggregatePublicKeys(marshal(pks))
		require.NoError(t, err)
		require.Equal(t, true, want.Equals(got))
	}
	inf := blst.AggregateMultiplePubkeys(nil)

	t.Run("empty", func(t *testing.T) {
		require.Equal(t, true, inf.IsInfinite())
		require.Equal(t, true, blst.AggregateMultiplePubkeys([]common.PublicKey{}).IsInfinite())
	})

	t.Run("single key", func(t *testing.T) {
		pks := randKeys(t, 1)
		requireBoth(t, pks, pks[0])
	})

	t.Run("random sets", func(t *testing.T) {
		// 1. 16 and more keys take blst's batched-inversion path.
		// 2. blst adds at most 1024 points per scratch chunk, so 1025 and 2049 keys cross chunk boundaries.
		for _, n := range []int{2, 3, 15, 16, 17, 64, 513, 1025, 2049} {
			pks := randKeys(t, n)
			requireBoth(t, pks, pairwise(pks))
		}
	})

	t.Run("repeated keys", func(t *testing.T) {
		// A sync committee can hold the same validator more than once, so the sum must double correctly.
		// 32 copies of one key put P+P pairs into the batched path.
		pks := randKeys(t, 3)
		pks = append(pks, pks[0], pks[0], pks[1])
		for range 32 {
			pks = append(pks, pks[2])
		}
		requireBoth(t, pks, pairwise(pks))
	})

	t.Run("point at infinity", func(t *testing.T) {
		pks := randKeys(t, 2)
		require.Equal(t, true, pairwise(pks).Equals(blst.AggregateMultiplePubkeys([]common.PublicKey{pks[0], inf, pks[1]})))
		big := randKeys(t, 40)
		withInf := append([]common.PublicKey{inf}, big[:20]...)
		withInf = append(withInf, inf, inf)
		withInf = append(withInf, big[20:]...)
		require.Equal(t, true, pairwise(big).Equals(blst.AggregateMultiplePubkeys(withInf)))
	})

	t.Run("cancelling keys", func(t *testing.T) {
		// P + -P is the point at infinity. AggregatePublicKeys accepts -P because it is a valid key.
		pks := randKeys(t, 21)
		var cancel []common.PublicKey
		for _, pk := range pks[:20] {
			cancel = append(cancel, pk, blst.SubtractPublicKey(inf, pk))
		}
		requireBoth(t, cancel[:2], inf)
		requireBoth(t, cancel, inf)
		requireBoth(t, append(cancel, pks[20]), pks[20])
	})
}
