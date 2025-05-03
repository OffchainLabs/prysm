//go:build ((linux && amd64) || (linux && arm64) || (darwin && amd64) || (darwin && arm64) || (windows && amd64)) && !blst_disabled

package blst_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/crypto/bls/blst"
	"github.com/OffchainLabs/prysm/v6/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func BenchmarkSignature_Verify(b *testing.B) {
	sk, err := blst.RandKey()
	require.NoError(b, err)

	msg := []byte("Some msg")
	sig := sk.Sign(msg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !sig.Verify(sk.PublicKey(), msg) {
			b.Fatal("could not verify sig")
		}
	}
}

func BenchmarkSignature_AggregateVerify(b *testing.B) {
	sigN := 128 // MAX_ATTESTATIONS per block.

	var pks []common.PublicKey
	var sigs []common.Signature
	var msgs [][32]byte
	for i := 0; i < sigN; i++ {
		msg := [32]byte{'s', 'i', 'g', 'n', 'e', 'd', byte(i)}
		sk, err := blst.RandKey()
		require.NoError(b, err)
		sig := sk.Sign(msg[:])
		pks = append(pks, sk.PublicKey())
		sigs = append(sigs, sig)
		msgs = append(msgs, msg)
	}
	aggregated := blst.AggregateSignatures(sigs)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !aggregated.AggregateVerify(pks, msgs) {
			b.Fatal("could not verify aggregate sig")
		}
	}
}

func BenchmarkSignature_FastAggregateVerify(b *testing.B) {
	sigN := 128 // MAX_ATTESTATIONS per block.

	msg := [32]byte{'s', 'i', 'g', 'n', 'e', 'd', 'm', 's', 'g'}

	var pks []common.PublicKey
	var sigs []common.Signature

	// Gen random keys and signatures for the same message
	for i := 0; i < sigN; i++ {
		sk, err := blst.RandKey()
		require.NoError(b, err)
		sig := sk.Sign(msg[:])
		pks = append(pks, sk.PublicKey())
		sigs = append(sigs, sig)
	}

	aggregated := blst.AggregateSignatures(sigs)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !aggregated.FastAggregateVerify(pks, msg) {
			b.Fatal("could not verify fast aggregate sig")
		}
	}
}

func BenchmarkSignature_Eth2FastAggregateVerify(b *testing.B) {
	sigN := 128

	msg := [32]byte{'s', 'i', 'g', 'n', 'e', 'd', 'm', 's', 'g'}

	var pks []common.PublicKey
	var sigs []common.Signature

	for i := 0; i < sigN; i++ {
		sk, err := blst.RandKey()
		require.NoError(b, err)
		sig := sk.Sign(msg[:])
		pks = append(pks, sk.PublicKey())
		sigs = append(sigs, sig)
	}

	aggregated := blst.AggregateSignatures(sigs)

	b.Run("Regular case with signatures", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !aggregated.Eth2FastAggregateVerify(pks, msg) {
				b.Fatal("could not verify eth2 fast aggregate sig")
			}
		}
	})

	// Special case: Empty pubkeys with infinite signature (Should pass considering as empty aggregates)
	infiniteSig, err := blst.SignatureFromBytes(common.InfiniteSignature[:])
	require.NoError(b, err)

	b.Run("Special case: empty pubkeys with infinite signature", func(b *testing.B) {
		emptyPks := make([]common.PublicKey, 0)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !infiniteSig.Eth2FastAggregateVerify(emptyPks, msg) {
				b.Fatal("infinite signature check failed with empty pubkeys")
			}
		}
	})

	// Non-infinite signature with empty pubkeys (should fail)
	b.Run("Invalid case: empty pubkeys with regular signature", func(b *testing.B) {
		emptyPks := make([]common.PublicKey, 0)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if aggregated.Eth2FastAggregateVerify(emptyPks, msg) {
				b.Fatal("verification unexpectedly passed with empty pubkeys and non-infinite sig")
			}
		}
	})
}

func BenchmarkSecretKey_Marshal(b *testing.B) {
	key, err := blst.RandKey()
	require.NoError(b, err)
	d := key.Marshal()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := blst.SecretKeyFromBytes(d)
		_ = err
	}
}
