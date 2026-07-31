//go:build fake_crypto

package bls

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/crypto/bls/blst"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// The property the fake backend exists for: the stub signature parses and
// verifies, where the real backend rejects it while deserializing.
func TestFakeAcceptsTheStubSignature(t *testing.T) {
	_, err := blst.SignatureFromBytes(stubSignature)
	require.NotNil(t, err, "real backend unexpectedly accepted the stub signature")

	pub := realPubkey(t)
	sig, err := SignatureFromBytes(stubSignature)
	require.NoError(t, err)

	require.Equal(t, true, sig.Verify(pub, make([]byte, 32)))
	require.Equal(t, true, sig.FastAggregateVerify([]PublicKey{pub}, [32]byte{}))

	valid, err := VerifyMultipleSignatures([][]byte{stubSignature}, [][32]byte{{}}, []PublicKey{pub})
	require.NoError(t, err)
	require.Equal(t, true, valid)
}

func TestFakeRejectsMalformedInput(t *testing.T) {
	_, err := SignatureFromBytes(stubSignature[:95])
	require.NotNil(t, err)

	_, err = VerifyMultipleSignatures([][]byte{stubSignature}, [][32]byte{{}, {}}, []PublicKey{realPubkey(t)})
	require.NotNil(t, err, "batch with mismatched lengths must not verify")
}

// Key material is real, because the spec computes AggregatePKs unconditionally.
// Ref: https://github.com/ethereum/consensus-specs/pull/5489
func TestFakeKeysAreReal(t *testing.T) {
	raw := make([][]byte, 3)
	keys := make([]PublicKey, 3)
	for i := range raw {
		keys[i] = realPubkey(t)
		raw[i] = keys[i].Marshal()
	}
	want, err := blst.AggregatePublicKeys(raw)
	require.NoError(t, err)

	got, err := AggregatePublicKeys(raw)
	require.NoError(t, err)
	require.DeepEqual(t, want.Marshal(), got.Marshal())
	require.DeepEqual(t, want.Marshal(), AggregateMultiplePubkeys(keys).Marshal())

	_, err = PublicKeyFromBytes(bytes.Repeat([]byte{0x22}, 48))
	require.NotNil(t, err, "the spec's unused STUB_PUBKEY is not a valid key")
	_, err = PublicKeyFromBytes(common.InfinitePublicKey[:])
	require.Equal(t, common.ErrInfinitePubKey, err)
}

func TestFakeSecretKeysSignWithTheStub(t *testing.T) {
	a, err := RandKey()
	require.NoError(t, err)
	require.DeepEqual(t, stubSignature, a.Sign([]byte("msg")).Marshal())

	sk, err := SecretKeyFromBytes(a.Marshal())
	require.NoError(t, err)
	require.Equal(t, true, sk.PublicKey().Equals(a.PublicKey()))

	_, err = SecretKeyFromBytes(common.ZeroSecretKey[:])
	require.Equal(t, common.ErrSecretUnmarshal, err)
}

func TestFakeBatchRoundTrip(t *testing.T) {
	pub := realPubkey(t)
	set := &SignatureBatch{
		Signatures:   [][]byte{stubSignature, stubSignature},
		PublicKeys:   []PublicKey{pub, pub.Copy()},
		Messages:     [][32]byte{{1}, {1}},
		Descriptions: []string{"a", "b"},
	}

	dups, deduped, err := set.Copy().RemoveDuplicates()
	require.NoError(t, err)
	require.Equal(t, 1, dups)
	require.Equal(t, 1, len(deduped.Signatures))

	agg, err := set.AggregateBatch()
	require.NoError(t, err)
	require.Equal(t, 1, len(agg.Signatures))
	require.Equal(t, true, bytes.Equal(stubSignature, agg.Signatures[0]))

	valid, err := agg.Verify()
	require.NoError(t, err)
	require.Equal(t, true, valid)
}

func realPubkey(t *testing.T) PublicKey {
	sk, err := blst.RandKey()
	require.NoError(t, err)
	return sk.PublicKey()
}
