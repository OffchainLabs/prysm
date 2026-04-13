package optionalproofs

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
)

const blobCommitmentVersionKZG byte = 0x01

// blockData holds the fields extracted from a beacon block needed to build a NewPayloadRequest.
type blockData struct {
	ParentRoot         string
	ExecutionPayload   *structs.ExecutionPayloadDeneb
	BlobKzgCommitments []string
	ExecutionRequests  *structs.ExecutionRequests
}

// kzgCommitmentsToVersionedHashes converts KZG commitments (hex strings) to versioned hashes.
func kzgCommitmentsToVersionedHashes(commitments []string) ([][]byte, error) {
	hashes := make([][]byte, 0, len(commitments))
	for _, c := range commitments {
		decoded, err := hex.DecodeString(strings.TrimPrefix(c, "0x"))
		if err != nil {
			return nil, err
		}
		h := sha256.Sum256(decoded)
		h[0] = blobCommitmentVersionKZG
		hashes = append(hashes, h[:])
	}
	return hashes, nil
}
