package cache

import (
	"crypto/sha256"
	"sync"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type InclusionLists struct {
	mu  sync.RWMutex
	ils map[primitives.Slot]map[primitives.ValidatorIndex]struct {
		txs       [][]byte
		seenTwice bool
	}
}

// NewInclusionLists initializes a new InclusionLists instance.
func NewInclusionLists() *InclusionLists {
	return &InclusionLists{
		ils: make(map[primitives.Slot]map[primitives.ValidatorIndex]struct {
			txs       [][]byte
			seenTwice bool
		}),
	}
}

// Add adds a set of transactions for a specific slot and validator index.
func (i *InclusionLists) Add(slot primitives.Slot, validatorIndex primitives.ValidatorIndex, txs [][]byte) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if _, ok := i.ils[slot]; !ok {
		i.ils[slot] = make(map[primitives.ValidatorIndex]struct {
			txs       [][]byte
			seenTwice bool
		})
	}

	entry := i.ils[slot][validatorIndex]
	if entry.seenTwice {
		return // No need to modify if already marked as seen twice.
	}

	if entry.txs == nil {
		entry.txs = txs
	} else {
		entry.seenTwice = true
		entry.txs = nil // Clear transactions to save space if seen twice.
	}
	i.ils[slot][validatorIndex] = entry
}

// Get retrieves unique transactions for a specific slot.
func (i *InclusionLists) Get(slot primitives.Slot) [][]byte {
	i.mu.RLock()
	defer i.mu.RUnlock()

	ils, exists := i.ils[slot]
	if !exists {
		return [][]byte{}
	}

	var uniqueTxs [][]byte
	seen := make(map[[32]byte]struct{})
	for _, entry := range ils {
		for _, tx := range entry.txs {
			hash := sha256.Sum256(tx)
			if _, duplicate := seen[hash]; !duplicate {
				uniqueTxs = append(uniqueTxs, tx)
				seen[hash] = struct{}{}
			}
		}
	}
	return uniqueTxs
}

// Delete removes all inclusion lists for a specific slot.
func (i *InclusionLists) Delete(slot primitives.Slot) {
	i.mu.Lock()
	defer i.mu.Unlock()

	delete(i.ils, slot)
}

// SeenTwice checks if a validator's transactions were marked as seen twice for a specific slot.
func (i *InclusionLists) SeenTwice(slot primitives.Slot, idx primitives.ValidatorIndex) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	ils, exists := i.ils[slot]
	if !exists {
		return false
	}

	entry, exists := ils[idx]
	return exists && entry.seenTwice
}
