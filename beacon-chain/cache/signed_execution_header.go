package cache

import (
	"bytes"
	"sync"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

// ExecutionPayloadHeaders is used  by the sync service to store signed execution payload headers after they pass validation,
// and filter out subsequent headers with lower value.
// The signed header from this cache could be used by the proposer when proposing the next slot.
type ExecutionPayloadHeaders struct {
	headers map[primitives.Slot][]*enginev1.SignedExecutionPayloadHeader
	sync.RWMutex
}

func NewExecutionPayloadHeaders() *ExecutionPayloadHeaders {
	return &ExecutionPayloadHeaders{
		headers: make(map[primitives.Slot][]*enginev1.SignedExecutionPayloadHeader),
	}
}

// SaveSignedExecutionPayloadHeader saves the signed execution payload header to the cache.
// The cache stores headers for up to two slots. If the input slot is higher than the lowest slot
// currently in the cache, the lowest slot is removed to make space for the new header.
// Only the highest value header for a given parent block hash will be stored.
// This function assumes caller already checks header's slot is current or next slot, it doesn't account for slot validation.
func (c *ExecutionPayloadHeaders) SaveSignedExecutionPayloadHeader(header *enginev1.SignedExecutionPayloadHeader) {
	c.Lock()
	defer c.Unlock()

	for s := range c.headers {
		if s+1 < header.Message.Slot {
			delete(c.headers, s)
		}
	}

	// Add or update the header in the map
	if _, ok := c.headers[header.Message.Slot]; !ok {
		c.headers[header.Message.Slot] = []*enginev1.SignedExecutionPayloadHeader{header}
	} else {
		found := false
		for i, h := range c.headers[header.Message.Slot] {
			if bytes.Equal(h.Message.ParentBlockHash, header.Message.ParentBlockHash) && bytes.Equal(h.Message.ParentBlockRoot, header.Message.ParentBlockRoot) {
				if header.Message.Value > h.Message.Value {
					c.headers[header.Message.Slot][i] = header
				}
				found = true
				break
			}
		}
		if !found {
			c.headers[header.Message.Slot] = append(c.headers[header.Message.Slot], header)
		}
	}
}

// SignedExecutionPayloadHeader returns the signed payload header for the given slot and parent block hash and block root.
// Returns nil if the header is not found.
// This should be used when the caller wants the header to match parent block hash and parent block root such as proposer choosing a header to propose.
func (c *ExecutionPayloadHeaders) SignedExecutionPayloadHeader(slot primitives.Slot, parentBlockHash []byte, parentBlockRoot []byte) *enginev1.SignedExecutionPayloadHeader {
	c.RLock()
	defer c.RUnlock()

	if headers, ok := c.headers[slot]; ok {
		for _, header := range headers {
			if bytes.Equal(header.Message.ParentBlockHash, parentBlockHash) && bytes.Equal(header.Message.ParentBlockRoot, parentBlockRoot) {
				return header
			}
		}
	}

	return nil
}
