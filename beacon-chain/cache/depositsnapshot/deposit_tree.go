// Package depositsnapshot implements the EIP-4881 standard for minimal sparse Merkle tree.
// The format proposed by the EIP allows for the pruning of deposits that are no longer needed to participate fully in consensus.
// Full EIP-4881 specification can be found here: https://eips.ethereum.org/EIPS/eip-4881
package depositsnapshot

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/math"
	protodb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

var (
	// ErrInvalidSnapshotRoot occurs when the snapshot root does not match the calculated root.
	ErrInvalidSnapshotRoot = errors.New("snapshot root is invalid")
	// ErrInvalidDepositCount occurs when the value for mix in length is 0.
	ErrInvalidDepositCount = errors.New("deposit count should be greater than 0")
	// ErrInvalidIndex occurs when the index is less than the number of finalized deposits.
	ErrInvalidIndex = errors.New("index should be greater than finalizedDeposits - 1")
	// ErrTooManyDeposits occurs when the number of deposits exceeds the capacity of the tree.
	ErrTooManyDeposits = errors.New("number of deposits should not be greater than the capacity of the tree")
)

// DepositTree is the Merkle tree representation of deposits.
type DepositTree struct {
	tree                    MerkleTreeNode
	depositCount            uint64 // number of deposits in the tree, reference implementation calls this mix_in_length.
	finalizedExecutionBlock executionBlock
}

type executionBlock struct {
	Hash  [32]byte
	Depth uint64
}

// NewDepositTree creates an empty deposit tree.
func NewDepositTree() *DepositTree {
	var leaves [][32]byte
	merkle := create(leaves, DepositContractDepth)
	return &DepositTree{
		tree:                    merkle,
		depositCount:            0,
		finalizedExecutionBlock: executionBlock{},
	}
}

// GetSnapshot returns a deposit tree snapshot.
func (d *DepositTree) GetSnapshot() (DepositTreeSnapshot, error) {
	var finalized [][32]byte
	depositCount, finalized := d.tree.GetFinalized(finalized)
	return fromTreeParts(finalized, depositCount, d.finalizedExecutionBlock)
}

// fromSnapshot returns a deposit tree from a deposit tree snapshot.
func fromSnapshot(snapshot DepositTreeSnapshot) (*DepositTree, error) {
	root, err := snapshot.CalculateRoot()
	if err != nil {
		return nil, err
	}
	if snapshot.depositRoot != root {
		return nil, ErrInvalidSnapshotRoot
	}
	if snapshot.depositCount >= math.PowerOf2(uint64(DepositContractDepth)) {
		return nil, ErrTooManyDeposits
	}
	tree, err := fromSnapshotParts(snapshot.finalized, snapshot.depositCount, DepositContractDepth)
	if err != nil {
		return nil, err
	}
	return &DepositTree{
		tree:                    tree,
		depositCount:            snapshot.depositCount,
		finalizedExecutionBlock: snapshot.executionBlock,
	}, nil
}

// Finalize marks a deposit as finalized.
func (d *DepositTree) Finalize(eth1DepositIndex int64, executionHash common.Hash, executionNumber uint64) error {
	var blockHash [32]byte
	copy(blockHash[:], executionHash[:])
	d.finalizedExecutionBlock = executionBlock{
		Hash:  blockHash,
		Depth: executionNumber,
	}
	depositCount := uint64(eth1DepositIndex + 1)
	_, err := d.tree.Finalize(depositCount, DepositContractDepth)
	if err != nil {
		return err
	}
	return nil
}

// getProof returns the deposit tree proof.
func (d *DepositTree) getProof(index uint64) ([32]byte, [][32]byte, error) {
	if d.depositCount <= 0 {
		return [32]byte{}, nil, ErrInvalidDepositCount
	}
	if index >= d.depositCount {
		return [32]byte{}, nil, ErrInvalidIndex
	}
	finalizedDeposits, _ := d.tree.GetFinalized([][32]byte{})
	finalizedIdx := -1
	if finalizedDeposits != 0 {
		fd, err := math.Int(finalizedDeposits)
		if err != nil {
			return [32]byte{}, nil, err
		}
		finalizedIdx = fd - 1
	}
	i, err := math.Int(index)
	if err != nil {
		return [32]byte{}, nil, err
	}
	if finalizedDeposits > 0 && i <= finalizedIdx {
		return [32]byte{}, nil, ErrInvalidIndex
	}
	leaf, proof := generateProof(d.tree, index, DepositContractDepth)
	var mixInLength [32]byte
	copy(mixInLength[:], bytesutil.Uint64ToBytesLittleEndian32(d.depositCount))
	proof = append(proof, mixInLength)
	return leaf, proof, nil
}

// getRoot returns the root of the deposit tree.
func (d *DepositTree) getRoot() [32]byte {
	var enc [32]byte
	binary.LittleEndian.PutUint64(enc[:], d.depositCount)

	root := d.tree.GetRoot()
	return hash.Hash(append(root[:], enc[:]...))
}

// pushLeaf adds a new leaf to the tree.
func (d *DepositTree) pushLeaf(leaf [32]byte) error {
	var err error
	d.tree, err = d.tree.PushLeaf(leaf, DepositContractDepth)
	if err != nil {
		return err
	}
	d.depositCount++
	return nil
}

// Insert is defined as part of MerkleTree interface and adds a new leaf to the tree.
func (d *DepositTree) Insert(item []byte, _ int) error {
	var leaf [32]byte
	copy(leaf[:], item[:32])
	return d.pushLeaf(leaf)
}

// HashTreeRoot is defined as part of MerkleTree interface and calculates the hash tree root.
func (d *DepositTree) HashTreeRoot() ([32]byte, error) {
	root := d.getRoot()
	if root == [32]byte{} {
		return [32]byte{}, errors.New("could not retrieve hash tree root")
	}
	return root, nil
}

// NumOfItems is defined as part of MerkleTree interface and returns the number of deposits in the tree.
func (d *DepositTree) NumOfItems() int {
	return int(d.depositCount)
}

// MerkleProof is defined as part of MerkleTree interface and generates a merkle proof.
func (d *DepositTree) MerkleProof(index int) ([][]byte, error) {
	_, proof, err := d.getProof(uint64(index))
	if err != nil {
		return nil, err
	}
	byteSlices := make([][]byte, len(proof))
	for i, p := range proof {
		copied := p
		byteSlices[i] = copied[:]
	}
	return byteSlices, nil
}

// Copy performs a deep copy of the tree.
func (d *DepositTree) Copy() (*DepositTree, error) {
	snapshot, err := d.GetSnapshot()
	if err != nil {
		return nil, err
	}
	return fromSnapshot(snapshot)
}

// ToProto returns a proto object of the deposit snapshot of
// the tree.
func (d *DepositTree) ToProto() (*protodb.DepositSnapshot, error) {
	snapshot, err := d.GetSnapshot()
	if err != nil {
		return nil, err
	}
	return snapshot.ToProto(), nil
}
