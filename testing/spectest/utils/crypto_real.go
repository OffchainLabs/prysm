//go:build !fake_crypto

package utils

// FakeCrypto reports whether this binary was built with the fake BLS backend.
const FakeCrypto = false
