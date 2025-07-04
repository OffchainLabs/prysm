package cache

import (
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/crypto/hash"
)

type CellsWithTimestamp struct {
	recorded time.Time
	cells    []blocks.VerifiedROCell
}

// CellCache is a cache that keeps track of the prepared cell for the blob (kzg commitment hash)
type CellCache struct {
	versionedHashToCellMap map[[32]byte]CellsWithTimestamp
	sync.Mutex
}

// NewCellCache returns a new cell cache
func NewCellCache() *CellCache {
	return &CellCache{versionedHashToCellMap: make(map[[32]byte]CellsWithTimestamp)}
}

// IsCellAvailable checks if the column is available (as a set of cells)
func (c *CellCache) IsCellAvailable(kzgCommitments [][]byte, column uint64) []blocks.VerifiedROCell {
	c.Lock()
	defer c.Unlock()

	cells := make([]blocks.VerifiedROCell, 0)
	for _, kzgCommitment := range kzgCommitments {
		versionedHash := hash.Hash(kzgCommitment)

		cwt, ok := c.versionedHashToCellMap[versionedHash]
		if !ok {
			return nil
		}
		for _, cell := range cwt.cells {
			if cell.ColumnIndex == column {
				cells = append(cells, cell)
			}
		}
	}

	return cells
}

func (c *CellCache) Set(cell blocks.VerifiedROCell) {
	c.Lock()
	defer c.Unlock()

	versionedHash := hash.Hash(cell.KzgCommitment)

	// Get existing cells or create new entry
	cwt, exists := c.versionedHashToCellMap[versionedHash]
	if !exists {
		cwt = CellsWithTimestamp{
			cells:    make([]blocks.VerifiedROCell, 0),
			recorded: time.Now(),
		}
	}

	// Add the new cell
	cwt.cells = append(cwt.cells, cell)
	c.versionedHashToCellMap[versionedHash] = cwt
}

func (c *CellCache) prune() {
	// todo(healthykim): parameter tuning
	// prune cells staged 5 slot ahead
	deadline := time.Now().Add(-1 * time.Minute)

	for key, value := range c.versionedHashToCellMap {
		if value.recorded.Before(deadline) {
			delete(c.versionedHashToCellMap, key)
		}

	}
}
