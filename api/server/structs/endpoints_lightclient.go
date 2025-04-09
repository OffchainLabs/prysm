package structs

import "encoding/json"

type LightClientHeader struct {
	Beacon *BeaconBlockHeader `json:"beacon"`
}

type LightClientHeaderCapella struct {
	Beacon          *BeaconBlockHeader             `json:"beacon"`
	Execution       *ExecutionPayloadHeaderCapella `json:"execution"`
	ExecutionBranch []string                       `json:"execution_branch"`
}

type LightClientHeaderDeneb struct {
	Beacon          *BeaconBlockHeader           `json:"beacon"`
	Execution       *ExecutionPayloadHeaderDeneb `json:"execution"`
	ExecutionBranch []string                     `json:"execution_branch"`
}

type LightClientBootstrap struct {
	Header                     json.RawMessage `json:"header"`
	CurrentSyncCommittee       *SyncCommittee  `json:"current_sync_committee"`
	CurrentSyncCommitteeBranch []string        `json:"current_sync_committee_branch"`
}

type LightClientUpdate struct {
	AttestedHeader          json.RawMessage `json:"attested_header"`
	NextSyncCommittee       *SyncCommittee  `json:"next_sync_committee,omitempty"`
	FinalizedHeader         json.RawMessage `json:"finalized_header,omitempty"`
	SyncAggregate           *SyncAggregate  `json:"sync_aggregate"`
	NextSyncCommitteeBranch []string        `json:"next_sync_committee_branch,omitempty"`
	FinalityBranch          []string        `json:"finality_branch,omitempty"`
	SignatureSlot           string          `json:"signature_slot"`
}

type LightClientFinalityUpdate struct {
	AttestedHeader  json.RawMessage `json:"attested_header"`
	FinalizedHeader json.RawMessage `json:"finalized_header"`
	FinalityBranch  []string        `json:"finality_branch"`
	SyncAggregate   *SyncAggregate  `json:"sync_aggregate"`
	SignatureSlot   string          `json:"signature_slot"`
}

type LightClientOptimisticUpdate struct {
	AttestedHeader json.RawMessage `json:"attested_header"`
	SyncAggregate  *SyncAggregate  `json:"sync_aggregate"`
	SignatureSlot  string          `json:"signature_slot"`
}

type LightClientBootstrapResponse struct {
	Version string                `json:"version"`
	Data    *LightClientBootstrap `json:"data"`
}

type LightClientUpdateResponse struct {
	Version string             `json:"version"`
	Data    *LightClientUpdate `json:"data"`
}

type LightClientFinalityUpdateResponse struct {
	Version string                     `json:"version"`
	Data    *LightClientFinalityUpdate `json:"data"`
}

type LightClientOptimisticUpdateResponse struct {
	Version string                       `json:"version"`
	Data    *LightClientOptimisticUpdate `json:"data"`
}

type LightClientUpdatesByRangeResponse struct {
	Updates []*LightClientUpdateResponse `json:"updates"`
}
