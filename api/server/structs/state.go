package structs

type BeaconState struct {
	GenesisTime                 string                `json:"genesis_time"`
	GenesisValidatorsRoot       string                `json:"genesis_validators_root"`
	Slot                        string                `json:"slot"`
	Fork                        *Fork                 `json:"fork"`
	LatestBlockHeader           *BeaconBlockHeader    `json:"latest_block_header"`
	BlockRoots                  []string              `json:"block_roots"`
	StateRoots                  []string              `json:"state_roots"`
	HistoricalRoots             []string              `json:"historical_roots"`
	Eth1Data                    *Eth1Data             `json:"eth1_data"`
	Eth1DataVotes               []*Eth1Data           `json:"eth1_data_votes"`
	Eth1DepositIndex            string                `json:"eth1_deposit_index"`
	Validators                  []*Validator          `json:"validators"`
	Balances                    []string              `json:"balances"`
	RandaoMixes                 []string              `json:"randao_mixes"`
	Slashings                   []string              `json:"slashings"`
	PreviousEpochAttestations   []*PendingAttestation `json:"previous_epoch_attestations"`
	CurrentEpochAttestations    []*PendingAttestation `json:"current_epoch_attestations"`
	JustificationBits           string                `json:"justification_bits"`
	PreviousJustifiedCheckpoint *Checkpoint           `json:"previous_justified_checkpoint"`
	CurrentJustifiedCheckpoint  *Checkpoint           `json:"current_justified_checkpoint"`
	FinalizedCheckpoint         *Checkpoint           `json:"finalized_checkpoint"`
}

type BeaconStateAltair struct {
	GenesisTime                 string             `json:"genesis_time"`
	GenesisValidatorsRoot       string             `json:"genesis_validators_root"`
	Slot                        string             `json:"slot"`
	Fork                        *Fork              `json:"fork"`
	LatestBlockHeader           *BeaconBlockHeader `json:"latest_block_header"`
	BlockRoots                  []string           `json:"block_roots"`
	StateRoots                  []string           `json:"state_roots"`
	HistoricalRoots             []string           `json:"historical_roots"`
	Eth1Data                    *Eth1Data          `json:"eth1_data"`
	Eth1DataVotes               []*Eth1Data        `json:"eth1_data_votes"`
	Eth1DepositIndex            string             `json:"eth1_deposit_index"`
	Validators                  []*Validator       `json:"validators"`
	Balances                    []string           `json:"balances"`
	RandaoMixes                 []string           `json:"randao_mixes"`
	Slashings                   []string           `json:"slashings"`
	PreviousEpochParticipation  []string           `json:"previous_epoch_participation"`
	CurrentEpochParticipation   []string           `json:"current_epoch_participation"`
	JustificationBits           string             `json:"justification_bits"`
	PreviousJustifiedCheckpoint *Checkpoint        `json:"previous_justified_checkpoint"`
	CurrentJustifiedCheckpoint  *Checkpoint        `json:"current_justified_checkpoint"`
	FinalizedCheckpoint         *Checkpoint        `json:"finalized_checkpoint"`
	InactivityScores            []string           `json:"inactivity_scores"`
	CurrentSyncCommittee        *SyncCommittee     `json:"current_sync_committee"`
	NextSyncCommittee           *SyncCommittee     `json:"next_sync_committee"`
}

type BeaconStateBellatrix struct {
	GenesisTime                  string                  `json:"genesis_time"`
	GenesisValidatorsRoot        string                  `json:"genesis_validators_root"`
	Slot                         string                  `json:"slot"`
	Fork                         *Fork                   `json:"fork"`
	LatestBlockHeader            *BeaconBlockHeader      `json:"latest_block_header"`
	BlockRoots                   []string                `json:"block_roots"`
	StateRoots                   []string                `json:"state_roots"`
	HistoricalRoots              []string                `json:"historical_roots"`
	Eth1Data                     *Eth1Data               `json:"eth1_data"`
	Eth1DataVotes                []*Eth1Data             `json:"eth1_data_votes"`
	Eth1DepositIndex             string                  `json:"eth1_deposit_index"`
	Validators                   []*Validator            `json:"validators"`
	Balances                     []string                `json:"balances"`
	RandaoMixes                  []string                `json:"randao_mixes"`
	Slashings                    []string                `json:"slashings"`
	PreviousEpochParticipation   []string                `json:"previous_epoch_participation"`
	CurrentEpochParticipation    []string                `json:"current_epoch_participation"`
	JustificationBits            string                  `json:"justification_bits"`
	PreviousJustifiedCheckpoint  *Checkpoint             `json:"previous_justified_checkpoint"`
	CurrentJustifiedCheckpoint   *Checkpoint             `json:"current_justified_checkpoint"`
	FinalizedCheckpoint          *Checkpoint             `json:"finalized_checkpoint"`
	InactivityScores             []string                `json:"inactivity_scores"`
	CurrentSyncCommittee         *SyncCommittee          `json:"current_sync_committee"`
	NextSyncCommittee            *SyncCommittee          `json:"next_sync_committee"`
	LatestExecutionPayloadHeader *ExecutionPayloadHeader `json:"latest_execution_payload_header"`
}

type BeaconStateCapella struct {
	GenesisTime                  string                         `json:"genesis_time"`
	GenesisValidatorsRoot        string                         `json:"genesis_validators_root"`
	Slot                         string                         `json:"slot"`
	Fork                         *Fork                          `json:"fork"`
	LatestBlockHeader            *BeaconBlockHeader             `json:"latest_block_header"`
	BlockRoots                   []string                       `json:"block_roots"`
	StateRoots                   []string                       `json:"state_roots"`
	HistoricalRoots              []string                       `json:"historical_roots"`
	Eth1Data                     *Eth1Data                      `json:"eth1_data"`
	Eth1DataVotes                []*Eth1Data                    `json:"eth1_data_votes"`
	Eth1DepositIndex             string                         `json:"eth1_deposit_index"`
	Validators                   []*Validator                   `json:"validators"`
	Balances                     []string                       `json:"balances"`
	RandaoMixes                  []string                       `json:"randao_mixes"`
	Slashings                    []string                       `json:"slashings"`
	PreviousEpochParticipation   []string                       `json:"previous_epoch_participation"`
	CurrentEpochParticipation    []string                       `json:"current_epoch_participation"`
	JustificationBits            string                         `json:"justification_bits"`
	PreviousJustifiedCheckpoint  *Checkpoint                    `json:"previous_justified_checkpoint"`
	CurrentJustifiedCheckpoint   *Checkpoint                    `json:"current_justified_checkpoint"`
	FinalizedCheckpoint          *Checkpoint                    `json:"finalized_checkpoint"`
	InactivityScores             []string                       `json:"inactivity_scores"`
	CurrentSyncCommittee         *SyncCommittee                 `json:"current_sync_committee"`
	NextSyncCommittee            *SyncCommittee                 `json:"next_sync_committee"`
	LatestExecutionPayloadHeader *ExecutionPayloadHeaderCapella `json:"latest_execution_payload_header"`
	NextWithdrawalIndex          string                         `json:"next_withdrawal_index"`
	NextWithdrawalValidatorIndex string                         `json:"next_withdrawal_validator_index"`
	HistoricalSummaries          []*HistoricalSummary           `json:"historical_summaries"`
}

type BeaconStateDeneb struct {
	GenesisTime                  string                       `json:"genesis_time"`
	GenesisValidatorsRoot        string                       `json:"genesis_validators_root"`
	Slot                         string                       `json:"slot"`
	Fork                         *Fork                        `json:"fork"`
	LatestBlockHeader            *BeaconBlockHeader           `json:"latest_block_header"`
	BlockRoots                   []string                     `json:"block_roots"`
	StateRoots                   []string                     `json:"state_roots"`
	HistoricalRoots              []string                     `json:"historical_roots"`
	Eth1Data                     *Eth1Data                    `json:"eth1_data"`
	Eth1DataVotes                []*Eth1Data                  `json:"eth1_data_votes"`
	Eth1DepositIndex             string                       `json:"eth1_deposit_index"`
	Validators                   []*Validator                 `json:"validators"`
	Balances                     []string                     `json:"balances"`
	RandaoMixes                  []string                     `json:"randao_mixes"`
	Slashings                    []string                     `json:"slashings"`
	PreviousEpochParticipation   []string                     `json:"previous_epoch_participation"`
	CurrentEpochParticipation    []string                     `json:"current_epoch_participation"`
	JustificationBits            string                       `json:"justification_bits"`
	PreviousJustifiedCheckpoint  *Checkpoint                  `json:"previous_justified_checkpoint"`
	CurrentJustifiedCheckpoint   *Checkpoint                  `json:"current_justified_checkpoint"`
	FinalizedCheckpoint          *Checkpoint                  `json:"finalized_checkpoint"`
	InactivityScores             []string                     `json:"inactivity_scores"`
	CurrentSyncCommittee         *SyncCommittee               `json:"current_sync_committee"`
	NextSyncCommittee            *SyncCommittee               `json:"next_sync_committee"`
	LatestExecutionPayloadHeader *ExecutionPayloadHeaderDeneb `json:"latest_execution_payload_header"`
	NextWithdrawalIndex          string                       `json:"next_withdrawal_index"`
	NextWithdrawalValidatorIndex string                       `json:"next_withdrawal_validator_index"`
	HistoricalSummaries          []*HistoricalSummary         `json:"historical_summaries"`
}

type BeaconStateElectra struct {
	GenesisTime                   string                       `json:"genesis_time"`
	GenesisValidatorsRoot         string                       `json:"genesis_validators_root"`
	Slot                          string                       `json:"slot"`
	Fork                          *Fork                        `json:"fork"`
	LatestBlockHeader             *BeaconBlockHeader           `json:"latest_block_header"`
	BlockRoots                    []string                     `json:"block_roots"`
	StateRoots                    []string                     `json:"state_roots"`
	HistoricalRoots               []string                     `json:"historical_roots"`
	Eth1Data                      *Eth1Data                    `json:"eth1_data"`
	Eth1DataVotes                 []*Eth1Data                  `json:"eth1_data_votes"`
	Eth1DepositIndex              string                       `json:"eth1_deposit_index"`
	Validators                    []*Validator                 `json:"validators"`
	Balances                      []string                     `json:"balances"`
	RandaoMixes                   []string                     `json:"randao_mixes"`
	Slashings                     []string                     `json:"slashings"`
	PreviousEpochParticipation    []string                     `json:"previous_epoch_participation"`
	CurrentEpochParticipation     []string                     `json:"current_epoch_participation"`
	JustificationBits             string                       `json:"justification_bits"`
	PreviousJustifiedCheckpoint   *Checkpoint                  `json:"previous_justified_checkpoint"`
	CurrentJustifiedCheckpoint    *Checkpoint                  `json:"current_justified_checkpoint"`
	FinalizedCheckpoint           *Checkpoint                  `json:"finalized_checkpoint"`
	InactivityScores              []string                     `json:"inactivity_scores"`
	CurrentSyncCommittee          *SyncCommittee               `json:"current_sync_committee"`
	NextSyncCommittee             *SyncCommittee               `json:"next_sync_committee"`
	LatestExecutionPayloadHeader  *ExecutionPayloadHeaderDeneb `json:"latest_execution_payload_header"`
	NextWithdrawalIndex           string                       `json:"next_withdrawal_index"`
	NextWithdrawalValidatorIndex  string                       `json:"next_withdrawal_validator_index"`
	HistoricalSummaries           []*HistoricalSummary         `json:"historical_summaries"`
	DepositRequestsStartIndex     string                       `json:"deposit_requests_start_index"`
	DepositBalanceToConsume       string                       `json:"deposit_balance_to_consume"`
	ExitBalanceToConsume          string                       `json:"exit_balance_to_consume"`
	EarliestExitEpoch             string                       `json:"earliest_exit_epoch"`
	ConsolidationBalanceToConsume string                       `json:"consolidation_balance_to_consume"`
	EarliestConsolidationEpoch    string                       `json:"earliest_consolidation_epoch"`
	PendingDeposits               []*PendingDeposit            `json:"pending_deposits"`
	PendingPartialWithdrawals     []*PendingPartialWithdrawal  `json:"pending_partial_withdrawals"`
	PendingConsolidations         []*PendingConsolidation      `json:"pending_consolidations"`
}

type BeaconStateFulu struct {
	GenesisTime                   string                       `json:"genesis_time"`
	GenesisValidatorsRoot         string                       `json:"genesis_validators_root"`
	Slot                          string                       `json:"slot"`
	Fork                          *Fork                        `json:"fork"`
	LatestBlockHeader             *BeaconBlockHeader           `json:"latest_block_header"`
	BlockRoots                    []string                     `json:"block_roots"`
	StateRoots                    []string                     `json:"state_roots"`
	HistoricalRoots               []string                     `json:"historical_roots"`
	Eth1Data                      *Eth1Data                    `json:"eth1_data"`
	Eth1DataVotes                 []*Eth1Data                  `json:"eth1_data_votes"`
	Eth1DepositIndex              string                       `json:"eth1_deposit_index"`
	Validators                    []*Validator                 `json:"validators"`
	Balances                      []string                     `json:"balances"`
	RandaoMixes                   []string                     `json:"randao_mixes"`
	Slashings                     []string                     `json:"slashings"`
	PreviousEpochParticipation    []string                     `json:"previous_epoch_participation"`
	CurrentEpochParticipation     []string                     `json:"current_epoch_participation"`
	JustificationBits             string                       `json:"justification_bits"`
	PreviousJustifiedCheckpoint   *Checkpoint                  `json:"previous_justified_checkpoint"`
	CurrentJustifiedCheckpoint    *Checkpoint                  `json:"current_justified_checkpoint"`
	FinalizedCheckpoint           *Checkpoint                  `json:"finalized_checkpoint"`
	InactivityScores              []string                     `json:"inactivity_scores"`
	CurrentSyncCommittee          *SyncCommittee               `json:"current_sync_committee"`
	NextSyncCommittee             *SyncCommittee               `json:"next_sync_committee"`
	LatestExecutionPayloadHeader  *ExecutionPayloadHeaderDeneb `json:"latest_execution_payload_header"`
	NextWithdrawalIndex           string                       `json:"next_withdrawal_index"`
	NextWithdrawalValidatorIndex  string                       `json:"next_withdrawal_validator_index"`
	HistoricalSummaries           []*HistoricalSummary         `json:"historical_summaries"`
	DepositRequestsStartIndex     string                       `json:"deposit_requests_start_index"`
	DepositBalanceToConsume       string                       `json:"deposit_balance_to_consume"`
	ExitBalanceToConsume          string                       `json:"exit_balance_to_consume"`
	EarliestExitEpoch             string                       `json:"earliest_exit_epoch"`
	ConsolidationBalanceToConsume string                       `json:"consolidation_balance_to_consume"`
	EarliestConsolidationEpoch    string                       `json:"earliest_consolidation_epoch"`
	PendingDeposits               []*PendingDeposit            `json:"pending_deposits"`
	PendingPartialWithdrawals     []*PendingPartialWithdrawal  `json:"pending_partial_withdrawals"`
	PendingConsolidations         []*PendingConsolidation      `json:"pending_consolidations"`
	ProposerLookahead             []string                     `json:"proposer_lookahead"`
}
