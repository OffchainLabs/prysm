package executionproof

import (
	"encoding/json"
	ssz "github.com/prysmaticlabs/fastssz"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

// MAX_PROOF_DATA_BYTES is the maximum size of proof data in bytes.
// Note: Most proofs will fit within 300KB. Some zkVMs have 1MB proofs (currently)
// and so this number was set to accommodate for the most zkVMs.
const MAX_PROOF_DATA_BYTES = 1_048_576

// Minimum number of execution proofs required from different proof types
// before marking an execution payload as available in ZK-VM mode.
//
// This provides client diversity - nodes wait for proofs from K different
// zkVM+EL combinations before considering an execution payload available.
const DEFAULT_MIN_PROOFS_REQUIRED = 2

// Maximum number of execution proofs that can be requested or stored.
// This corresponds to the maximum number of proof types (zkVM+EL combinations)
// that can be supported, which is currently 8 (ExecutionProofId is 0-7).
const MAX_PROOFS = 8

// ExecutionProof represents a cryptographic `proof of execution` that
// an execution payload is valid. 
// 
// In short, it is proof that if we were to run a particular execution layer client
// with the given execution payload, they would return the output values that are attached
// to the proof.
//
// Each proof is associated with a specific proof_id, which identifies the
// zkVM and EL combination used to generate it. Multiple proofs from different
// subnets can exist for the same execution payload, providing both client and EL diversity.
type ExecutionProof struct {
	// Which proof this proof belongs to
    // TODO(zkproofs): The node should provide this in themselves since they
    // know what proof the proof came from.
    ProofId ExecutionProofId `json:"proof_id"`
    
    // The slot of the beacon block this proof validates
    Slot primitives.Slot `json:"slot"`
    
    // The block hash of the execution payload this proof validates
    BlockHash common.Hash `json:"block_hash"`
    
    // The beacon block root corresponding to the beacon block
    // with the execution payload, that this proof attests to.
    BlockRoot common.Hash `json:"block_root"`
    
    // The actual proof data 
    ProofData []byte `json:"proof_data"`
}

// NewExecutionProof creates a new ExecutionProof, validating the proof data size.
func NewExecutionProof(
	proofId ExecutionProofId,
	slot primitives.Slot,
	blockHash common.Hash,
	blockRoot common.Hash,
	proofData []byte,
) (*ExecutionProof, error) {
	if len(proofData) > MAX_PROOF_DATA_BYTES {
		return nil, fmt.Errorf(
			"Proof data too large: %d bytes, max is %d",
			len(proofData), MAX_PROOF_DATA_BYTES,
		)
	}

	return &ExecutionProof{
		ProofId:   proofId,
		Slot:      slot,
		BlockHash: blockHash,
		BlockRoot: blockRoot,
		ProofData: proofData,
	}, nil
}

// ProofDataSize returns the size of the proof data in bytes.
func (ep *ExecutionProof) ProofDataSize() int {
	return len(ep.ProofData)
}

// ProofDataSlice returns a reference to the proof data as a slice.
func (ep *ExecutionProof) ProofDataSlice() []byte {
	return ep.ProofData
}

// IsForBlock checks if this proof is for a specific execution block hash.
func (ep *ExecutionProof) IsForBlock(blockHash *common.Hash) bool {
	return ep.BlockHash == *blockHash
}

// Check if this proof is from a specific proof type
func (ep *ExecutionProof) IsFromProofType(proofId ExecutionProofId) bool {
	return ep.ProofId == proofId
}

// Get the proof type ID
func (ep *ExecutionProof) GetProofTypeId() ExecutionProofId {
	return ep.ProofId
}

// Minimum size of an ExecutionProof in SSZ bytes (with empty proof_data)
func (ep *ExecutionProof) MinimumSize() int {
	zeroProof := &ExecutionProof{
		ProofId:   0,
		Slot:      0,
		BlockHash: common.Hash{},
		BlockRoot: common.Hash{},
		ProofData: []byte{},
	}
	b, err := zeroProof.MarshalSSZ()
	if err != nil {
		return 0
	}
	return len(b)
}

// Maximum size of an ExecutionProof in SSZ bytes (with max proof_data)
func (ep *ExecutionProof) MaximumSize() int {
	maxProofData := make([]byte, MAX_PROOF_DATA_BYTES)
	maxProof := &ExecutionProof{
		ProofId:   0,
		Slot:      0,
		BlockHash: common.Hash{},
		BlockRoot: common.Hash{},
		ProofData: maxProofData,
	}
	b, err := maxProof.MarshalSSZ()
	if err != nil {
		return 0
	}
	return len(b)
}

// String implements the fmt.Stringer interface
func (ep *ExecutionProof) String() string {
	return fmt.Sprintf(
		"ExecutionProof(ProofId: %s, BlockHash: %s, BlockRoot: %s, ProofDataSize: %d)",
		ep.ProofId,
		ep.BlockHash,
		ep.BlockRoot,
		len(ep.ProofData),
	)
}

// MarshalJSON implements the json.Marshaler interface.
func (ep *ExecutionProof) MarshalJSON() ([]byte, error) {
	// Use a temporary struct to handle []byte as hex string
	type Alias ExecutionProof
	return json.Marshal(&struct {
		*Alias
		ProofData string `json:"proof_data"`
	}{
		Alias:     (*Alias)(ep),
		ProofData: fmt.Sprintf("0x%x", ep.ProofData),
	})
}

// MarshalSSZ implements the ExecutionProof object
func (ep *ExecutionProof) MarshalSSZ() ([]byte, error) {
	panic("TODO: method not implemented")
}

// MarshalSSZTo implements the ExecutionProof object
func (ep *ExecutionProof) MarshalSSZTo(buf []byte) (dst []byte, err error) {
	panic("TODO: method not implemented")
}

// UnmarshalSSZ implements the ExecutionProof object
func (ep *ExecutionProof) UnmarshalSSZ(buf []byte) error {
	panic("TODO: method not implemented")
}

// SizeSSZ implements the ExecutionProof object
func (ep *ExecutionProof) SizeSSZ() (size int) {
	panic("TODO: method not implemented")
}

// HashTreeRootWith ssz hashes the ExecutionPayload object with a hasher
func (ep *ExecutionProof) HashTreeRootWith(hh *ssz.Hasher) (err error) {
	indx := hh.Index()

	// Field (0) 'SubnetId'
	hh.PutBytes([]byte{ep.ProofId.AsU8()})

	// Field (1) 'BlockHash'
	if size := len(ep.BlockHash); size != 32 {
		err = ssz.ErrBytesLengthFn("--.BlockHash", size, 32)
		return
	}
	hh.PutBytes(ep.BlockHash.Bytes())

	// Field (2) 'BlockRoot'
	if size := len(ep.BlockRoot); size != 32 {
		err = ssz.ErrBytesLengthFn("--.BlockRoot", size, 32)
		return
	}
	hh.PutBytes(ep.BlockRoot.Bytes())

	// Field (3) 'ReceiptsRoot'
	hh.PutBytes(ep.ProofData)


	hh.Merkleize(indx)
	return
}