package client

// signingLockNamespace keeps per-validator attestation and proposal locks distinct.
type signingLockNamespace byte

const (
	attestationSigningLock signingLockNamespace = iota + 1
	proposalSigningLock
)
