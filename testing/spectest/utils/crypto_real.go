//go:build !fake_crypto

package utils

import "testing"

// FakeCrypto reports whether this binary was built with the fake BLS backend.
const FakeCrypto = false

func StubPubkeyAggregation(_ testing.TB) {}
