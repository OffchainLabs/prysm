package structs

import "encoding/json"

type BeaconBlockBodyHeze struct {
	RandaoReveal              string                        `json:"randao_reveal"`
	Eth1Data                  *Eth1Data                     `json:"eth1_data"`
	Graffiti                  string                        `json:"graffiti"`
	ProposerSlashings         []*ProposerSlashing           `json:"proposer_slashings"`
	AttesterSlashings         []*AttesterSlashingElectra    `json:"attester_slashings"`
	Attestations              []*AttestationElectra         `json:"attestations"`
	Deposits                  []*Deposit                    `json:"deposits"`
	VoluntaryExits            []*SignedVoluntaryExit        `json:"voluntary_exits"`
	SyncAggregate             *SyncAggregate                `json:"sync_aggregate"`
	BLSToExecutionChanges     []*SignedBLSToExecutionChange `json:"bls_to_execution_changes"`
	SignedExecutionPayloadBid *SignedExecutionPayloadBid    `json:"signed_execution_payload_bid"`
	PayloadAttestations       []*PayloadAttestation         `json:"payload_attestations"`
	ParentExecutionRequests   *ExecutionRequestsGloas       `json:"parent_execution_requests"`
}

type BeaconBlockHeze struct {
	Slot          string               `json:"slot"`
	ProposerIndex string               `json:"proposer_index"`
	ParentRoot    string               `json:"parent_root"`
	StateRoot     string               `json:"state_root"`
	Body          *BeaconBlockBodyHeze `json:"body"`
}

type SignedBeaconBlockHeze struct {
	Message   *BeaconBlockHeze `json:"message"`
	Signature string           `json:"signature"`
}

var _ SignedMessageJsoner = &SignedBeaconBlockHeze{}

func (s *SignedBeaconBlockHeze) MessageRawJson() ([]byte, error) {
	return json.Marshal(s.Message)
}

func (s *SignedBeaconBlockHeze) SigString() string {
	return s.Signature
}

// BeaconStateHeze matches BeaconStateGloas until the Heze state diverges.
type BeaconStateHeze BeaconStateGloas

type BlockContentsHeze struct {
	Block                    *BeaconBlockHeze          `json:"block"`
	ExecutionPayloadEnvelope *ExecutionPayloadEnvelope `json:"execution_payload_envelope"`
	KzgProofs                []string                  `json:"kzg_proofs"`
	Blobs                    []string                  `json:"blobs"`
}
