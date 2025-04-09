package depositsnapshot

import (
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/container/slice"
	"github.com/prysmaticlabs/prysm/v5/container/trie"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	"github.com/prysmaticlabs/prysm/v5/math"
)

const (
	// DepositContractDepth is the maximum tree depth as defined by EIP-4881.
	DepositContractDepth = 32
)

var (
	// ErrFinalizedNodeCannotPushLeaf may occur when attempting to push a leaf to a finalized node. When a node is finalized, it cannot be modified or changed.
	ErrFinalizedNodeCannotPushLeaf = errors.New("can't push a leaf to a finalized node")
	// ErrLeafNodeCannotPushLeaf may occur when attempting to push a leaf to a leaf node.
	ErrLeafNodeCannotPushLeaf = errors.New("can't push a leaf to a leaf node")
	// ErrZeroLevel occurs when the value of level is 0.
	ErrZeroLevel = errors.New("level should be greater than 0")
	// ErrZeroDepth occurs when the value of depth is 0.
	ErrZeroDepth = errors.New("depth should be greater than 0")
)

// MerkleTreeNode is the interface for a Merkle tree.
type MerkleTreeNode interface {
	// GetRoot returns the root of the Merkle tree.
	GetRoot() [32]byte
	// IsFull returns whether there is space left for deposits.
	IsFull() bool
	// Finalize marks deposits of the Merkle tree as finalized.
	Finalize(depositsToFinalize uint64, depth uint64) (MerkleTreeNode, error)
	// GetFinalized returns the number of deposits and a list of hashes of all the finalized nodes.
	GetFinalized(result [][32]byte) (uint64, [][32]byte)
	// PushLeaf adds a new leaf node at the next available Zero node.
	PushLeaf(leaf [32]byte, depth uint64) (MerkleTreeNode, error)

	// Right represents the right child of a node.
	Right() MerkleTreeNode
	// Left represents the left child of a node.
	Left() MerkleTreeNode
}

// create builds a new merkle tree
func create(leaves [][32]byte, depth uint64) MerkleTreeNode {
	length := uint64(len(leaves))
	if length == 0 {
		return &ZeroNode{depth: depth}
	}
	if depth == 0 {
		return &LeafNode{hash: leaves[0]}
	}
	split := math.Min(math.PowerOf2(depth-1), length)
	left := create(leaves[0:split], depth-1)
	right := create(leaves[split:], depth-1)
	return &InnerNode{left: left, right: right}
}

// fromSnapshotParts creates a new Merkle tree from a list of finalized leaves, number of deposits and specified depth.
func fromSnapshotParts(finalized [][32]byte, deposits uint64, level uint64) (MerkleTreeNode, error) {
	var err error
	if len(finalized) < 1 || deposits == 0 {
		return &ZeroNode{
			depth: level,
		}, nil
	}
	if deposits == math.PowerOf2(level) {
		return &FinalizedNode{
			depositCount: deposits,
			hash:         finalized[0],
		}, nil
	}
	if level == 0 {
		return &ZeroNode{}, ErrZeroLevel
	}
	node := InnerNode{}
	if leftSubtree := math.PowerOf2(level - 1); deposits <= leftSubtree {
		node.left, err = fromSnapshotParts(finalized, deposits, level-1)
		if err != nil {
			return &ZeroNode{}, err
		}
		node.right = &ZeroNode{depth: level - 1}
	} else {
		node.left = &FinalizedNode{
			depositCount: leftSubtree,
			hash:         finalized[0],
		}
		node.right, err = fromSnapshotParts(finalized[1:], deposits-leftSubtree, level-1)
		if err != nil {
			return &ZeroNode{}, err
		}
	}
	return &node, nil
}

// generateProof returns a merkle proof and root
func generateProof(tree MerkleTreeNode, index uint64, depth uint64) ([32]byte, [][32]byte) {
	var proof [][32]byte
	node := tree
	for depth > 0 {
		ithBit := (index >> (depth - 1)) & 0x1
		if ithBit == 1 {
			proof = append(proof, node.Left().GetRoot())
			node = node.Right()
		} else {
			proof = append(proof, node.Right().GetRoot())
			node = node.Left()
		}
		depth--
	}
	proof = slice.Reverse(proof)
	return node.GetRoot(), proof
}

// FinalizedNode represents a finalized node and satisfies the MerkleTreeNode interface.
type FinalizedNode struct {
	depositCount uint64
	hash         [32]byte
}

// GetRoot returns the root of the Merkle tree.
func (f *FinalizedNode) GetRoot() [32]byte {
	return f.hash
}

// IsFull returns whether there is space left for deposits.
// A FinalizedNode will always return true as by definition it
// is full and deposits can't be added to it.
func (_ *FinalizedNode) IsFull() bool {
	return true
}

// Finalize marks deposits of the Merkle tree as finalized.
func (f *FinalizedNode) Finalize(depositsToFinalize uint64, depth uint64) (MerkleTreeNode, error) {
	return f, nil
}

// GetFinalized returns a list of hashes of all the finalized nodes and the number of deposits.
func (f *FinalizedNode) GetFinalized(result [][32]byte) (uint64, [][32]byte) {
	return f.depositCount, append(result, f.hash)
}

// PushLeaf adds a new leaf node at the next available zero node.
func (_ *FinalizedNode) PushLeaf(_ [32]byte, _ uint64) (MerkleTreeNode, error) {
	return nil, ErrFinalizedNodeCannotPushLeaf
}

// Right returns nil as a finalized node can't have any children.
func (_ *FinalizedNode) Right() MerkleTreeNode {
	return nil
}

// Left returns nil as a finalized node can't have any children.
func (_ *FinalizedNode) Left() MerkleTreeNode {
	return nil
}

// LeafNode represents a leaf node holding a deposit and satisfies the MerkleTreeNode interface.
type LeafNode struct {
	hash [32]byte
}

// GetRoot returns the root of the Merkle tree.
func (l *LeafNode) GetRoot() [32]byte {
	return l.hash
}

// IsFull returns whether there is space left for deposits.
// A LeafNode will always return true as it is the last node
// in the tree and therefore can't have any deposits added to it.
func (_ *LeafNode) IsFull() bool {
	return true
}

// Finalize marks deposits of the Merkle tree as finalized.
func (l *LeafNode) Finalize(depositsToFinalize uint64, depth uint64) (MerkleTreeNode, error) {
	return &FinalizedNode{1, l.hash}, nil
}

// GetFinalized returns a list of hashes of all the finalized nodes and the number of deposits.
func (_ *LeafNode) GetFinalized(result [][32]byte) (uint64, [][32]byte) {
	return 0, result
}

// PushLeaf adds a new leaf node at the next available zero node.
func (_ *LeafNode) PushLeaf(_ [32]byte, _ uint64) (MerkleTreeNode, error) {
	return nil, ErrLeafNodeCannotPushLeaf
}

// Right returns nil as a leaf node is the last node and can't have any children.
func (_ *LeafNode) Right() MerkleTreeNode {
	return nil
}

// Left returns nil as a leaf node is the last node and can't have any children.
func (_ *LeafNode) Left() MerkleTreeNode {
	return nil
}

// InnerNode represents an inner node with two children and satisfies the MerkleTreeNode interface.
type InnerNode struct {
	left, right MerkleTreeNode
}

// GetRoot returns the root of the Merkle tree.
func (n *InnerNode) GetRoot() [32]byte {
	left := n.left.GetRoot()
	right := n.right.GetRoot()
	return hash.Hash(append(left[:], right[:]...))
}

// IsFull returns whether there is space left for deposits.
func (n *InnerNode) IsFull() bool {
	return n.right.IsFull()
}

// Finalize marks deposits of the Merkle tree as finalized.
func (n *InnerNode) Finalize(depositsToFinalize uint64, depth uint64) (MerkleTreeNode, error) {
	var err error
	deposits := math.PowerOf2(depth)
	if deposits <= depositsToFinalize {
		return &FinalizedNode{deposits, n.GetRoot()}, nil
	}
	if depth == 0 {
		return &ZeroNode{}, ErrZeroDepth
	}
	n.left, err = n.left.Finalize(depositsToFinalize, depth-1)
	if err != nil {
		return &ZeroNode{}, err
	}
	if depositsToFinalize > deposits/2 {
		remaining := depositsToFinalize - deposits/2
		n.right, err = n.right.Finalize(remaining, depth-1)
		if err != nil {
			return &ZeroNode{}, err
		}
	}
	return n, nil
}

// GetFinalized returns a list of hashes of all the finalized nodes and the number of deposits.
func (n *InnerNode) GetFinalized(result [][32]byte) (uint64, [][32]byte) {
	leftDeposits, result := n.left.GetFinalized(result)
	rightDeposits, result := n.right.GetFinalized(result)
	return leftDeposits + rightDeposits, result
}

// PushLeaf adds a new leaf node at the next available zero node.
func (n *InnerNode) PushLeaf(leaf [32]byte, depth uint64) (MerkleTreeNode, error) {
	if !n.left.IsFull() {
		left, err := n.left.PushLeaf(leaf, depth-1)
		if err == nil {
			n.left = left
		} else {
			return n, err
		}
	} else {
		right, err := n.right.PushLeaf(leaf, depth-1)
		if err == nil {
			n.right = right
		} else {
			return n, err
		}
	}
	return n, nil
}

// Right returns the child node on the right.
func (n *InnerNode) Right() MerkleTreeNode {
	return n.right
}

// Left returns the child node on the left.
func (n *InnerNode) Left() MerkleTreeNode {
	return n.left
}

// ZeroNode represents an empty node without a deposit and satisfies the MerkleTreeNode interface.
type ZeroNode struct {
	depth uint64
}

// GetRoot returns the root of the Merkle tree.
func (z *ZeroNode) GetRoot() [32]byte {
	if z.depth == DepositContractDepth {
		return hash.Hash(append(trie.ZeroHashes[z.depth-1][:], trie.ZeroHashes[z.depth-1][:]...))
	}
	return trie.ZeroHashes[z.depth]
}

// IsFull returns wh   ether there is space left for deposits.
// A ZeroNode will always return false as a ZeroNode is an empty node
// that gets replaced by a deposit.
func (_ *ZeroNode) IsFull() bool {
	return false
}

// Finalize marks deposits of the Merkle tree as finalized.
func (_ *ZeroNode) Finalize(depositsToFinalize uint64, depth uint64) (MerkleTreeNode, error) {
	return &ZeroNode{}, nil
}

// GetFinalized returns a list of hashes of all the finalized nodes and the number of deposits.
func (_ *ZeroNode) GetFinalized(result [][32]byte) (uint64, [][32]byte) {
	return 0, result
}

// PushLeaf adds a new leaf node at the next available zero node.
func (_ *ZeroNode) PushLeaf(leaf [32]byte, depth uint64) (MerkleTreeNode, error) {
	return create([][32]byte{leaf}, depth), nil
}

// Right returns nil as a zero node can't have any children.
func (_ *ZeroNode) Right() MerkleTreeNode {
	return nil
}

// Left returns nil as a zero node can't have any children.
func (_ *ZeroNode) Left() MerkleTreeNode {
	return nil
}
