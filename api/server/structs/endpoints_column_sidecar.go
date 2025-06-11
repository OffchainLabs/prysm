package structs

// DataColumnSidecar represents a sidecar containing data columns for a specific index.
type DataColumnSidecar struct {
	Index                        string                   `json:"index"`
	Column                       []string                 `json:"column"`
	KZGCommitments               []string                 `json:"kzg_commitments"`
	KZGProofs                    []string                 `json:"kzg_proofs"`
	SignedBlockHeader            *SignedBeaconBlockHeader `json:"signed_block_header"`
	KZGCommitmentsInclusionProof []string                 `json:"kzg_commitments_inclusion_proof"`
}

// DataColumnSidecarResponse represents the response structure for data column sidecars for beacon api endpoints.
type DataColumnSidecarResponse struct {
	Version             string               `json:"version"`
	Data                []*DataColumnSidecar `json:"data"`
	ExecutionOptimistic bool                 `json:"execution_optimistic"`
	Finalized           bool                 `json:"finalized"`
}
