package client

// signingLockNamespace keeps the per-validator attestation and proposal locks
// distinct without coupling lock identity to the slot scheduler's data model.
type signingLockNamespace byte

const (
	attestationSigningLock signingLockNamespace = iota + 1
	proposalSigningLock
)
