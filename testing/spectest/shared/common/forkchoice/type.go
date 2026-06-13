package forkchoice

type Step struct {
	Tick                      *int            `json:"tick"`
	Block                     *string         `json:"block"`
	Blobs                     *string         `json:"blobs"`
	Proofs                    []*string       `json:"proofs"`
	Valid                     *bool           `json:"valid"`
	Attestation               *string         `json:"attestation"`
	AttesterSlashing          *string         `json:"attester_slashing"`
	PayloadStatus             *MockEngineResp `json:"payload_status"`
	PowBlock                  *string         `json:"pow_block"`
	Check                     *Check          `json:"checks"`
	DataColumns               []*string       `json:"columns"`
	ExecutionPayload          *string         `json:"execution_payload"`
	PayloadAttestationMessage *string         `json:"payload_attestation_message"`
}

type Check struct {
	Time                        *int            `json:"time"`
	GenesisTime                 int             `json:"genesis_time"`
	ProposerBoostRoot           *string         `json:"proposer_boost_root"`
	Head                        *SlotRoot       `json:"head"`
	JustifiedCheckPoint         *EpochRoot      `json:"justified_checkpoint"`
	BestJustifiedCheckPoint     *EpochRoot      `json:"best_justified_checkpoint"`
	FinalizedCheckPoint         *EpochRoot      `json:"finalized_checkpoint"`
	GetProposerHead             *string         `json:"get_proposer_head"`
	ShouldOverrideFCU           *ShouldOverride `json:"should_override_forkchoice_update"`
	HeadPayloadStatus           *int            `json:"head_payload_status"`
	PayloadTimelinessVote       *PTCVotes       `json:"payload_timeliness_vote"`
	PayloadDataAvailabilityVote *PTCVotes       `json:"payload_data_availability_vote"`
}

type PTCVotes struct {
	BlockRoot string  `json:"block_root"`
	Votes     []*bool `json:"votes"`
}

type SlotRoot struct {
	Slot int    `json:"slot"`
	Root string `json:"root"`
}

type EpochRoot struct {
	Epoch int    `json:"epoch"`
	Root  string `json:"root"`
}

type MockEngineResp struct {
	Status          *string `json:"status"`
	LatestValidHash *string `json:"latest_valid_hash"`
	ValidationError *string `json:"validation_error"`
}

type ShouldOverride struct {
	ValidatorConnected bool `json:"validator_is_connected"`
	Result             bool `json:"result"`
}
