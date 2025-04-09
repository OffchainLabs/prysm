package depositsnapshot

import (
	"github.com/prysmaticlabs/prysm/v5/container/trie"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	protodb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// DepositTreeSnapshot represents the data used to create a deposit tree given a snapshot.
type DepositTreeSnapshot struct {
	finalized      [][32]byte
	depositRoot    [32]byte
	depositCount   uint64
	executionBlock executionBlock
}

// CalculateRoot returns the root of a deposit tree snapshot.
func (ds *DepositTreeSnapshot) CalculateRoot() ([32]byte, error) {
	size := ds.depositCount
	index := len(ds.finalized)
	root := trie.ZeroHashes[0]
	for i := 0; i < DepositContractDepth; i++ {
		if (size & 1) == 1 {
			if index == 0 {
				break
			}
			index--
			root = hash.Hash(append(ds.finalized[index][:], root[:]...))
		} else {
			root = hash.Hash(append(root[:], trie.ZeroHashes[i][:]...))
		}
		size >>= 1
	}
	return hash.Hash(append(root[:], bytesutil.Uint64ToBytesLittleEndian32(ds.depositCount)...)), nil
}

// fromTreeParts constructs the deposit tree from pre-existing data.
func fromTreeParts(finalised [][32]byte, depositCount uint64, executionBlock executionBlock) (DepositTreeSnapshot, error) {
	snapshot := DepositTreeSnapshot{
		finalized:      finalised,
		depositRoot:    trie.ZeroHashes[0],
		depositCount:   depositCount,
		executionBlock: executionBlock,
	}
	root, err := snapshot.CalculateRoot()
	if err != nil {
		return snapshot, ErrInvalidSnapshotRoot
	}
	snapshot.depositRoot = root
	return snapshot, nil
}

// ToProto converts the underlying trie into its corresponding proto object.
func (ds *DepositTreeSnapshot) ToProto() *protodb.DepositSnapshot {
	tree := &protodb.DepositSnapshot{
		Finalized:      make([][]byte, len(ds.finalized)),
		DepositRoot:    bytesutil.SafeCopyBytes(ds.depositRoot[:]),
		DepositCount:   ds.depositCount,
		ExecutionHash:  bytesutil.SafeCopyBytes(ds.executionBlock.Hash[:]),
		ExecutionDepth: ds.executionBlock.Depth,
	}
	for i := range ds.finalized {
		tree.Finalized[i] = bytesutil.SafeCopyBytes(ds.finalized[i][:])
	}
	return tree
}

// DepositTreeFromSnapshotProto generates a deposit tree object from a provided snapshot.
func DepositTreeFromSnapshotProto(snapshotProto *protodb.DepositSnapshot) (*DepositTree, error) {
	finalized := make([][32]byte, len(snapshotProto.Finalized))
	for i := range snapshotProto.Finalized {
		finalized[i] = bytesutil.ToBytes32(snapshotProto.Finalized[i])
	}
	snapshot := DepositTreeSnapshot{
		finalized:    finalized,
		depositRoot:  bytesutil.ToBytes32(snapshotProto.DepositRoot),
		depositCount: snapshotProto.DepositCount,
		executionBlock: executionBlock{
			Hash:  bytesutil.ToBytes32(snapshotProto.ExecutionHash),
			Depth: snapshotProto.ExecutionDepth,
		},
	}
	return fromSnapshot(snapshot)
}
