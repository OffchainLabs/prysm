package blocks

import (
	"encoding/binary"
	"errors"
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	sszutil "github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

var (
	errNilExecutionProof          = errors.New("execution proof is nil")
	errEmptyProofData             = errors.New("proof data is empty")
	errEmptyNewPayloadRequestRoot = errors.New("new payload request root is empty")
)

// ROExecutionProof represents a read-only execution proof with its block root.
type ROSignedExecutionProof struct {
	*ethpb.SignedExecutionProof
	blockRoot [fieldparams.RootLength]byte
	epoch     primitives.Epoch
}

func roSignedExecutionProofNilCheck(sep *ethpb.SignedExecutionProof) error {
	if sep == nil {
		return errNilExecutionProof
	}

	ep := sep.Message

	if len(ep.ProofData) == 0 {
		return errEmptyProofData
	}

	if len(ep.PublicInput.NewPayloadRequestRoot) == 0 {
		return errEmptyNewPayloadRequestRoot
	}

	return nil
}

// NewROSignedExecutionProofWithRoot creates a new ROSignedExecutionProof with a given root.
func NewROSignedExecutionProof(
	signedExecutionProof *ethpb.SignedExecutionProof,
	root [fieldparams.RootLength]byte,
	epoch primitives.Epoch,
) (ROSignedExecutionProof, error) {
	if err := roSignedExecutionProofNilCheck(signedExecutionProof); err != nil {
		return ROSignedExecutionProof{}, fmt.Errorf("ro signed execution proof nil check: %w", err)
	}

	roSignedExecutionProof := ROSignedExecutionProof{
		SignedExecutionProof: signedExecutionProof,
		blockRoot:            root,
		epoch:                epoch,
	}

	return roSignedExecutionProof, nil
}

// BlockRoot returns the block root of the execution proof.
func (p *ROSignedExecutionProof) BlockRoot() [fieldparams.RootLength]byte {
	return p.blockRoot
}

// Epoch returns the epoch of the execution proof.
func (p *ROSignedExecutionProof) Epoch() primitives.Epoch {
	return p.epoch
}

// // ProofType returns the proof type of the execution proof.
// func (p *ROExecutionProof) ProofType() primitives.ProofType {
// 	return p.ExecutionProof.ProofType
// }

// VerifiedROExecutionProof represents an ROExecutionProof that has undergone full verification.
type VerifiedROSignedExecutionProof struct {
	ROSignedExecutionProof
}

// NewVerifiedROExecutionProof "upgrades" an ROExecutionProof to a VerifiedROExecutionProof.
// This method should only be used by the verification package.
func NewVerifiedROSignedExecutionProof(ro ROSignedExecutionProof) VerifiedROSignedExecutionProof {
	return VerifiedROSignedExecutionProof{ROSignedExecutionProof: ro}
}

// ExecutionProofHashTreeRoot computes the correct SSZ hash tree root of an
// ExecutionProof. This replaces the fastssz-generated HashTreeRoot which
// double-merkleizes ByteList fields longer than 32 bytes (PutBytes
// internally merkleizes the chunks into a single root, then
// MerkleizeWithMixin re-merkleizes that root with the list limit).
func ExecutionProofHashTreeRoot(ep *ethpb.ExecutionProof) ([32]byte, error) {
	const maxProofDataChunks = (307200 + 31) / 32 // ceil(MAX_PROOF_SIZE / 32)

	// Field 0: proof_data — ByteList[MAX_PROOF_SIZE]
	proofDataChunks := packBytes(ep.ProofData)
	proofDataRoot, err := sszutil.BitwiseMerkleize(proofDataChunks, uint64(len(proofDataChunks)), maxProofDataChunks)
	if err != nil {
		return [32]byte{}, fmt.Errorf("merkleize proof_data: %w", err)
	}
	var lengthBuf [32]byte
	binary.LittleEndian.PutUint64(lengthBuf[:8], uint64(len(ep.ProofData)))
	field0 := sszutil.MixInLength(proofDataRoot, lengthBuf[:])

	// Field 1: proof_type — uint8 (fixed, 1 byte)
	if len(ep.ProofType) != 1 {
		return [32]byte{}, fmt.Errorf("invalid ProofType length: %d", len(ep.ProofType))
	}
	field1 := [32]byte{ep.ProofType[0]}

	// Field 2: public_input — PublicInput container (single 32-byte Root field)
	if ep.PublicInput == nil || len(ep.PublicInput.NewPayloadRequestRoot) != 32 {
		return [32]byte{}, fmt.Errorf("invalid PublicInput")
	}
	field2 := [32]byte(ep.PublicInput.NewPayloadRequestRoot)

	// Container merkleization: 3 fields, padded to next power of 2 (4).
	return sszutil.BitwiseMerkleize([][32]byte{field0, field1, field2}, 3, 4)
}

// packBytes packs a byte slice into 32-byte chunks.
func packBytes(data []byte) [][32]byte {
	if len(data) == 0 {
		return [][32]byte{}
	}
	numChunks := (len(data) + 31) / 32
	chunks := make([][32]byte, numChunks)
	for i := range chunks {
		start := i * 32
		end := min(start+32, len(data))
		copy(chunks[i][:], data[start:end])
	}
	return chunks
}
