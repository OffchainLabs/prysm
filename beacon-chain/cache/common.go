package cache

import (
	"k8s.io/client-go/tools/cache"
)

// trim the FIFO queue to the maxSize.
func trim(queue *cache.FIFO, maxSize uint64) {
	for s := uint64(len(queue.ListKeys())); s > maxSize; s-- {
		_, err := queue.Pop(popProcessNoopFunc)
		if err != nil {
			// popProcessNoopFunc never returns an error, but we handle this anyway to make linter
			// happy.
			return
		}
	}
}

// popProcessNoopFunc is a no-op function that never returns an error.
func popProcessNoopFunc(_ interface{}, _ bool) error {
	return nil
}
