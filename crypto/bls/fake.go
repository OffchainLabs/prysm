//go:build fake_crypto

// Package bls, under the `fake_crypto` tag, accepts the consensus spec's stub
// signature encoding and treats every signature as valid.
//
// The split mirrors eth2spec/utils/bls.py with `bls_active = False`, where only
// the signature functions are stubbed: Verify/AggregateVerify/FastAggregateVerify
// return True and Sign/Aggregate return STUB_SIGNATURE, while AggregatePKs is
// computed for real because their results are data written into the beacon state.
//
// Never production: the cmd/beacon-chain, cmd/validator and cmd/prysmctl builds fail with the tag.
package bls

import (
	"bytes"
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/blst"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/pkg/errors"
)

var (
	// stubSignature is what the spec returns from Sign and Aggregate with BLS
	// disabled.
	// Ref: https://github.com/ethereum/consensus-specs/blob/123d1efa05751e762fa2f877a978513f6bf5040c/tests/core/pyspec/eth_consensus_specs/utils/bls.py#L11
	//
	// Note: It is not a curve point, so the real backend cannot even deserialize it.
	stubSignature = bytes.Repeat([]byte{0x11}, fieldparams.BLSSignatureLength)

	_ common.SecretKey = (*fakeSecretKey)(nil)
	_ common.Signature = (*fakeSignature)(nil)
)

type (
	// fakeSecretKey is a real key that signs with the stub.
	fakeSecretKey struct{ common.SecretKey }

	// fakeSignature holds raw bytes rather than a curve point.
	fakeSignature struct{ b []byte }
)

// SecretKeyFromBytes --
func SecretKeyFromBytes(privKey []byte) (SecretKey, error) {
	sk, err := blst.SecretKeyFromBytes(privKey)
	if err != nil {
		return nil, err
	}
	return &fakeSecretKey{sk}, nil
}

// PublicKeyFromBytes --
func PublicKeyFromBytes(pubKey []byte) (PublicKey, error) {
	return blst.PublicKeyFromBytes(pubKey)
}

// SignatureFromBytesNoValidation --
func SignatureFromBytesNoValidation(sig []byte) (Signature, error) {
	return SignatureFromBytes(sig)
}

// SignatureFromBytes --
func SignatureFromBytes(sig []byte) (Signature, error) {
	if len(sig) != fieldparams.BLSSignatureLength {
		return nil, fmt.Errorf("signature must be %d bytes", fieldparams.BLSSignatureLength)
	}
	return &fakeSignature{b: copyBytes(sig)}, nil
}

// MultipleSignaturesFromBytes --
func MultipleSignaturesFromBytes(sigs [][]byte) ([]Signature, error) {
	if len(sigs) == 0 {
		return nil, errors.New("0 signatures provided to the method")
	}
	out := make([]Signature, len(sigs))
	for i, s := range sigs {
		sig, err := SignatureFromBytes(s)
		if err != nil {
			return nil, err
		}
		out[i] = sig
	}
	return out, nil
}

// AggregatePublicKeys --
func AggregatePublicKeys(pubs [][]byte) (PublicKey, error) {
	return blst.AggregatePublicKeys(pubs)
}

// AggregateMultiplePubkeys --
func AggregateMultiplePubkeys(pubs []PublicKey) PublicKey {
	return blst.AggregateMultiplePubkeys(pubs)
}

// AggregateSignatures --
func AggregateSignatures(sigs []common.Signature) common.Signature {
	if len(sigs) == 0 {
		return nil
	}
	return NewAggregateSignature()
}

// AggregateCompressedSignatures --
func AggregateCompressedSignatures(multiSigs [][]byte) (common.Signature, error) {
	for _, s := range multiSigs {
		if len(s) != fieldparams.BLSSignatureLength {
			return nil, fmt.Errorf("signature must be %d bytes", fieldparams.BLSSignatureLength)
		}
	}
	return NewAggregateSignature(), nil
}

// VerifySignature succeeds for any well-formed signature.
func VerifySignature(sig []byte, _ [32]byte, _ common.PublicKey) (bool, error) {
	if _, err := SignatureFromBytes(sig); err != nil {
		return false, err
	}
	return true, nil
}

// VerifyMultipleSignatures succeeds for any well-formed batch.
// Only malformed batches fail with structural errors.
func VerifyMultipleSignatures(sigs [][]byte, msgs [][32]byte, pubKeys []common.PublicKey) (bool, error) {
	if len(sigs) == 0 || len(pubKeys) == 0 {
		return false, nil
	}

	if len(sigs) != len(pubKeys) || len(sigs) != len(msgs) {
		return false, fmt.Errorf("provided signatures, pubkeys and messages have differing lengths. S: %d, P: %d,M %d",
			len(sigs), len(pubKeys), len(msgs))
	}

	for _, s := range sigs {
		if len(s) != fieldparams.BLSSignatureLength {
			return false, fmt.Errorf("signature must be %d bytes", fieldparams.BLSSignatureLength)
		}
	}
	return true, nil
}

// NewAggregateSignature --
func NewAggregateSignature() common.Signature {
	return newSignature()
}

// RandKey --
func RandKey() (common.SecretKey, error) {
	sk, err := blst.RandKey()
	if err != nil {
		return nil, fmt.Errorf("blst randkey: %w", err)
	}
	return &fakeSecretKey{sk}, nil
}

// Below are the methods that implement the common interfaces for the fake types.

// Sign returns the spec's stub signature.
func (s *fakeSecretKey) Sign(_ []byte) common.Signature {
	return newSignature()
}

// Verify always succeeds, as the spec does with BLS disabled.
func (s *fakeSignature) Verify(_ common.PublicKey, _ []byte) bool { return true }

// AggregateVerify succeeds unless the arguments are structurally invalid.
func (s *fakeSignature) AggregateVerify(pubKeys []common.PublicKey, msgs [][32]byte) bool {
	return len(pubKeys) == len(msgs)
}

// FastAggregateVerify always succeeds, as the spec does with BLS disabled.
func (s *fakeSignature) FastAggregateVerify(_ []common.PublicKey, _ [32]byte) bool { return true }

// Eth2FastAggregateVerify always succeeds, as the spec does with BLS disabled.
func (s *fakeSignature) Eth2FastAggregateVerify(_ []common.PublicKey, _ [32]byte) bool { return true }

// Marshal a signature into a byte slice.
func (s *fakeSignature) Marshal() []byte { return copyBytes(s.b) }

// Copy returns a full deep copy of a signature.
func (s *fakeSignature) Copy() common.Signature { return &fakeSignature{b: copyBytes(s.b)} }

func newSignature() *fakeSignature {
	return &fakeSignature{b: copyBytes(stubSignature)}
}

func copyBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
