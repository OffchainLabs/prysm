// Package fifo provides a thread-safe FIFO (First-In-First-Out) cache implementation.
// It supports key-based lookups and maintains insertion order for eviction.
package fifo

import (
	"errors"
	"sync"
)

// ErrClosed is returned when operations are performed on a closed FIFO.
var ErrClosed = errors.New("fifo is closed")

// KeyFunc is a function that extracts a key from an object.
type KeyFunc func(obj interface{}) (string, error)

// PopProcessFunc is a function called during Pop to process the item.
// The boolean indicates whether the item was found.
type PopProcessFunc func(obj interface{}, isInInitialList bool) error

// FIFO is a thread-safe First-In-First-Out cache that supports key-based lookups.
// It provides O(1) lookups by key and O(1) insertions, with FIFO ordering for eviction.
type FIFO struct {
	lock    sync.RWMutex
	cond    sync.Cond
	items   map[string]interface{}
	queue   []string
	keyFunc KeyFunc
	closed  bool
}

// New creates a new FIFO cache with the given key function.
func New(keyFunc KeyFunc) *FIFO {
	f := &FIFO{
		items:   make(map[string]interface{}),
		queue:   make([]string, 0),
		keyFunc: keyFunc,
	}
	f.cond.L = &f.lock
	return f
}

// Add inserts an item into the FIFO. If an item with the same key exists,
// it updates the existing item but does not change its position in the queue.
func (f *FIFO) Add(obj interface{}) error {
	key, err := f.keyFunc(obj)
	if err != nil {
		return err
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	if f.closed {
		return ErrClosed
	}

	if _, exists := f.items[key]; !exists {
		f.queue = append(f.queue, key)
	}
	f.items[key] = obj
	f.cond.Broadcast()
	return nil
}

// AddIfNotPresent adds an item to the FIFO only if no item with the same key exists.
func (f *FIFO) AddIfNotPresent(obj interface{}) error {
	key, err := f.keyFunc(obj)
	if err != nil {
		return err
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	if f.closed {
		return ErrClosed
	}

	if _, exists := f.items[key]; exists {
		return nil
	}

	f.queue = append(f.queue, key)
	f.items[key] = obj
	f.cond.Broadcast()
	return nil
}

// Update updates an existing item in the FIFO.
func (f *FIFO) Update(obj interface{}) error {
	return f.Add(obj)
}

// Delete removes an item from the FIFO.
func (f *FIFO) Delete(obj interface{}) error {
	key, err := f.keyFunc(obj)
	if err != nil {
		return err
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	if f.closed {
		return ErrClosed
	}

	if _, exists := f.items[key]; !exists {
		return nil
	}

	delete(f.items, key)
	// Remove from queue
	for i, k := range f.queue {
		if k == key {
			f.queue = append(f.queue[:i], f.queue[i+1:]...)
			break
		}
	}
	return nil
}

// GetByKey returns the item associated with the given key.
func (f *FIFO) GetByKey(key string) (item interface{}, exists bool, err error) {
	f.lock.RLock()
	defer f.lock.RUnlock()

	if f.closed {
		return nil, false, ErrClosed
	}

	item, exists = f.items[key]
	return item, exists, nil
}

// ListKeys returns a list of all keys in the FIFO.
func (f *FIFO) ListKeys() []string {
	f.lock.RLock()
	defer f.lock.RUnlock()

	keys := make([]string, 0, len(f.items))
	for key := range f.items {
		keys = append(keys, key)
	}
	return keys
}

// List returns all items in the FIFO.
func (f *FIFO) List() []interface{} {
	f.lock.RLock()
	defer f.lock.RUnlock()

	items := make([]interface{}, 0, len(f.items))
	for _, item := range f.items {
		items = append(items, item)
	}
	return items
}

// Pop removes and returns the oldest item from the FIFO.
// It blocks until an item is available or the FIFO is closed.
func (f *FIFO) Pop(process PopProcessFunc) (interface{}, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	for {
		for len(f.queue) == 0 {
			if f.closed {
				return nil, ErrClosed
			}
			f.cond.Wait()
		}

		if f.closed {
			return nil, ErrClosed
		}

		// Get the oldest key
		key := f.queue[0]
		f.queue = f.queue[1:]

		item, exists := f.items[key]
		if !exists {
			// Item was deleted, try next item
			continue
		}

		delete(f.items, key)

		if process != nil {
			if err := process(item, false); err != nil {
				// Re-add on error (requeue behavior)
				f.items[key] = item
				f.queue = append([]string{key}, f.queue...)
				return nil, err
			}
		}

		return item, nil
	}
}

// Len returns the number of items in the FIFO.
func (f *FIFO) Len() int {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return len(f.items)
}

// Close closes the FIFO. After Close is called, Pop will return ErrClosed.
func (f *FIFO) Close() {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.closed = true
	f.cond.Broadcast()
}

// IsClosed returns true if the FIFO has been closed.
func (f *FIFO) IsClosed() bool {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.closed
}

// HasSynced returns true (for compatibility with k8s cache interface).
func (f *FIFO) HasSynced() bool {
	return true
}

// Resync is a no-op (for compatibility with k8s cache interface).
func (f *FIFO) Resync() error {
	return nil
}

// Replace replaces the contents of the FIFO with the given list.
func (f *FIFO) Replace(list []interface{}, _ string) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.closed {
		return ErrClosed
	}

	f.items = make(map[string]interface{})
	f.queue = make([]string, 0, len(list))

	for _, obj := range list {
		key, err := f.keyFunc(obj)
		if err != nil {
			return err
		}
		f.items[key] = obj
		f.queue = append(f.queue, key)
	}

	f.cond.Broadcast()
	return nil
}
