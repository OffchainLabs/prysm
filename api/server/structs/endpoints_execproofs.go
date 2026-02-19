package structs

type SignedExecutionProof struct {
	Message        *ExecutionProof `json:"message"`
	ValidatorIndex uint64          `json:"validator_index"`
	Signature      []byte          `json:"signature"`
}

type ExecutionProof struct {
	ProofData   []byte       `json:"proof_data"`
	ProofType   uint8        `json:"proof_type"`
	PublicInput *PublicInput `json:"public_input"`
}

type PublicInput struct {
	NewPayloadRequestRoot []byte `json:"new_payload_request_root"`
}
