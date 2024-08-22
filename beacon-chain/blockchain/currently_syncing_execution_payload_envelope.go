package blockchain

import (
	"sync"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type currentlySyncingPayload struct {
	sync.Mutex
	roots map[[32]byte]primitives.PTCStatus
}

func (b *currentlySyncingPayload) set(envelope interfaces.ROExecutionPayloadEnvelope) {
	b.Lock()
	defer b.Unlock()
	if envelope.PayloadWithheld() {
		b.roots[envelope.BeaconBlockRoot()] = primitives.PAYLOAD_WITHHELD
	} else {
		b.roots[envelope.BeaconBlockRoot()] = primitives.PAYLOAD_PRESENT
	}
}

func (b *currentlySyncingPayload) unset(root [32]byte) {
	b.Lock()
	defer b.Unlock()
	delete(b.roots, root)
}

func (b *currentlySyncingPayload) isSyncing(root [32]byte) (status primitives.PTCStatus, isSyncing bool) {
	b.Lock()
	defer b.Unlock()
	status, isSyncing = b.roots[root]
	return
}
