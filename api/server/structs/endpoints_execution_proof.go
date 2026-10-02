package structs

// SignedExecutionProofEnvelope is the JSON form of an EIP-8025 execution proof
// envelope and its prover signature.
type SignedExecutionProofEnvelope struct {
	Message        *ExecutionProofEnvelope `json:"message"`
	ValidatorIndex string                  `json:"validator_index"`
	Signature      string                  `json:"signature"`
}

// ExecutionProofEnvelope is the JSON form of the object a prover signs.
type ExecutionProofEnvelope struct {
	ProofData       string `json:"proof_data"`
	ProofType       string `json:"proof_type"`
	BeaconBlockRoot string `json:"beacon_block_root"`
}
