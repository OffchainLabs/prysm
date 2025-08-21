package blockchain

import "sync"

type currentlySyncingBlock struct {
	sync.Mutex
	roots map[[32]byte]struct{}
}

func (b *currentlySyncingBlock) set(root [32]byte) error {
	b.Lock()
	defer b.Unlock()
	_, ok := b.roots[root]
	if ok {
		return errBlockBeingSynced
	}
	b.roots[root] = struct{}{}
	return nil
}

func (b *currentlySyncingBlock) unset(root [32]byte) {
	b.Lock()
	defer b.Unlock()
	delete(b.roots, root)
}

func (b *currentlySyncingBlock) isSyncing(root [32]byte) bool {
	b.Lock()
	defer b.Unlock()
	_, ok := b.roots[root]
	return ok
}
