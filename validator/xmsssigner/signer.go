// Package xmsssigner signs the messages of a leanXMSS validator client.
//
// It is the only way to sign with an XMSS key: for each message, it derives the leaf from the message,
// checks and records it in the signing protection database, and only then signs at that leaf.
// Signing at an arbitrary leaf is not possible.
package xmsssigner

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/validator/db/signingprotection"
	leanxmss "github.com/nalepae/go-leanxmss"
)

// ErrUnknownPublicKey is returned when signing with a public key the signer has no private key for.
var ErrUnknownPublicKey = errors.New("unknown public key")

type (
	// PrivateKey signs at a given leaf.
	PrivateKey interface {
		PublicKey() leanxmss.PublicKey
		Sign(leafIndex uint32, message [32]byte) (*leanxmss.Signature, error)
	}

	// Signer signs messages with XMSS keys, under the protection of a signing protection database.
	Signer struct {
		protection *signingprotection.Store

		keys sync.Map // [signingprotection.PubkeyLength]byte -> PrivateKey
	}
)

// New returns a signer of `keys`, protected by `protection`.
func New(protection *signingprotection.Store, keys []PrivateKey) (*Signer, error) {
	if protection == nil {
		return nil, errors.New("nil signing protection store")
	}

	signer := &Signer{protection: protection}

	for _, key := range keys {
		if err := signer.AddKey(key); err != nil {
			return nil, fmt.Errorf("add key: %w", err)
		}
	}

	return signer, nil
}

// AddKey makes the signer able to sign with `key`.
func (s *Signer) AddKey(key PrivateKey) error {
	if key == nil {
		return errors.New("nil private key")
	}

	pubkey, err := pubkeyArray(key.PublicKey())
	if err != nil {
		return fmt.Errorf("public key: %w", err)
	}

	if _, loaded := s.keys.LoadOrStore(pubkey, key); loaded {
		return fmt.Errorf("public key %#x is already known", pubkey)
	}

	return nil
}

// RemoveKey makes the signer unable to sign with the private key of `pubkey`.
func (s *Signer) RemoveKey(pubkey [signingprotection.PubkeyLength]byte) {
	s.keys.Delete(pubkey)
}

// PublicKeys returns the public keys the signer can sign with.
func (s *Signer) PublicKeys() [][signingprotection.PubkeyLength]byte {
	pubkeys := make([][signingprotection.PubkeyLength]byte, 0)
	s.keys.Range(func(pubkey, _ any) bool {
		pubkeys = append(pubkeys, pubkey.([signingprotection.PubkeyLength]byte))
		return true
	})

	return pubkeys
}

// SignAttestation signs `signingRoot`, the signing root of an attestation with `source` and `target` epochs,
// at the leaf of `target`.
func (s *Signer) SignAttestation(
	pubkey [signingprotection.PubkeyLength]byte,
	source, target primitives.Epoch,
	signingRoot [32]byte,
) (*leanxmss.Signature, error) {
	leafIndex, err := AttestationLeafIndex(target)
	if err != nil {
		return nil, fmt.Errorf("attestation leaf index: %w", err)
	}

	key, err := s.key(pubkey)
	if err != nil {
		return nil, fmt.Errorf("key: %w", err)
	}

	if err := s.protection.CheckAndSaveAttestation(pubkey, source, target); err != nil {
		return nil, fmt.Errorf("check and save attestation: %w", err)
	}

	signature, err := key.Sign(leafIndex, signingRoot)
	if err != nil {
		return nil, fmt.Errorf("sign at leaf %d: %w", leafIndex, err)
	}

	return signature, nil
}

// SignSlotMessage signs `signingRoot`, the signing root of a message of type `message` for `slot`,
// at the leaf of `slot` and `message`.
func (s *Signer) SignSlotMessage(
	pubkey [signingprotection.PubkeyLength]byte,
	message signingprotection.SlotMessage,
	slot primitives.Slot,
	signingRoot [32]byte,
) (*leanxmss.Signature, error) {
	leafIndex, err := SlotLeafIndex(slot, message)
	if err != nil {
		return nil, fmt.Errorf("slot leaf index: %w", err)
	}

	key, err := s.key(pubkey)
	if err != nil {
		return nil, fmt.Errorf("key: %w", err)
	}

	if err := s.protection.CheckAndSaveSlotMessage(pubkey, message, slot); err != nil {
		return nil, fmt.Errorf("check and save slot message: %w", err)
	}

	signature, err := key.Sign(leafIndex, signingRoot)
	if err != nil {
		return nil, fmt.Errorf("sign at leaf %d: %w", leafIndex, err)
	}

	return signature, nil
}

// SignReserved signs `signingRoot`, the signing root of a message without a leaf of its own,
// at the next unused reserved leaf.
func (s *Signer) SignReserved(pubkey [signingprotection.PubkeyLength]byte, signingRoot [32]byte) (*leanxmss.Signature, error) {
	key, err := s.key(pubkey)
	if err != nil {
		return nil, fmt.Errorf("key: %w", err)
	}

	leaf, err := s.protection.NextReservedLeaf(pubkey)
	if err != nil {
		return nil, fmt.Errorf("next reserved leaf: %w", err)
	}

	if leaf > math.MaxUint32 {
		return nil, fmt.Errorf("reserved leaf %d does not fit in a leaf index", leaf)
	}

	leafIndex := uint32(leaf) // #nosec G115 -- Checked above.

	signature, err := key.Sign(leafIndex, signingRoot)
	if err != nil {
		return nil, fmt.Errorf("sign at leaf %d: %w", leafIndex, err)
	}

	return signature, nil
}

// key returns the private key of `pubkey`.
func (s *Signer) key(pubkey [signingprotection.PubkeyLength]byte) (PrivateKey, error) {
	key, ok := s.keys.Load(pubkey)
	if !ok {
		return nil, fmt.Errorf("%w: %#x", ErrUnknownPublicKey, pubkey)
	}

	return key.(PrivateKey), nil
}

// pubkeyArray returns the bytes of `pubkey` as an array.
func pubkeyArray(pubkey leanxmss.PublicKey) ([signingprotection.PubkeyLength]byte, error) {
	encoded := pubkey.Bytes()
	if len(encoded) != signingprotection.PubkeyLength {
		return [signingprotection.PubkeyLength]byte{}, fmt.Errorf("public key has %d bytes, expected %d", len(encoded), signingprotection.PubkeyLength)
	}

	return [signingprotection.PubkeyLength]byte(encoded), nil
}
