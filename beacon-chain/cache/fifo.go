package cache

import (
	"sync"
)

// KeyFunc knows how to make a key from an object. Implementations should be deterministic.
type KeyFunc func(obj interface{}) (string, error)


// PopProcessFunc is passed to Pop() method.
// It is supposed to process the accumulator popped from the queue.
type PopProcessFunc func(obj interface{}) error

// FIFO is a thread-safe FIFO queue that maintains the most recent version
// of each object identified by its key. Objects are processed in FIFO order,
// but duplicate keys are deduplicated - only the latest version is kept.
//
// Key features:
// - Thread-safe operations with RWMutex
// - Deduplication based on key function
// - Simple FIFO queue for cache trimming
type FIFO struct {
	lock    sync.RWMutex
	keyFunc KeyFunc
	items   map[string]interface{}
	queue   []string
}

// Add inserts an item, and puts it in the queue. The item is only enqueued
// if it doesn't already exist in the set. But the item value is always updated.
func (f *FIFO) Add(obj interface{}) error {
	id, err := f.keyFunc(obj)
	if err != nil {
		return err
	}
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, exists := f.items[id]; !exists {
		f.queue = append(f.queue, id)
	}
	f.items[id] = obj
	return nil
}

// AddIfNotPresent inserts an item, and puts it in the queue. If the item is already
// present in the set, it is neither enqueued nor added to the set.
//
// This is useful in a single producer/consumer scenario so that the consumer can
// safely retry items without contending with the producer and potentially enqueueing
// stale items.
func (f *FIFO) AddIfNotPresent(obj interface{}) error {
	id, err := f.keyFunc(obj)
	if err != nil {
		return err
	}
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, exists := f.items[id]; exists {
		return nil
	}

	f.queue = append(f.queue, id)
	f.items[id] = obj
	return nil
}

// Delete removes an item. It doesn't add it to the queue, because
// this implementation assumes the consumer only cares about the objects,
// not the order in which they were created/added.
func (f *FIFO) Delete(obj interface{}) error {
	id, err := f.keyFunc(obj)
	if err != nil {
		return err
	}
	f.lock.Lock()
	defer f.lock.Unlock()
	delete(f.items, id)
	return nil
}

// Len returns the number of items in the FIFO.
func (f *FIFO) Len() int {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return len(f.items)
}

// List returns a list of all the items.
func (f *FIFO) List() []interface{} {
	f.lock.RLock()
	defer f.lock.RUnlock()
	list := make([]interface{}, 0, len(f.items))
	for _, item := range f.items {
		list = append(list, item)
	}
	return list
}

// GetByKey returns the requested item, or sets exists=false.
func (f *FIFO) GetByKey(key string) (item interface{}, exists bool, err error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	item, exists = f.items[key]
	return item, exists, nil
}

// Pop waits until an item is ready and processes it. If multiple items are
// ready, they are returned in the order in which they were added/updated.
// The item is removed from the queue (and the store) before it is processed,
// so if you don't successfully process it, you need to add it back with
// AddIfNotPresent(). process function is called under lock, so it is safe
// update data structures in it that need to be in sync with the queue.
func (f *FIFO) Pop(process PopProcessFunc) (interface{}, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if len(f.queue) == 0 {
		return nil, nil
	}
	id := f.queue[0]
	f.queue = f.queue[1:]
	item, ok := f.items[id]
	if !ok {
		// Item may have been deleted subsequently.
		return nil, nil
	}
	delete(f.items, id)
	err := process(item)
	return item, err
}

// NewFIFO returns a Store which can be used to queue up items to process.
func NewFIFO(keyFunc KeyFunc) *FIFO {
	return &FIFO{
		items:   map[string]interface{}{},
		queue:   []string{},
		keyFunc: keyFunc,
	}
}