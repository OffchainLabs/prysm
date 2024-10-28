package blockchain

import "sync"

type currentlySyncingPayload struct {
	sync.Mutex
	roots map[[32]byte]struct{}
}

func (b *currentlySyncingPayload) set(root [32]byte) {
	b.Lock()
	defer b.Unlock()
	b.roots[root] = struct{}{}
}

func (b *currentlySyncingPayload) unset(root [32]byte) {
	b.Lock()
	defer b.Unlock()
	delete(b.roots, root)
}

func (b *currentlySyncingPayload) isSyncing(root [32]byte) bool {
	b.Lock()
	defer b.Unlock()
	_, ok := b.roots[root]
	return ok
}
