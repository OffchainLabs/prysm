//go:build fake_crypto

package utils

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/crypto/bls"
)

// FakeCrypto reports whether this binary was built with the fake BLS backend.
const FakeCrypto = true

func StubPubkeyAggregation(t testing.TB) {
	bls.SetStubPubkeyAggregation(true)
	t.Cleanup(func() { bls.SetStubPubkeyAggregation(false) })
}
