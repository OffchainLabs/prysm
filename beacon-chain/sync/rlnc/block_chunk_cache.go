package rlnc

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

var numChunks = uint(10)
var maxChunkSize = uint(6554) // 2MB for 10 chunks.

type BlockChunkCache struct {
	sync.Mutex
	committer *committer
	nodes     map[primitives.Slot]map[primitives.ValidatorIndex]*Node
}

func NewBlockChunkCache() *BlockChunkCache {
	return &BlockChunkCache{
		committer: newCommitter(maxChunkSize),
		nodes:     make(map[primitives.Slot]map[primitives.ValidatorIndex]*Node),
	}
}

func (b *BlockChunkCache) AddChunk(chunk interfaces.ReadOnlyBeaconBlockChunk) error {
	b.Lock()
	defer b.Unlock()

	m, err := newMessage(chunk)
	if err != nil {
		return errors.Wrap(err, "failed to create new message")
	}
	if _, ok := b.nodes[chunk.Slot()]; !ok {
		b.nodes[chunk.Slot()] = make(map[primitives.ValidatorIndex]*Node)
	} else if n, ok := b.nodes[chunk.Slot()][chunk.ProposerIndex()]; ok {
		return n.receive(m)
	}
	node := NewNode(b.committer, numChunks)
	if err := node.receive(m); err != nil {
		return errors.Wrap(err, "failed to receive message")
	}
	b.nodes[chunk.Slot()][chunk.ProposerIndex()] = node
	return ErrSignatureNotVerified
}

// GetBlockData returns the block for the given slot and proposer index if all the chunks are present.
func (b *BlockChunkCache) GetBlockData(slot primitives.Slot, proposerIndex primitives.ValidatorIndex) ([]byte, error) {
	b.Lock()
	defer b.Unlock()

	if _, ok := b.nodes[slot]; !ok {
		return nil, ErrNoData
	}
	if _, ok := b.nodes[slot][proposerIndex]; !ok {
		return nil, ErrNoData
	}
	node := b.nodes[slot][proposerIndex]
	return node.decode() // Only error is ErrNoData when the node is not full.
}

// Prune removes all nodes from before the given slot.
func (b *BlockChunkCache) Prune(slot primitives.Slot) {
	b.Lock()
	defer b.Unlock()

	for s := range b.nodes {
		if s < slot {
			delete(b.nodes, s)
		}
	}
}

// RemoveNode removes the node that has the given chunk.
func (b *BlockChunkCache) RemoveNode(chunk interfaces.ReadOnlyBeaconBlockChunk) {
	b.Lock()
	defer b.Unlock()

	if _, ok := b.nodes[chunk.Slot()]; !ok {
		return
	}
	delete(b.nodes[chunk.Slot()], chunk.ProposerIndex())
}
