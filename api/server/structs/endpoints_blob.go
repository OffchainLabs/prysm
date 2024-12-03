package structs

type SidecarsResponse struct {
	Version             string     `json:"version"`
	Data                []*Sidecar `json:"data"`
	ExecutionOptimistic bool       `json:"execution_optimistic"`
	Finalized           bool       `json:"finalized"`
}

type Sidecar struct {
	Index                    string                   `json:"index"`
	Blob                     string                   `json:"blob"`
	SignedBeaconBlockHeader  *SignedBeaconBlockHeader `json:"signed_block_header"`
	KzgCommitment            string                   `json:"kzg_commitment"`
	KzgProof                 string                   `json:"kzg_proof"`
	CommitmentInclusionProof []string                 `json:"kzg_commitment_inclusion_proof"`
}

type BlobSidecars struct {
	Sidecars []*Sidecar `json:"sidecars"`
}

type PublishBlobsRequest struct {
	BlobSidecars *BlobSidecars `json:"blob_sidecars"`
	BlockRoot    string        `json:"block_root"`
}
