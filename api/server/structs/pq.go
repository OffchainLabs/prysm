package structs

// The JSON containers of the PQ fork.
//
// A PQ container whose fields keep their name and their JSON type reuses the JSON container of the previous fork,
// with PQ conversions: `SingleAttestation`, `PayloadAttestationMessage`, `SignedProposerPreferences`,
// `ProposerSlashing` and `SignedExecutionPayloadBid`. Only its signature changes, from BLS to leanXMSS.
//
// The JSON encoding follows Lighthouse:
//   - a leanXMSS signature, a leanXMSS public key and an attestation proof are hex strings,
//   - a VRF proof, `Vector[Bytes32, VRF_TREE_DEPTH + 1]`, is a list of hex strings.

type ValidatorPQ struct {
	Pubkey                     string `json:"pubkey"`
	VrfRoot                    string `json:"vrf_root"`
	WithdrawalCredentials      string `json:"withdrawal_credentials"`
	EffectiveBalance           string `json:"effective_balance"`
	Slashed                    bool   `json:"slashed"`
	ActivationEligibilityEpoch string `json:"activation_eligibility_epoch"`
	ActivationEpoch            string `json:"activation_epoch"`
	ExitEpoch                  string `json:"exit_epoch"`
	WithdrawableEpoch          string `json:"withdrawable_epoch"`
}

type ValidatorContainerPQ struct {
	Index     string       `json:"index"`
	Balance   string       `json:"balance"`
	Status    string       `json:"status"`
	Validator *ValidatorPQ `json:"validator"`
}

type GetValidatorsResponsePQ struct {
	ExecutionOptimistic bool                    `json:"execution_optimistic"`
	Finalized           bool                    `json:"finalized"`
	Data                []*ValidatorContainerPQ `json:"data"`
}

type GetValidatorResponsePQ struct {
	ExecutionOptimistic bool                  `json:"execution_optimistic"`
	Finalized           bool                  `json:"finalized"`
	Data                *ValidatorContainerPQ `json:"data"`
}

type AttestationPQ struct {
	AggregationBits string           `json:"aggregation_bits"`
	Data            *AttestationData `json:"data"`
	CommitteeBits   string           `json:"committee_bits"`
}

type IndexedAttestationPQ struct {
	AttestingIndices []string         `json:"attesting_indices"`
	Data             *AttestationData `json:"data"`
	Proof            string           `json:"proof"`
}

type AttesterSlashingPQ struct {
	Attestation1 *IndexedAttestationPQ `json:"attestation_1"`
	Attestation2 *IndexedAttestationPQ `json:"attestation_2"`
}

type AggregateAttestationAndProofPQ struct {
	AggregatorIndex string         `json:"aggregator_index"`
	Aggregate       *AttestationPQ `json:"aggregate"`
	Proof           string         `json:"proof"`
}

type SignedAggregateAttestationAndProofPQ struct {
	Message   *AggregateAttestationAndProofPQ `json:"message"`
	Signature string                          `json:"signature"`
}

// AggregatePQ is the data of the response to `GET /eth/v2/validator/aggregate_attestation` in PQ:
// an aggregate, with the proof of the signatures of its attesters.
type AggregatePQ struct {
	Aggregate *AttestationPQ `json:"aggregate"`
	Proof     string         `json:"proof"`
}

type ValidatorPreregistrationRequest struct {
	Pubkey                string `json:"pubkey"`
	WithdrawalCredentials string `json:"withdrawal_credentials"`
	VrfRoot               string `json:"vrf_root"`
}

type BuilderPreregistrationRequest struct {
	Pubkey                string `json:"pubkey"`
	WithdrawalCredentials string `json:"withdrawal_credentials"`
}

type VoluntaryExitRequest struct {
	Pubkey string `json:"pubkey"`
}

type ExecutionRequestsPQ struct {
	Deposits                  []*DepositRequest                  `json:"deposits"`
	Withdrawals               []*WithdrawalRequest               `json:"withdrawals"`
	Consolidations            []*ConsolidationRequest            `json:"consolidations"`
	BuilderDeposits           []*BuilderDepositRequest           `json:"builder_deposits"`
	BuilderExits              []*BuilderExitRequest              `json:"builder_exits"`
	ValidatorPreregistrations []*ValidatorPreregistrationRequest `json:"validator_preregistrations"`
	BuilderPreregistrations   []*BuilderPreregistrationRequest   `json:"builder_preregistrations"`
	VoluntaryExits            []*VoluntaryExitRequest            `json:"voluntary_exits"`
}

type ExecutionPayloadEnvelopePQ struct {
	Payload               *ExecutionPayloadGloas `json:"payload"`
	ExecutionRequests     *ExecutionRequestsPQ   `json:"execution_requests"`
	BuilderIndex          string                 `json:"builder_index"`
	BeaconBlockRoot       string                 `json:"beacon_block_root"`
	ParentBeaconBlockRoot string                 `json:"parent_beacon_block_root"`
}

type SignedExecutionPayloadEnvelopePQ struct {
	Message   *ExecutionPayloadEnvelopePQ `json:"message"`
	Signature string                      `json:"signature"`
}

type BeaconBlockBodyPQ struct {
	RandaoReveal              []string                   `json:"randao_reveal"`
	Eth1Data                  *Eth1Data                  `json:"eth1_data"`
	Graffiti                  string                     `json:"graffiti"`
	ProposerSlashings         []*ProposerSlashing        `json:"proposer_slashings"`
	AttesterSlashings         []*AttesterSlashingPQ      `json:"attester_slashings"`
	Attestations              []*AttestationPQ           `json:"attestations"`
	Deposits                  []*Deposit                 `json:"deposits"`
	SignedExecutionPayloadBid *SignedExecutionPayloadBid `json:"signed_execution_payload_bid"`
	ParentExecutionRequests   *ExecutionRequestsPQ       `json:"parent_execution_requests"`
	Proof                     string                     `json:"proof"`
}

type BeaconBlockPQ struct {
	Slot          string             `json:"slot"`
	ProposerIndex string             `json:"proposer_index"`
	ParentRoot    string             `json:"parent_root"`
	StateRoot     string             `json:"state_root"`
	Body          *BeaconBlockBodyPQ `json:"body"`
}

type SignedBeaconBlockPQ struct {
	Message   *BeaconBlockPQ `json:"message"`
	Signature string         `json:"signature"`
}

// BlockContentsPQ is the data of the response to `POST /eth/v4/validator/blocks/{slot}` in PQ: the block, with the
// envelope of its self-built payload and the blobs of the payload.
type BlockContentsPQ struct {
	Block                    *BeaconBlockPQ              `json:"block"`
	ExecutionPayloadEnvelope *ExecutionPayloadEnvelopePQ `json:"execution_payload_envelope"`
	KzgProofs                []string                    `json:"kzg_proofs"`
	Blobs                    []string                    `json:"blobs"`
}

// SignedExecutionPayloadEnvelopeContentsPQ bundles a signed envelope with the blobs of its payload and their proofs.
type SignedExecutionPayloadEnvelopeContentsPQ struct {
	SignedExecutionPayloadEnvelope *SignedExecutionPayloadEnvelopePQ `json:"signed_execution_payload_envelope"`
	KzgProofs                      []string                          `json:"kzg_proofs"`
	Blobs                          []string                          `json:"blobs"`
}
