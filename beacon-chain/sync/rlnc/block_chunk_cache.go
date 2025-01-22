package rlnc

import (
	"sync"

	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

var numChunks = uint(10)
var maxChunkSize = uint(6554) // 2MB for 10 chunks.

type BlockChunkCache struct {
	sync.Mutex
	committer *Committer
	nodes     map[primitives.Slot]map[primitives.ValidatorIndex]*Node
}

func NewBlockChunkCache() *BlockChunkCache {
	committer, err := LoadTrustedSetup()
	if err != nil {
		panic("cannot load the RLNC trusted setup") // lint:nopanic -- TODO: Do not panic!
	}
	return &BlockChunkCache{
		committer: committer,
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

// PrepareMessage prepares a message to broadcast after receiving the given chunk.
func (b *BlockChunkCache) PrepareMessage(chunk interfaces.ReadOnlyBeaconBlockChunk) (*ethpb.BeaconBlockChunk, error) {
	b.Lock()
	defer b.Unlock()

	if _, ok := b.nodes[chunk.Slot()]; !ok {
		return nil, ErrNoData
	}
	if _, ok := b.nodes[chunk.Slot()][chunk.ProposerIndex()]; !ok {
		return nil, ErrNoData
	}
	node := b.nodes[chunk.Slot()][chunk.ProposerIndex()]
	msg, err := node.prepareMessage()
	if err != nil {
		return nil, errors.Wrap(err, "failed to prepare message")
	}
	signature := chunk.Signature()
	return &ethpb.BeaconBlockChunk{
		Data:         msg.Data(),
		Coefficients: msg.Coefficients(),
		Header:       chunk.Header(),
		Signature:    signature[:],
	}, nil
}
