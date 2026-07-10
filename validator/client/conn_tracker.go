package client

import (
	"sync"

	"github.com/sirupsen/logrus"
)

// pushKind identifies a category of data pushed to the beacon node that must
// be re-pushed after a host switch.
type pushKind string

const (
	proposerPrefsPush pushKind = "proposer-preferences"
	registrationsPush pushKind = "validator-registrations"
)

// connTracker records, per push kind, the connection generation at which
// that push was last confirmed, so each kind independently retries a failed
// re-push after a beacon host switch. The zero value is ready to use.
type connTracker struct {
	mu        sync.Mutex
	confirmed map[pushKind]uint64
}

// changed reports whether gen differs from the last confirmed push of kind.
func (t *connTracker) changed(kind pushKind, gen uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return gen != t.confirmed[kind]
}

// confirm records gen as kind's last confirmed generation, moving forward only
// so a stale in-flight push can't mask a newer host switch.
func (t *connTracker) confirm(kind pushKind, gen uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if gen <= t.confirmed[kind] {
		return
	}
	if t.confirmed == nil {
		t.confirmed = make(map[pushKind]uint64)
	}
	t.confirmed[kind] = gen
	log.WithFields(logrus.Fields{
		"kind":           kind,
		"connGeneration": gen,
	}).Debug("Confirmed push for new beacon connection")
}

// connGeneration returns the current beacon-node connection generation.
func (v *validator) connGeneration() uint64 {
	return v.validatorClient.ConnectionGeneration()
}
