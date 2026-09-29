//go:build fake_crypto

package main

// The fake BLS backend accepts any signature and signs with a stub, so it must
// never reach a released binary. This reference is deliberately undefined: the
// build fails naming it.
var _ = fakeCryptoMustNotBeBuiltIntoPrysmctl
