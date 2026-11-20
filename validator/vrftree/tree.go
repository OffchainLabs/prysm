// Package vrftree builds the VRF tree of a leanXMSS validator, and opens its leaves.
//
// The tree is a Merkle tree of depth `VRF_TREE_DEPTH` whose leaves are secret values, one for each slot.
// Its root is the validator's VRF root. Its proof for a slot, a `VRFProof`, is the leaf at index `slot`
// followed by the leaf's Merkle branch: the validator reveals it as its RANDAO reveal when it proposes,
// and as its selection proof when it aggregates.
//
// The spec does not say how the leaves are derived. Here, they are derived as the ethPandaOps genesis tool
// (https://github.com/ethpandaops/eth-beacon-genesis/tree/pq) and Lighthouse derive them, from the validator's
// BLS secret key `sk`:
//   - `vrf_seed = sha256("vrf-seed" || sk)`,
//   - the leaf at index `i < 2^PopulatedDepth` is `keccak256(vrf_seed || uint64_le(i))`,
//   - a node is `keccak256(left || right)`.
//
// The validator client derives the first 2^PopulatedDepth leaves only. Above them, the tree is either:
//   - padded: every leaf is zero (`New`), or
//   - full: the leaves are only known to the genesis tool, which publishes, for each validator, the Merkle branch
//     of the leaf of slot 0 (`NewFull`). The upper part of this branch joins the derived leaves to the VRF root.
//
// Storing the derived leaves would take 32 MiB per key. So the package caches only the upper levels of the derived
// part, down to the roots of its subtrees of height `subtreeHeight`. Building the tree hashes every derived leaf once.
// Opening a leaf recomputes the subtree it is in, and reads the rest of its branch from the cached upper levels.
// This only changes how the tree is stored: its root and proofs are those of a plain Merkle tree.
package vrftree

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"runtime"
	"sync/atomic"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"golang.org/x/crypto/sha3"
	"golang.org/x/sync/errgroup"
)

const (
	Depth          = 31 // Depth of the tree (`VRF_TREE_DEPTH`).
	PopulatedDepth = 20 // The validator client derives the leaves below index 2^PopulatedDepth only.
	subtreeHeight  = 14 // Height of the subtrees recomputed to open a leaf.
)

var (
	vrfSeedDomain = []byte("vrf-seed")  // Separates the VRF seed from the other secrets derived from the BLS secret key.
	zeroHashes    = computeZeroHashes() // zeroHashes[level] is the root of a subtree of height `level` whose leaves are all zero.
)

// Tree is the VRF tree of a validator.
type Tree struct {
	seed           [32]byte // VRF seed, from which the leaves are derived.
	depth          uint64
	populatedDepth uint64
	subtreeHeight  uint64

	// upper holds the cached upper levels of the derived part of the tree, from level `subtreeHeight` to level
	// `populatedDepth`. upper[0] holds the roots of the subtrees, and upper[len(upper)-1] holds a single node,
	// the root of the derived part.
	upper [][][32]byte

	// full is true if the leaves from index 2^populatedDepth on are not known, and false if they are zero.
	full bool

	// siblings holds the siblings of the root of the derived part, from level `populatedDepth` to level `depth - 1`.
	// They are the roots of zero subtrees in a padded tree, and the published ones in a full tree.
	siblings [][32]byte

	// spine holds, from level `populatedDepth` to level `depth`, the node of index 0 of each level.
	// spine[0] is the root of the derived part, and spine[len(spine)-1] is the root of the tree.
	spine [][32]byte
}

// New builds the padded VRF tree of the validator whose BLS secret key is `secretKey`: its leaves from index
// 2^PopulatedDepth on are zero. It hashes the 2^PopulatedDepth derived leaves of the tree.
func New(ctx context.Context, secretKey bls.SecretKey) (*Tree, error) {
	if secretKey == nil {
		return nil, errors.New("nil secret key")
	}

	return newTree(ctx, vrfSeed(secretKey.Marshal()), Depth, PopulatedDepth, subtreeHeight, nil)
}

// NewFull builds the full VRF tree of the validator whose BLS secret key is `secretKey`, whose root is `root`, and in
// which `branch` is the Merkle branch of the leaf of slot 0, as the genesis tool publishes them.
// It hashes the 2^PopulatedDepth derived leaves of the tree, and checks that they match `root` and `branch`.
// The tree opens the leaves below index 2^PopulatedDepth only.
func NewFull(ctx context.Context, secretKey bls.SecretKey, root [32]byte, branch [Depth][32]byte) (*Tree, error) {
	if secretKey == nil {
		return nil, errors.New("nil secret key")
	}

	return newFullTree(ctx, vrfSeed(secretKey.Marshal()), Depth, PopulatedDepth, subtreeHeight, root, branch[:])
}

// newFullTree builds the full tree of `depth`, whose leaves below index 2^`populatedDepth` are derived from `seed`,
// whose root is `root`, and in which `branch` is the Merkle branch of the leaf at index 0.
func newFullTree(
	ctx context.Context,
	seed [32]byte,
	depth, populatedDepth, subtreeHeight uint64,
	root [32]byte,
	branch [][32]byte,
) (*Tree, error) {
	if uint64(len(branch)) != depth {
		return nil, fmt.Errorf("branch has %d nodes instead of %d", len(branch), depth)
	}

	// In a padded tree, the siblings of the derived part are zero subtrees. A full tree has none.
	siblings := branch[populatedDepth:]
	for i, sibling := range siblings {
		level := populatedDepth + uint64(i)
		if sibling == zeroHashes[level] {
			return nil, fmt.Errorf("sibling of level %d of the branch is the root of a zero subtree", level)
		}
	}

	tree, err := newTree(ctx, seed, depth, populatedDepth, subtreeHeight, siblings)
	if err != nil {
		return nil, err
	}

	if actual := tree.Root(); actual != root {
		return nil, fmt.Errorf("root %#x of the derived leaves and the branch is not the root %#x", actual, root)
	}

	proof, err := tree.Proof(0)
	if err != nil {
		return nil, fmt.Errorf("proof of slot 0: %w", err)
	}

	for level, node := range proof[1:] {
		if node != branch[level] {
			return nil, fmt.Errorf("node of level %d of the branch does not match the derived leaves", level)
		}
	}

	return tree, nil
}

// newTree builds the tree of `depth` whose leaves below index 2^`populatedDepth` are derived from the VRF seed `seed`.
// The tree is full if `siblings`, the siblings of the root of the derived part, are given, and padded otherwise.
// It caches the upper levels of the derived part down to the roots of its subtrees of `subtreeHeight`.
func newTree(ctx context.Context, seed [32]byte, depth, populatedDepth, subtreeHeight uint64, siblings [][32]byte) (*Tree, error) {
	if subtreeHeight == 0 || subtreeHeight > populatedDepth || populatedDepth > depth || depth > Depth {
		return nil, fmt.Errorf(
			"subtree height %d, populated depth %d and depth %d are not ordered between 1 and %d",
			subtreeHeight, populatedDepth, depth, Depth,
		)
	}

	tree := &Tree{
		seed:           seed,
		depth:          depth,
		populatedDepth: populatedDepth,
		subtreeHeight:  subtreeHeight,
		full:           siblings != nil,
		siblings:       siblings,
	}

	if siblings == nil {
		tree.siblings = zeroHashes[populatedDepth:depth]
	}

	if uint64(len(tree.siblings)) != depth-populatedDepth {
		return nil, fmt.Errorf("%d siblings instead of %d", len(tree.siblings), depth-populatedDepth)
	}

	subtreeCount := tree.subtreeCount()

	roots := make([][32]byte, subtreeCount)
	next := atomic.Uint64{}

	workerCount := min(uint64(runtime.GOMAXPROCS(0)), subtreeCount)
	group, ctx := errgroup.WithContext(ctx)
	for range workerCount {
		group.Go(func() error {
			hasher := sha3.NewLegacyKeccak256()
			levels := newSubtreeLevels(subtreeHeight)

			for {
				if err := ctx.Err(); err != nil {
					return err
				}

				subtree := next.Add(1) - 1
				if subtree >= subtreeCount {
					return nil
				}

				tree.hashSubtree(hasher, subtree<<subtreeHeight, levels)
				roots[subtree] = levels[subtreeHeight][0]
			}
		})
	}

	if err := group.Wait(); err != nil {
		return nil, fmt.Errorf("hash subtrees: %w", err)
	}

	hasher := sha3.NewLegacyKeccak256()
	tree.upper = hashUpperLevels(hasher, roots, subtreeHeight, populatedDepth)

	tree.spine = make([][32]byte, 0, depth-populatedDepth+1)
	tree.spine = append(tree.spine, tree.upper[len(tree.upper)-1][0])
	for _, sibling := range tree.siblings {
		tree.spine = append(tree.spine, hashNode(hasher, tree.spine[len(tree.spine)-1], sibling))
	}

	return tree, nil
}

// Root returns the root of the tree, the validator's VRF root.
func (t *Tree) Root() [32]byte {
	return t.spine[len(t.spine)-1]
}

// Leaf returns the leaf of the tree at index `slot`, the first element of the proof of `slot`.
// It is cheap: it does not open the leaf.
func (t *Tree) Leaf(slot primitives.Slot) ([32]byte, error) {
	if err := t.checkSlot(slot); err != nil {
		return [32]byte{}, err
	}

	index := uint64(slot)
	if index >= 1<<t.populatedDepth {
		return [32]byte{}, nil
	}

	return hashLeaf(sha3.NewLegacyKeccak256(), t.seed, index), nil
}

// Proof returns the proof of `slot` (`VRFProof`): the leaf of the tree at index `slot`, followed by its Merkle branch.
func (t *Tree) Proof(slot primitives.Slot) ([][32]byte, error) {
	if err := t.checkSlot(slot); err != nil {
		return nil, err
	}

	index := uint64(slot)
	hasher := sha3.NewLegacyKeccak256()

	proof := make([][32]byte, 0, t.depth+1)

	if index < 1<<t.populatedDepth {
		// The leaf is derived: recompute its subtree for the leaf and the branch in the subtree.
		subtree := index >> t.subtreeHeight
		local := index & (1<<t.subtreeHeight - 1) // Index of the leaf in its subtree.

		levels := newSubtreeLevels(t.subtreeHeight)
		t.hashSubtree(hasher, subtree<<t.subtreeHeight, levels)

		proof = append(proof, levels[0][local])
		for level := range t.subtreeHeight {
			proof = append(proof, levels[level][(local>>level)^1])
		}

		// The branch in the cached upper levels of the derived part.
		for level := t.subtreeHeight; level < t.populatedDepth; level++ {
			proof = append(proof, t.upper[level-t.subtreeHeight][(index>>level)^1])
		}

		// The siblings of the derived part.
		proof = append(proof, t.siblings...)
	} else {
		// In a padded tree, the leaf and every node over it below the populated depth are zero.
		proof = append(proof, [32]byte{})
		proof = append(proof, zeroHashes[:t.populatedDepth]...)

		// Above, the only node that is not over zero leaves only is the node of index 0 of each level.
		for level := t.populatedDepth; level < t.depth; level++ {
			if (index>>level)^1 == 0 {
				proof = append(proof, t.spine[level-t.populatedDepth])
				continue
			}

			proof = append(proof, zeroHashes[level])
		}
	}

	// Never reveal an invalid proof, for instance if the cached upper levels are corrupted.
	if !isValidMerkleBranch(hasher, proof[0], proof[1:], index, t.Root()) {
		return nil, fmt.Errorf("proof of slot %d does not match the root of the tree", slot)
	}

	return proof, nil
}

// subtreeCount returns the number of subtrees of the derived part of the tree.
func (t *Tree) subtreeCount() uint64 {
	return uint64(1) << (t.populatedDepth - t.subtreeHeight)
}

// checkSlot checks that the tree can open the leaf at index `slot`.
func (t *Tree) checkSlot(slot primitives.Slot) error {
	if uint64(slot) >= 1<<t.depth {
		return fmt.Errorf("slot %d is beyond the %d leaves of the tree", slot, uint64(1)<<t.depth)
	}

	if t.full && uint64(slot) >= 1<<t.populatedDepth {
		return fmt.Errorf("slot %d is beyond the %d derived leaves of the full tree", slot, uint64(1)<<t.populatedDepth)
	}

	return nil
}

// hashSubtree fills `levels` with the subtree of `subtreeHeight` whose first leaf is at index `first`, which must be derived:
// levels[0] holds its leaves, and levels[t.subtreeHeight] holds a single node, its root.
// `levels` comes from `newSubtreeLevels`.
func (t *Tree) hashSubtree(hasher hash.Hash, first uint64, levels [][][32]byte) {
	for i := range levels[0] {
		levels[0][i] = hashLeaf(hasher, t.seed, first+uint64(i))
	}

	for level := 1; level < len(levels); level++ {
		for i := range levels[level] {
			levels[level][i] = hashNode(hasher, levels[level-1][2*i], levels[level-1][2*i+1])
		}
	}
}

// newSubtreeLevels returns the levels `hashSubtree` fills for a subtree of `height`.
func newSubtreeLevels(height uint64) [][][32]byte {
	levels := make([][][32]byte, height+1)
	for level := range levels {
		levels[level] = make([][32]byte, 1<<(height-uint64(level)))
	}

	return levels
}

// hashUpperLevels returns the levels of the tree from `bottom` to `top`, over `roots`, the nodes of level `bottom`,
// whose count is a power of two: levels[0] is `roots`, and levels[top-bottom] holds a single node.
func hashUpperLevels(hasher hash.Hash, roots [][32]byte, bottom, top uint64) [][][32]byte {
	levels := make([][][32]byte, 0, top-bottom+1)
	levels = append(levels, roots)

	for level := bottom + 1; level <= top; level++ {
		previous := levels[len(levels)-1]

		current := make([][32]byte, len(previous)/2)
		for i := range current {
			current[i] = hashNode(hasher, previous[2*i], previous[2*i+1])
		}

		levels = append(levels, current)
	}

	return levels
}

// isValidMerkleBranch checks that `branch` opens `leaf` at `index` of the tree whose root is `root` (`is_valid_vrf_proof`).
func isValidMerkleBranch(hasher hash.Hash, leaf [32]byte, branch [][32]byte, index uint64, root [32]byte) bool {
	node := leaf
	for level, sibling := range branch {
		if (index>>level)&1 == 1 {
			node = hashNode(hasher, sibling, node)
			continue
		}

		node = hashNode(hasher, node, sibling)
	}

	return node == root
}

// vrfSeed returns the VRF seed derived from `secretKey`, the serialized BLS secret key: `sha256("vrf-seed" || sk)`.
func vrfSeed(secretKey []byte) [32]byte {
	input := make([]byte, 0, len(vrfSeedDomain)+len(secretKey))
	input = append(input, vrfSeedDomain...)
	input = append(input, secretKey...)
	return sha256.Sum256(input)
}

// hashLeaf returns the derived leaf at `index`: `keccak256(seed || uint64_le(index))`. `hasher` is a Keccak-256 hasher.
func hashLeaf(hasher hash.Hash, seed [32]byte, index uint64) [32]byte {
	input := [40]byte{}
	copy(input[:32], seed[:])
	binary.LittleEndian.PutUint64(input[32:], index)
	return keccak(hasher, input[:])
}

// hashNode returns the parent of `left` and `right`: `keccak256(left || right)`. `hasher` is a Keccak-256 hasher.
func hashNode(hasher hash.Hash, left, right [32]byte) [32]byte {
	input := [64]byte{}
	copy(input[:32], left[:])
	copy(input[32:], right[:])
	return keccak(hasher, input[:])
}

// keccak returns the Keccak-256 hash of `data`, with `hasher`, a Keccak-256 hasher reused to avoid allocations.
func keccak(hasher hash.Hash, data []byte) [32]byte {
	hasher.Reset()
	hasher.Write(data) // Never returns an error.

	out := [32]byte{}
	hasher.Sum(out[:0])
	return out
}

// computeZeroHashes returns the roots of the subtrees of height 0 to `Depth` whose leaves are all zero.
func computeZeroHashes() [Depth + 1][32]byte {
	hasher := sha3.NewLegacyKeccak256()

	zeros := [Depth + 1][32]byte{}
	for level := 1; level <= Depth; level++ {
		zeros[level] = hashNode(hasher, zeros[level-1], zeros[level-1])
	}

	return zeros
}
