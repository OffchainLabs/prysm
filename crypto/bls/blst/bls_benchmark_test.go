//go:build ((linux && amd64) || (linux && arm64) || (darwin && amd64) || (darwin && arm64) || (windows && amd64)) && !blst_disabled

package blst_test

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/crypto/bls/blst"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func BenchmarkSignature_Verify(b *testing.B) {
	sk, err := blst.RandKey()
	require.NoError(b, err)

	msg := []byte("Some msg")
	sig := sk.Sign(msg)

	for b.Loop() {
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
	for i := range sigN {
		msg := [32]byte{'s', 'i', 'g', 'n', 'e', 'd', byte(i)}
		sk, err := blst.RandKey()
		require.NoError(b, err)
		sig := sk.Sign(msg[:])
		pks = append(pks, sk.PublicKey())
		sigs = append(sigs, sig)
		msgs = append(msgs, msg)
	}
	aggregated := blst.AggregateSignatures(sigs)

	b.ReportAllocs()
	for b.Loop() {
		if !aggregated.AggregateVerify(pks, msgs) {
			b.Fatal("could not verify aggregate sig")
		}
	}
}

func BenchmarkSecretKey_Marshal(b *testing.B) {
	key, err := blst.RandKey()
	require.NoError(b, err)
	d := key.Marshal()

	for b.Loop() {
		_, err := blst.SecretKeyFromBytes(d)
		_ = err
	}
}

func BenchmarkAggregatePublicKeys(b *testing.B) {
	for _, n := range []int{64, 128, 512, 2048, 16_384, 32_768} {
		raw := make([][]byte, n)
		for i := range raw {
			sk, err := blst.RandKey()
			require.NoError(b, err)
			raw[i] = sk.PublicKey().Marshal()
		}

		// Fill the pubkey cache, as on a live node.
		_, err := blst.AggregatePublicKeys(raw)
		require.NoError(b, err)

		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			for b.Loop() {
				_, err := blst.AggregatePublicKeys(raw)
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkAggregateMultiplePubkeys(b *testing.B) {
	for _, n := range []int{64, 128, 512, 2048, 16_384, 32_768} {
		pks := make([]common.PublicKey, n)
		for i := range pks {
			sk, err := blst.RandKey()
			require.NoError(b, err)
			pks[i] = sk.PublicKey()
		}

		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			for b.Loop() {
				blst.AggregateMultiplePubkeys(pks)
			}
		})
	}
}
