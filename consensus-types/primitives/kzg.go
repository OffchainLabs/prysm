package primitives

import (
	"crypto/sha256"

	"github.com/ethereum/go-ethereum/common"
)

const blobCommitmentVersionKZG uint8 = 0x01

func ConvertKzgCommitmentToVersionedHash(commitment []byte) common.Hash {
	versionedHash := sha256.Sum256(commitment)
	versionedHash[0] = blobCommitmentVersionKZG
	return versionedHash
}
