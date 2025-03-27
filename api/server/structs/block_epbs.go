package structs

import "encoding/json"

// ----------------------------------------------------------------------------
// Epbs
// ----------------------------------------------------------------------------
type SignedBeaconBlockEpbs struct {
	Message   *BeaconBlockEpbs `json:"message"`
	Signature string           `json:"signature"`
}

var _ SignedMessageJsoner = &SignedBeaconBlockElectra{}

func (s *SignedBeaconBlockEpbs) MessageRawJson() ([]byte, error) {
	return json.Marshal(s.Message)
}

func (s *SignedBeaconBlockEpbs) SigString() string {
	return s.Signature
}

type BeaconBlockEpbs struct {
	Slot          string               `json:"slot"`
	ProposerIndex string               `json:"proposer_index"`
	ParentRoot    string               `json:"parent_root"`
	StateRoot     string               `json:"state_root"`
	Body          *BeaconBlockBodyEpbs `json:"body"`
}

type BeaconBlockBodyEpbs struct {
	RandaoReveal                 string                        `json:"randao_reveal"`
	Eth1Data                     *Eth1Data                     `json:"eth1_data"`
	Graffiti                     string                        `json:"graffiti"`
	ProposerSlashings            []*ProposerSlashing           `json:"proposer_slashings"`
	AttesterSlashings            []*AttesterSlashingElectra    `json:"attester_slashings"`
	Attestations                 []*AttestationElectra         `json:"attestations"`
	Deposits                     []*Deposit                    `json:"deposits"`
	VoluntaryExits               []*SignedVoluntaryExit        `json:"voluntary_exits"`
	SyncAggregate                *SyncAggregate                `json:"sync_aggregate"`
	BLSToExecutionChanges        []*SignedBLSToExecutionChange `json:"bls_to_execution_changes"`
	SignedExecutionPayloadHeader *SignedExecutionPayloadHeader `json:"signed_execution_payload_header"`
	PayloadAttestations          []*PayloadAttestation         `json:"payload_attestations"`
}

type SignedExecutionPayloadEnvelope struct {
	Message   *ExecutionPayloadEnvelope `json:"message"`
	Signature string                    `json:"signature"`
}

type ExecutionPayloadEnvelope struct {
	Payload            *ExecutionPayloadDeneb `json:"payload"`
	ExecutionRequests  *ExecutionRequests     `json:"execution_requests"`
	BuilderIndex       string                 `json:"builder_index"`
	BeaconBlockRoot    string                 `json:"beacon_block_root"`
	Slot               string                 `json:"slot"`
	BlobKzgCommitments []string               `json:"blob_kzg_commitments"`
	StateRoot          string                 `json:"state_root"`
}
type SignedExecutionPayloadHeader struct {
	Message   *ExecutionPayloadHeaderEPBS `json:"message"`
	Signature string                      `json:"signature"`
}

type ExecutionPayloadHeaderEPBS struct {
	ParentBlockHash        string `json:"parent_block_hash"`
	ParentBlockRoot        string `json:"parent_block_root"`
	BlockHash              string `json:"block_hash"`
	GasLimit               string `json:"gas_limit"`
	BuilderIndex           string `json:"builder_index"`
	Slot                   string `json:"slot"`
	Value                  string `json:"value"`
	BlobKzgCommitmentsRoot string `json:"blob_kzg_commitments_root"`
}

type PayloadAttestation struct {
	AggregationBits string                  `json:"aggregation_bits"`
	Data            *PayloadAttestationData `json:"data"`
	Signature       string                  `json:"signature"`
}

type PayloadAttestationData struct {
	BeaconBlockRoot string `json:"beacon_block_root"`
	Slot            string `json:"slot"`
	PayloadStatus   string `json:"payload_status"`
}

type GetExecutionPayloadV1Response struct {
	Version             string                          `json:"version"`
	ExecutionOptimistic bool                            `json:"execution_optimistic"`
	Finalized           bool                            `json:"finalized"`
	Data                *SignedExecutionPayloadEnvelope `json:"data"`
}
