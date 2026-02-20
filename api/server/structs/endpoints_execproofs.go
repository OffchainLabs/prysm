package structs

// SignedExecutionProofResponse is the REST API response for a signed execution proof.
type SignedExecutionProofResponse struct {
	Data *SignedExecutionProof `json:"data"`
}

// SignedExecutionProof represents a signed execution proof.
type SignedExecutionProof struct {
	Message        *ExecutionProof `json:"message"`
	ValidatorIndex uint64          `json:"validator_index"`
	Signature      []byte          `json:"signature"`
}

// ExecutionProofRequest is the REST API request for signing an execution proof.
type ExecutionProofRequest struct {
	Data *ExecutionProof `json:"data"`
}

// ExecutionProof represents an execution proof.
type ExecutionProof struct {
	ProofData   []byte       `json:"proof_data"`
	ProofType   uint8        `json:"proof_type"`
	PublicInput *PublicInput `json:"public_input"`
}

// PublicInput represents the public input for an execution proof.
type PublicInput struct {
	NewPayloadRequestRoot []byte `json:"new_payload_request_root"`
}
