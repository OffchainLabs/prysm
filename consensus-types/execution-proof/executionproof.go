package executionproof

import (
	"encoding/json"
	ssz "github.com/prysmaticlabs/fastssz"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
)

// MAX_PROOF_DATA_BYTES is the maximum size of proof data in bytes.
// Note: Most proofs will fit within 300KB. Some zkVMs have 1MB proofs (currently)
// and so this number was set to accommodate for the most zkVMs.
const MAX_PROOF_DATA_BYTES = 1_048_576


// ExecutionProof represents a cryptographic `proof of execution` that
// an execution payload is valid. 
// 
// In short, it is proof that if we were to run a particular execution layer client
// with the given execution payload, they would return the output values that are attached
// to the proof.
//
// Each proof is associated with a specific subnet_id, which identifies the
// zkVM and EL combination used to generate it. Multiple proofs from different
// subnets can exist for the same execution payload, providing both client and EL diversity.
type ExecutionProof struct {
	// Which subnet/zkVM this proof belongs to
    // TODO(zkproofs): The node should provide this in themselves since they
    // know what subnet the proof came from.
    SubnetId ExecutionProofSubnetId `json:"subnet_id"`
    
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
	subnetId ExecutionProofSubnetId,
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
		SubnetId:  subnetId,
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

// IsFromSubnet checks if this proof is from a specific subnet.
func (ep *ExecutionProof) IsFromSubnet(subnetId ExecutionProofSubnetId) bool {
	return ep.SubnetId == subnetId
}

// String implements the fmt.Stringer interface
func (ep *ExecutionProof) String() string {
	return fmt.Sprintf(
		"ExecutionProof(SubnetId: %s, BlockHash: %s, BlockRoot: %s, ProofDataSize: %d)",
		ep.SubnetId,
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
	hh.PutBytes([]byte{ep.SubnetId.AsU8()})

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