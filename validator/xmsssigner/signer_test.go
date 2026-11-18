package xmsssigner

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
	leanxmss "github.com/nalepae/go-leanxmss"
)

// fakeKey is a private key recording the leaves it signs at, without computing real signatures.
type fakeKey struct {
	pubkey leanxmss.PublicKey
	leaves []uint32
}

func newFakeKey(t *testing.T, b byte) *fakeKey {
	encoded := [signingprotection.PubkeyLength]byte{b}
	pubkey, err := leanxmss.PublicKeyFromBytes(encoded[:])
	require.NoError(t, err)

	return &fakeKey{pubkey: pubkey}
}

func (k *fakeKey) PublicKey() leanxmss.PublicKey {
	return k.pubkey
}

func (k *fakeKey) Sign(leafIndex uint32, _ [32]byte) (*leanxmss.Signature, error) {
	k.leaves = append(k.leaves, leafIndex)
	return &leanxmss.Signature{}, nil
}

func (k *fakeKey) pubkeyArray() [signingprotection.PubkeyLength]byte {
	return [signingprotection.PubkeyLength]byte(k.pubkey.Bytes())
}

func newSigner(t *testing.T, keys ...PrivateKey) (*Signer, *signingprotection.Store) {
	store, err := signingprotection.NewStore(t.TempDir(), [32]byte{0xaa})
	require.NoError(t, err)

	signer, err := New(store, keys)
	require.NoError(t, err)

	return signer, store
}

func TestNew(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		_, err := New(nil, nil)
		require.ErrorContains(t, "nil signing protection store", err)
	})

	t.Run("keys", func(t *testing.T) {
		key1, key2 := newFakeKey(t, 1), newFakeKey(t, 2)
		signer, _ := newSigner(t, key1, key2)
		require.Equal(t, 2, len(signer.PublicKeys()))
	})

	t.Run("duplicated key", func(t *testing.T) {
		store, err := signingprotection.NewStore(t.TempDir(), [32]byte{0xaa})
		require.NoError(t, err)

		key := newFakeKey(t, 1)
		_, err = New(store, []PrivateKey{key, key})
		require.ErrorContains(t, "already known", err)
	})
}

func TestAddKey(t *testing.T) {
	t.Run("nil key", func(t *testing.T) {
		signer, _ := newSigner(t)
		require.ErrorContains(t, "nil private key", signer.AddKey(nil))
	})

	t.Run("new key", func(t *testing.T) {
		signer, _ := newSigner(t)
		key := newFakeKey(t, 1)

		require.NoError(t, signer.AddKey(key))
		require.DeepEqual(t, [][signingprotection.PubkeyLength]byte{key.pubkeyArray()}, signer.PublicKeys())
	})

	t.Run("known key", func(t *testing.T) {
		key := newFakeKey(t, 1)
		signer, _ := newSigner(t, key)
		require.ErrorContains(t, "already known", signer.AddKey(key))
	})
}

func TestRemoveKey(t *testing.T) {
	key := newFakeKey(t, 1)
	signer, _ := newSigner(t, key)

	signer.RemoveKey(key.pubkeyArray())
	require.Equal(t, 0, len(signer.PublicKeys()))

	_, err := signer.SignSlotMessage(key.pubkeyArray(), signingprotection.Block, 1, [32]byte{})
	require.ErrorIs(t, err, ErrUnknownPublicKey)
}

func TestPublicKeys(t *testing.T) {
	t.Run("no key", func(t *testing.T) {
		signer, _ := newSigner(t)
		require.Equal(t, 0, len(signer.PublicKeys()))
	})

	t.Run("some keys", func(t *testing.T) {
		key1, key2 := newFakeKey(t, 1), newFakeKey(t, 2)
		signer, _ := newSigner(t, key1, key2)

		actual := make(map[[signingprotection.PubkeyLength]byte]bool)
		for _, pubkey := range signer.PublicKeys() {
			actual[pubkey] = true
		}

		require.DeepEqual(t, map[[signingprotection.PubkeyLength]byte]bool{key1.pubkeyArray(): true, key2.pubkeyArray(): true}, actual)
	})
}

func TestSignAttestation(t *testing.T) {
	t.Run("signs at the leaf of the target epoch", func(t *testing.T) {
		key := newFakeKey(t, 1)
		signer, store := newSigner(t, key)

		_, err := signer.SignAttestation(key.pubkeyArray(), 1200, 1201, [32]byte{})
		require.NoError(t, err)
		require.DeepEqual(t, []uint32{1<<15 + 1201}, key.leaves)

		protection, err := store.SigningProtection(key.pubkeyArray())
		require.NoError(t, err)
		require.Equal(t, uint64(1201), *protection.LastSignedAttestationTargetEpoch)
	})

	t.Run("refused attestation is not signed", func(t *testing.T) {
		key := newFakeKey(t, 1)
		signer, _ := newSigner(t, key)

		_, err := signer.SignAttestation(key.pubkeyArray(), 1200, 1201, [32]byte{})
		require.NoError(t, err)

		_, err = signer.SignAttestation(key.pubkeyArray(), 1200, 1201, [32]byte{0x01})
		require.ErrorIs(t, err, signingprotection.ErrSigningRefused)
		require.Equal(t, 1, len(key.leaves))
	})

	t.Run("unknown public key records nothing", func(t *testing.T) {
		signer, store := newSigner(t)
		pubkey := [signingprotection.PubkeyLength]byte{1}

		_, err := signer.SignAttestation(pubkey, 1200, 1201, [32]byte{})
		require.ErrorIs(t, err, ErrUnknownPublicKey)

		protection, err := store.SigningProtection(pubkey)
		require.NoError(t, err)
		require.DeepEqual(t, &signingprotection.ValidatorSigningProtection{}, protection)
	})

	t.Run("target epoch beyond the attestation leaves records nothing", func(t *testing.T) {
		key := newFakeKey(t, 1)
		signer, store := newSigner(t, key)

		_, err := signer.SignAttestation(key.pubkeyArray(), 0, 1<<29, [32]byte{})
		require.ErrorContains(t, "attestation leaf index", err)
		require.Equal(t, 0, len(key.leaves))

		protection, err := store.SigningProtection(key.pubkeyArray())
		require.NoError(t, err)
		require.DeepEqual(t, &signingprotection.ValidatorSigningProtection{}, protection)
	})
}

func TestSignSlotMessage(t *testing.T) {
	const first = 1<<15 + 1<<29 + 1<<30

	t.Run("signs at the leaf of the slot and message type", func(t *testing.T) {
		key := newFakeKey(t, 1)
		signer, store := newSigner(t, key)

		_, err := signer.SignSlotMessage(key.pubkeyArray(), signingprotection.AggregateAndProof, 38450, [32]byte{})
		require.NoError(t, err)
		require.DeepEqual(t, []uint32{first + 38450*8 + 1}, key.leaves)

		protection, err := store.SigningProtection(key.pubkeyArray())
		require.NoError(t, err)
		require.Equal(t, uint64(38450), *protection.LastSignedAggregateAndProofSlot)
	})

	t.Run("refused message is not signed", func(t *testing.T) {
		key := newFakeKey(t, 1)
		signer, _ := newSigner(t, key)

		_, err := signer.SignSlotMessage(key.pubkeyArray(), signingprotection.Block, 10, [32]byte{})
		require.NoError(t, err)

		_, err = signer.SignSlotMessage(key.pubkeyArray(), signingprotection.Block, 10, [32]byte{0x01})
		require.ErrorIs(t, err, signingprotection.ErrSigningRefused)
		require.Equal(t, 1, len(key.leaves))
	})

	t.Run("unknown public key records nothing", func(t *testing.T) {
		signer, store := newSigner(t)
		pubkey := [signingprotection.PubkeyLength]byte{1}

		_, err := signer.SignSlotMessage(pubkey, signingprotection.Block, 10, [32]byte{})
		require.ErrorIs(t, err, ErrUnknownPublicKey)

		protection, err := store.SigningProtection(pubkey)
		require.NoError(t, err)
		require.DeepEqual(t, &signingprotection.ValidatorSigningProtection{}, protection)
	})

	t.Run("unknown message type records nothing", func(t *testing.T) {
		key := newFakeKey(t, 1)
		signer, _ := newSigner(t, key)

		_, err := signer.SignSlotMessage(key.pubkeyArray(), signingprotection.SlotMessage(8), 10, [32]byte{})
		require.ErrorContains(t, "slot leaf index", err)
		require.Equal(t, 0, len(key.leaves))
	})
}

func TestSignReserved(t *testing.T) {
	t.Run("signs at the next reserved leaves", func(t *testing.T) {
		key := newFakeKey(t, 1)
		signer, store := newSigner(t, key)

		for range 3 {
			_, err := signer.SignReserved(key.pubkeyArray(), [32]byte{})
			require.NoError(t, err)
		}

		require.DeepEqual(t, []uint32{1, 2, 3}, key.leaves)

		protection, err := store.SigningProtection(key.pubkeyArray())
		require.NoError(t, err)
		require.Equal(t, uint64(3), protection.LastSignedReservedLeaf)
	})

	t.Run("unknown public key allocates nothing", func(t *testing.T) {
		signer, store := newSigner(t)
		pubkey := [signingprotection.PubkeyLength]byte{1}

		_, err := signer.SignReserved(pubkey, [32]byte{})
		require.ErrorIs(t, err, ErrUnknownPublicKey)

		protection, err := store.SigningProtection(pubkey)
		require.NoError(t, err)
		require.Equal(t, uint64(0), protection.LastSignedReservedLeaf)
	})

	t.Run("all reserved leaves used", func(t *testing.T) {
		key := newFakeKey(t, 1)
		signer, store := newSigner(t, key)

		require.NoError(t, store.Merge(key.pubkeyArray(), &signingprotection.ValidatorSigningProtection{LastSignedReservedLeaf: 1<<15 - 1}))

		_, err := signer.SignReserved(key.pubkeyArray(), [32]byte{})
		require.ErrorIs(t, err, signingprotection.ErrSigningRefused)
		require.Equal(t, 0, len(key.leaves))
	})
}

func TestRealKey(t *testing.T) {
	key := leanxmss.NewKeyFromSeed([32]byte{0x01})
	signer, _ := newSigner(t, key)

	pubkey, err := pubkeyArray(key.PublicKey())
	require.NoError(t, err)

	signingRoot := [32]byte{0x02}
	signature, err := signer.SignSlotMessage(pubkey, signingprotection.Block, 38450, signingRoot)
	require.NoError(t, err)

	leafIndex, err := SlotLeafIndex(38450, signingprotection.Block)
	require.NoError(t, err)
	require.Equal(t, leafIndex, signature.LeafIndex())
	require.Equal(t, true, leanxmss.Verify(key.PublicKey(), leafIndex, signingRoot, signature))
}
