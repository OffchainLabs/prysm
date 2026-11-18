// Package vrftree builds the VRF tree of a leanXMSS validator, and opens its leaves.
//
// The tree is a Merkle tree of depth `VRF_TREE_DEPTH` whose leaves are secret values, one for each slot.
// Its root is the validator's VRF root. Its proof for a slot, a `VRFProof`, is the leaf at index `slot`
// followed by the leaf's Merkle branch: the validator reveals it as its RANDAO reveal when it proposes,
// and as its selection proof when it aggregates.
//
// The spec does not say how the leaves are derived. Here, they are derived from the validator's XMSS seed,
// so the XMSS seed is the only secret of a validator:
// the leaf at index `i` is `sha256(vrf_seed || uint_to_bytes32(i))`, where `vrf_seed = sha256(xmss_seed || "VRF_SEED")`.
//
// The tree has 2^31 leaves: storing it would take 128 GiB. So the package caches only its upper levels,
// down to the roots of its subtrees of height `subtreeHeight`. Building the tree hashes every leaf once.
// Opening a leaf recomputes the subtree it is in, and reads the rest of its branch from the cached upper levels.
// This only changes how the tree is stored: its root and proofs are those of a plain Merkle tree.
//
// Building the tree takes tens of seconds. To build it only once, save its `SubtreeRoots` (256 KiB),
// and rebuild it from them with `FromSubtreeRoots`, which hashes the upper levels only.
package vrftree

import (
	"context"
	"encoding/binary"
	"fmt"
	"runtime"
	"sync/atomic"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	"golang.org/x/sync/errgroup"
)

const (
	Depth         = 31 // Depth of the tree (`VRF_TREE_DEPTH`).
	subtreeHeight = 18 // Height of the subtrees recomputed to open a leaf.
)

// vrfSeedDomain separates the VRF seed from the other secrets derived from the XMSS seed.
var vrfSeedDomain = []byte("VRF_SEED")

// Tree is the VRF tree of a validator.
type Tree struct {
	seed          [32]byte // VRF seed, from which the leaves are derived.
	depth         uint64
	subtreeHeight uint64

	// upper holds the cached upper levels of the tree: upper[0] holds the roots of the subtrees,
	// and upper[len(upper)-1] holds a single node, the root of the tree.
	upper [][][32]byte
}

// New builds the VRF tree of the validator whose XMSS seed is `xmssSeed`.
// It hashes the 2^31 leaves of the tree.
func New(ctx context.Context, xmssSeed [32]byte) (*Tree, error) {
	return newTree(ctx, xmssSeed, Depth, subtreeHeight)
}

// newTree builds the VRF tree of `depth` of the validator whose XMSS seed is `xmssSeed`,
// caching its upper levels down to the roots of its subtrees of `subtreeHeight`.
func newTree(ctx context.Context, xmssSeed [32]byte, depth, subtreeHeight uint64) (*Tree, error) {
	tree, err := newEmptyTree(xmssSeed, depth, subtreeHeight)
	if err != nil {
		return nil, fmt.Errorf("new empty tree: %w", err)
	}

	subtreeCount := tree.subtreeCount()

	roots := make([][32]byte, subtreeCount)
	next := atomic.Uint64{}

	workerCount := min(uint64(runtime.GOMAXPROCS(0)), subtreeCount)
	group, ctx := errgroup.WithContext(ctx)
	for range workerCount {
		group.Go(func() error {
			chunks, levels := newSubtreeBuffers(subtreeHeight)

			for {
				if err := ctx.Err(); err != nil {
					return err
				}

				subtree := next.Add(1) - 1
				if subtree >= subtreeCount {
					return nil
				}

				if err := tree.hashSubtree(subtree<<subtreeHeight, chunks, levels); err != nil {
					return fmt.Errorf("hash subtree %d: %w", subtree, err)
				}

				roots[subtree] = levels[subtreeHeight][0]
			}
		})
	}

	if err := group.Wait(); err != nil {
		return nil, fmt.Errorf("hash subtrees: %w", err)
	}

	if err := tree.setSubtreeRoots(roots); err != nil {
		return nil, fmt.Errorf("set subtree roots: %w", err)
	}

	return tree, nil
}

// FromSubtreeRoots returns the VRF tree of the validator whose XMSS seed is `xmssSeed`,
// from `subtreeRoots`, the roots of its subtrees returned by `SubtreeRoots`. It is fast: it hashes the upper levels only.
// It refuses `subtreeRoots` if the root of the tree is not `expectedRoot`, the validator's VRF root.
func FromSubtreeRoots(xmssSeed [32]byte, subtreeRoots [][32]byte, expectedRoot [32]byte) (*Tree, error) {
	return fromSubtreeRoots(xmssSeed, subtreeRoots, expectedRoot, Depth, subtreeHeight)
}

// fromSubtreeRoots is `FromSubtreeRoots` for a tree of `depth` with subtrees of `subtreeHeight`.
func fromSubtreeRoots(xmssSeed [32]byte, subtreeRoots [][32]byte, expectedRoot [32]byte, depth, subtreeHeight uint64) (*Tree, error) {
	tree, err := newEmptyTree(xmssSeed, depth, subtreeHeight)
	if err != nil {
		return nil, fmt.Errorf("new empty tree: %w", err)
	}

	if uint64(len(subtreeRoots)) != tree.subtreeCount() {
		return nil, fmt.Errorf("got %d subtree roots, expected %d", len(subtreeRoots), tree.subtreeCount())
	}

	roots := make([][32]byte, len(subtreeRoots))
	copy(roots, subtreeRoots)

	if err := tree.setSubtreeRoots(roots); err != nil {
		return nil, fmt.Errorf("set subtree roots: %w", err)
	}

	if root := tree.Root(); root != expectedRoot {
		return nil, fmt.Errorf("root %#x of the subtree roots is not the expected root %#x", root, expectedRoot)
	}

	return tree, nil
}

// SubtreeRoots returns the roots of the subtrees of the tree, from which `FromSubtreeRoots` rebuilds the tree.
func (t *Tree) SubtreeRoots() [][32]byte {
	roots := make([][32]byte, len(t.upper[0]))
	copy(roots, t.upper[0])
	return roots
}

// Root returns the root of the tree, the validator's VRF root.
func (t *Tree) Root() [32]byte {
	return t.upper[len(t.upper)-1][0]
}

// Leaf returns the leaf of the tree at index `slot`, the first element of the proof of `slot`.
// It is cheap: it does not open the leaf.
func (t *Tree) Leaf(slot primitives.Slot) ([32]byte, error) {
	if err := t.checkSlot(slot); err != nil {
		return [32]byte{}, err
	}

	chunk := leafChunk(t.seed, uint64(slot))
	return hash.Hash(chunk[:]), nil
}

// Proof returns the proof of `slot` (`VRFProof`): the leaf of the tree at index `slot`, followed by its Merkle branch.
func (t *Tree) Proof(slot primitives.Slot) ([][32]byte, error) {
	if err := t.checkSlot(slot); err != nil {
		return nil, err
	}

	index := uint64(slot)
	subtree := index >> t.subtreeHeight
	local := index & (1<<t.subtreeHeight - 1) // Index of the leaf in its subtree.

	chunks, levels := newSubtreeBuffers(t.subtreeHeight)
	if err := t.hashSubtree(subtree<<t.subtreeHeight, chunks, levels); err != nil {
		return nil, fmt.Errorf("hash subtree %d: %w", subtree, err)
	}

	proof := make([][32]byte, 0, t.depth+1)
	proof = append(proof, levels[0][local])

	// The branch in the subtree.
	for level := range t.subtreeHeight {
		proof = append(proof, levels[level][(local>>level)^1])
	}

	// The branch in the cached upper levels.
	for level := range t.depth - t.subtreeHeight {
		proof = append(proof, t.upper[level][(subtree>>level)^1])
	}

	// Never reveal an invalid proof, for instance if the cached upper levels are corrupted.
	if !isValidMerkleBranch(proof[0], proof[1:], index, t.Root()) {
		return nil, fmt.Errorf("proof of slot %d does not match the root of the tree", slot)
	}

	return proof, nil
}

// newEmptyTree returns the tree of `depth` with subtrees of `subtreeHeight` of the validator whose XMSS seed is `xmssSeed`,
// without its upper levels.
func newEmptyTree(xmssSeed [32]byte, depth, subtreeHeight uint64) (*Tree, error) {
	if subtreeHeight == 0 || subtreeHeight > depth {
		return nil, fmt.Errorf("subtree height %d is not between 1 and the depth %d", subtreeHeight, depth)
	}

	tree := &Tree{
		seed:          vrfSeed(xmssSeed),
		depth:         depth,
		subtreeHeight: subtreeHeight,
	}

	return tree, nil
}

// subtreeCount returns the number of subtrees of the tree.
func (t *Tree) subtreeCount() uint64 {
	return uint64(1) << (t.depth - t.subtreeHeight)
}

// setSubtreeRoots sets the upper levels of the tree, from the roots of its subtrees.
func (t *Tree) setSubtreeRoots(roots [][32]byte) error {
	upper, err := hashLevels(roots, t.depth-t.subtreeHeight)
	if err != nil {
		return fmt.Errorf("hash upper levels: %w", err)
	}

	t.upper = upper
	return nil
}

// checkSlot checks that the tree has a leaf at index `slot`.
func (t *Tree) checkSlot(slot primitives.Slot) error {
	if uint64(slot) >= 1<<t.depth {
		return fmt.Errorf("slot %d is beyond the %d leaves of the tree", slot, uint64(1)<<t.depth)
	}

	return nil
}

// hashSubtree fills `levels` with the subtree of `subtreeHeight` whose first leaf is at index `first`:
// levels[0] holds its leaves, and levels[t.subtreeHeight] holds a single node, its root.
// `chunks` and `levels` come from `newSubtreeBuffers`.
func (t *Tree) hashSubtree(first uint64, chunks [][32]byte, levels [][][32]byte) error {
	for i := range levels[0] {
		chunks[2*i] = t.seed
		chunks[2*i+1] = indexChunk(first + uint64(i))
	}

	if err := htr.Hash(levels[0], chunks); err != nil {
		return fmt.Errorf("hash leaves: %w", err)
	}

	for level := 1; level < len(levels); level++ {
		if err := htr.Hash(levels[level], levels[level-1]); err != nil {
			return fmt.Errorf("hash level %d: %w", level, err)
		}
	}

	return nil
}

// newSubtreeBuffers returns the buffers `hashSubtree` fills for a subtree of `height`.
func newSubtreeBuffers(height uint64) (chunks [][32]byte, levels [][][32]byte) {
	chunks = make([][32]byte, 2<<height)

	levels = make([][][32]byte, height+1)
	for level := range levels {
		levels[level] = make([][32]byte, 1<<(height-uint64(level)))
	}

	return chunks, levels
}

// hashLevels returns the levels of the tree of `height` over `leaves`:
// levels[0] is `leaves`, and levels[height] holds a single node, the root.
func hashLevels(leaves [][32]byte, height uint64) ([][][32]byte, error) {
	levels := make([][][32]byte, 0, height+1)
	levels = append(levels, leaves)

	for level := uint64(1); level <= height; level++ {
		previous := levels[level-1]
		current := make([][32]byte, len(previous)/2)
		if err := htr.Hash(current, previous); err != nil {
			return nil, fmt.Errorf("hash level %d: %w", level, err)
		}

		levels = append(levels, current)
	}

	return levels, nil
}

// isValidMerkleBranch checks that `branch` opens `leaf` at `index` of the tree whose root is `root` (`is_valid_merkle_branch`).
func isValidMerkleBranch(leaf [32]byte, branch [][32]byte, index uint64, root [32]byte) bool {
	node := leaf
	chunk := [64]byte{}
	for level, sibling := range branch {
		if (index>>level)&1 == 1 {
			copy(chunk[:32], sibling[:])
			copy(chunk[32:], node[:])
			node = hash.Hash(chunk[:])

			continue
		}

		copy(chunk[:32], node[:])
		copy(chunk[32:], sibling[:])
		node = hash.Hash(chunk[:])
	}

	return node == root
}

// vrfSeed returns the VRF seed derived from `xmssSeed`: `sha256(xmss_seed || "VRF_SEED")`.
func vrfSeed(xmssSeed [32]byte) [32]byte {
	return hash.Hash(append(xmssSeed[:], vrfSeedDomain...))
}

// leafChunk returns the 64 bytes whose hash is the leaf at `index`: `seed || uint_to_bytes32(index)`.
func leafChunk(seed [32]byte, index uint64) [64]byte {
	chunk := [64]byte{}
	indexBytes := indexChunk(index)
	copy(chunk[:32], seed[:])
	copy(chunk[32:], indexBytes[:])
	return chunk
}

// indexChunk returns `uint_to_bytes32(index)`.
func indexChunk(index uint64) [32]byte {
	chunk := [32]byte{}
	binary.LittleEndian.PutUint64(chunk[:8], index)
	return chunk
}
