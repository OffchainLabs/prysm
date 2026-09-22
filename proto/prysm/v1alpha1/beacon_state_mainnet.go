//go:build !minimal

package eth

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

// BeaconState is the Phase0 SSZ state.
type BeaconState struct {
	GenesisTime                 uint64
	GenesisValidatorsRoot       []byte `ssz-size:"32"`
	Slot                        primitives.Slot
	Fork                        *Fork
	LatestBlockHeader           *BeaconBlockHeader
	BlockRoots                  [][]byte `ssz-size:"8192,32"`
	StateRoots                  [][]byte `ssz-size:"8192,32"`
	HistoricalRoots             [][]byte `ssz-max:"16777216" ssz-size:"?,32"`
	Eth1Data                    *Eth1Data
	Eth1DataVotes               []*Eth1Data `ssz-max:"2048"`
	Eth1DepositIndex            uint64
	Validators                  []*Validator          `ssz-max:"1099511627776"`
	Balances                    []uint64              `ssz-max:"1099511627776"`
	RandaoMixes                 [][]byte              `ssz-size:"65536,32"`
	Slashings                   []uint64              `ssz-size:"8192"`
	PreviousEpochAttestations   []*PendingAttestation `ssz-max:"4096"`
	CurrentEpochAttestations    []*PendingAttestation `ssz-max:"4096"`
	JustificationBits           bitfield.Bitvector4   `ssz-size:"1"`
	PreviousJustifiedCheckpoint *Checkpoint
	CurrentJustifiedCheckpoint  *Checkpoint
	FinalizedCheckpoint         *Checkpoint
}

// HistoricalBatch is the Phase0 container whose root is appended to historical_roots.
type HistoricalBatch struct {
	BlockRoots [][]byte `ssz-size:"8192,32"`
	StateRoots [][]byte `ssz-size:"8192,32"`
}

// BeaconStateAltair is the Altair SSZ state.
type BeaconStateAltair struct {
	GenesisTime                 uint64
	GenesisValidatorsRoot       []byte `ssz-size:"32"`
	Slot                        primitives.Slot
	Fork                        *Fork
	LatestBlockHeader           *BeaconBlockHeader
	BlockRoots                  [][]byte `ssz-size:"8192,32"`
	StateRoots                  [][]byte `ssz-size:"8192,32"`
	HistoricalRoots             [][]byte `ssz-size:"?,32" ssz-max:"16777216"`
	Eth1Data                    *Eth1Data
	Eth1DataVotes               []*Eth1Data `ssz-max:"2048"`
	Eth1DepositIndex            uint64
	Validators                  []*Validator        `ssz-max:"1099511627776"`
	Balances                    []uint64            `ssz-max:"1099511627776"`
	RandaoMixes                 [][]byte            `ssz-size:"65536,32"`
	Slashings                   []uint64            `ssz-size:"8192"`
	PreviousEpochParticipation  []byte              `ssz-max:"1099511627776"`
	CurrentEpochParticipation   []byte              `ssz-max:"1099511627776"`
	JustificationBits           bitfield.Bitvector4 `ssz-size:"1"`
	PreviousJustifiedCheckpoint *Checkpoint
	CurrentJustifiedCheckpoint  *Checkpoint
	FinalizedCheckpoint         *Checkpoint
	InactivityScores            []uint64 `ssz-max:"1099511627776"`
	CurrentSyncCommittee        *SyncCommittee
	NextSyncCommittee           *SyncCommittee
}

// BeaconStateBellatrix is the Bellatrix SSZ state.
type BeaconStateBellatrix struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        []byte `ssz-size:"32"`
	Slot                         primitives.Slot
	Fork                         *Fork
	LatestBlockHeader            *BeaconBlockHeader
	BlockRoots                   [][]byte `ssz-size:"8192,32"`
	StateRoots                   [][]byte `ssz-size:"8192,32"`
	HistoricalRoots              [][]byte `ssz-max:"16777216" ssz-size:"?,32"`
	Eth1Data                     *Eth1Data
	Eth1DataVotes                []*Eth1Data `ssz-max:"2048"`
	Eth1DepositIndex             uint64
	Validators                   []*Validator        `ssz-max:"1099511627776"`
	Balances                     []uint64            `ssz-max:"1099511627776"`
	RandaoMixes                  [][]byte            `ssz-size:"65536,32"`
	Slashings                    []uint64            `ssz-size:"8192"`
	PreviousEpochParticipation   []byte              `ssz-max:"1099511627776"`
	CurrentEpochParticipation    []byte              `ssz-max:"1099511627776"`
	JustificationBits            bitfield.Bitvector4 `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Checkpoint
	CurrentJustifiedCheckpoint   *Checkpoint
	FinalizedCheckpoint          *Checkpoint
	InactivityScores             []uint64 `ssz-max:"1099511627776"`
	CurrentSyncCommittee         *SyncCommittee
	NextSyncCommittee            *SyncCommittee
	LatestExecutionPayloadHeader *enginev1.ExecutionPayloadHeader
}

// BeaconStateCapella is the Capella SSZ state.
type BeaconStateCapella struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        []byte `ssz-size:"32"`
	Slot                         primitives.Slot
	Fork                         *Fork
	LatestBlockHeader            *BeaconBlockHeader
	BlockRoots                   [][]byte `ssz-size:"8192,32"`
	StateRoots                   [][]byte `ssz-size:"8192,32"`
	HistoricalRoots              [][]byte `ssz-max:"16777216" ssz-size:"?,32"`
	Eth1Data                     *Eth1Data
	Eth1DataVotes                []*Eth1Data `ssz-max:"2048"`
	Eth1DepositIndex             uint64
	Validators                   []*Validator        `ssz-max:"1099511627776"`
	Balances                     []uint64            `ssz-max:"1099511627776"`
	RandaoMixes                  [][]byte            `ssz-size:"65536,32"`
	Slashings                    []uint64            `ssz-size:"8192"`
	PreviousEpochParticipation   []byte              `ssz-max:"1099511627776"`
	CurrentEpochParticipation    []byte              `ssz-max:"1099511627776"`
	JustificationBits            bitfield.Bitvector4 `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Checkpoint
	CurrentJustifiedCheckpoint   *Checkpoint
	FinalizedCheckpoint          *Checkpoint
	InactivityScores             []uint64 `ssz-max:"1099511627776"`
	CurrentSyncCommittee         *SyncCommittee
	NextSyncCommittee            *SyncCommittee
	LatestExecutionPayloadHeader *enginev1.ExecutionPayloadHeaderCapella
	NextWithdrawalIndex          uint64
	NextWithdrawalValidatorIndex primitives.ValidatorIndex
	HistoricalSummaries          []*HistoricalSummary `ssz-max:"16777216"`
}

// BeaconStateDeneb is the Deneb SSZ state.
type BeaconStateDeneb struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        []byte `ssz-size:"32"`
	Slot                         primitives.Slot
	Fork                         *Fork
	LatestBlockHeader            *BeaconBlockHeader
	BlockRoots                   [][]byte `ssz-size:"8192,32"`
	StateRoots                   [][]byte `ssz-size:"8192,32"`
	HistoricalRoots              [][]byte `ssz-max:"16777216" ssz-size:"?,32"`
	Eth1Data                     *Eth1Data
	Eth1DataVotes                []*Eth1Data `ssz-max:"2048"`
	Eth1DepositIndex             uint64
	Validators                   []*Validator        `ssz-max:"1099511627776"`
	Balances                     []uint64            `ssz-max:"1099511627776"`
	RandaoMixes                  [][]byte            `ssz-size:"65536,32"`
	Slashings                    []uint64            `ssz-size:"8192"`
	PreviousEpochParticipation   []byte              `ssz-max:"1099511627776"`
	CurrentEpochParticipation    []byte              `ssz-max:"1099511627776"`
	JustificationBits            bitfield.Bitvector4 `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Checkpoint
	CurrentJustifiedCheckpoint   *Checkpoint
	FinalizedCheckpoint          *Checkpoint
	InactivityScores             []uint64 `ssz-max:"1099511627776"`
	CurrentSyncCommittee         *SyncCommittee
	NextSyncCommittee            *SyncCommittee
	LatestExecutionPayloadHeader *enginev1.ExecutionPayloadHeaderDeneb
	NextWithdrawalIndex          uint64
	NextWithdrawalValidatorIndex primitives.ValidatorIndex
	HistoricalSummaries          []*HistoricalSummary `ssz-max:"16777216"`
}

// BeaconStateElectra is the Electra SSZ state.
type BeaconStateElectra struct {
	GenesisTime                   uint64
	GenesisValidatorsRoot         []byte `ssz-size:"32"`
	Slot                          primitives.Slot
	Fork                          *Fork
	LatestBlockHeader             *BeaconBlockHeader
	BlockRoots                    [][]byte `ssz-size:"8192,32"`
	StateRoots                    [][]byte `ssz-size:"8192,32"`
	HistoricalRoots               [][]byte `ssz-max:"16777216" ssz-size:"?,32"`
	Eth1Data                      *Eth1Data
	Eth1DataVotes                 []*Eth1Data `ssz-max:"2048"`
	Eth1DepositIndex              uint64
	Validators                    []*Validator        `ssz-max:"1099511627776"`
	Balances                      []uint64            `ssz-max:"1099511627776"`
	RandaoMixes                   [][]byte            `ssz-size:"65536,32"`
	Slashings                     []uint64            `ssz-size:"8192"`
	PreviousEpochParticipation    []byte              `ssz-max:"1099511627776"`
	CurrentEpochParticipation     []byte              `ssz-max:"1099511627776"`
	JustificationBits             bitfield.Bitvector4 `ssz-size:"1"`
	PreviousJustifiedCheckpoint   *Checkpoint
	CurrentJustifiedCheckpoint    *Checkpoint
	FinalizedCheckpoint           *Checkpoint
	InactivityScores              []uint64 `ssz-max:"1099511627776"`
	CurrentSyncCommittee          *SyncCommittee
	NextSyncCommittee             *SyncCommittee
	LatestExecutionPayloadHeader  *enginev1.ExecutionPayloadHeaderDeneb
	NextWithdrawalIndex           uint64
	NextWithdrawalValidatorIndex  primitives.ValidatorIndex
	HistoricalSummaries           []*HistoricalSummary `ssz-max:"16777216"`
	DepositRequestsStartIndex     uint64
	DepositBalanceToConsume       primitives.Gwei
	ExitBalanceToConsume          primitives.Gwei
	EarliestExitEpoch             primitives.Epoch
	ConsolidationBalanceToConsume primitives.Gwei
	EarliestConsolidationEpoch    primitives.Epoch
	PendingDeposits               []*PendingDeposit           `ssz-max:"134217728"`
	PendingPartialWithdrawals     []*PendingPartialWithdrawal `ssz-max:"134217728"`
	PendingConsolidations         []*PendingConsolidation     `ssz-max:"262144"`
}

// BeaconStateFulu is the Fulu SSZ state.
type BeaconStateFulu struct {
	GenesisTime                   uint64
	GenesisValidatorsRoot         []byte `ssz-size:"32"`
	Slot                          primitives.Slot
	Fork                          *Fork
	LatestBlockHeader             *BeaconBlockHeader
	BlockRoots                    [][]byte `ssz-size:"8192,32"`
	StateRoots                    [][]byte `ssz-size:"8192,32"`
	HistoricalRoots               [][]byte `ssz-max:"16777216" ssz-size:"?,32"`
	Eth1Data                      *Eth1Data
	Eth1DataVotes                 []*Eth1Data `ssz-max:"2048"`
	Eth1DepositIndex              uint64
	Validators                    []*Validator        `ssz-max:"1099511627776"`
	Balances                      []uint64            `ssz-max:"1099511627776"`
	RandaoMixes                   [][]byte            `ssz-size:"65536,32"`
	Slashings                     []uint64            `ssz-size:"8192"`
	PreviousEpochParticipation    []byte              `ssz-max:"1099511627776"`
	CurrentEpochParticipation     []byte              `ssz-max:"1099511627776"`
	JustificationBits             bitfield.Bitvector4 `ssz-size:"1"`
	PreviousJustifiedCheckpoint   *Checkpoint
	CurrentJustifiedCheckpoint    *Checkpoint
	FinalizedCheckpoint           *Checkpoint
	InactivityScores              []uint64 `ssz-max:"1099511627776"`
	CurrentSyncCommittee          *SyncCommittee
	NextSyncCommittee             *SyncCommittee
	LatestExecutionPayloadHeader  *enginev1.ExecutionPayloadHeaderDeneb
	NextWithdrawalIndex           uint64
	NextWithdrawalValidatorIndex  primitives.ValidatorIndex
	HistoricalSummaries           []*HistoricalSummary `ssz-max:"16777216"`
	DepositRequestsStartIndex     uint64
	DepositBalanceToConsume       primitives.Gwei
	ExitBalanceToConsume          primitives.Gwei
	EarliestExitEpoch             primitives.Epoch
	ConsolidationBalanceToConsume primitives.Gwei
	EarliestConsolidationEpoch    primitives.Epoch
	PendingDeposits               []*PendingDeposit           `ssz-max:"134217728"`
	PendingPartialWithdrawals     []*PendingPartialWithdrawal `ssz-max:"134217728"`
	PendingConsolidations         []*PendingConsolidation     `ssz-max:"262144"`
	ProposerLookahead             []primitives.ValidatorIndex `ssz-size:"64"`
}

// BeaconStateGloas is the Gloas SSZ state.
type BeaconStateGloas struct {
	GenesisTime                   uint64
	GenesisValidatorsRoot         []byte `ssz-size:"32"`
	Slot                          primitives.Slot
	Fork                          *Fork
	LatestBlockHeader             *BeaconBlockHeader
	BlockRoots                    [][]byte `ssz-size:"8192,32"`
	StateRoots                    [][]byte `ssz-size:"8192,32"`
	HistoricalRoots               [][]byte `ssz-max:"16777216" ssz-size:"?,32"`
	Eth1Data                      *Eth1Data
	Eth1DataVotes                 []*Eth1Data `ssz-max:"2048"`
	Eth1DepositIndex              uint64
	Validators                    []*Validator        `ssz-max:"1099511627776"`
	Balances                      []uint64            `ssz-max:"1099511627776"`
	RandaoMixes                   [][]byte            `ssz-size:"65536,32"`
	Slashings                     []uint64            `ssz-size:"8192"`
	PreviousEpochParticipation    []byte              `ssz-max:"1099511627776"`
	CurrentEpochParticipation     []byte              `ssz-max:"1099511627776"`
	JustificationBits             bitfield.Bitvector4 `ssz-size:"1"`
	PreviousJustifiedCheckpoint   *Checkpoint
	CurrentJustifiedCheckpoint    *Checkpoint
	FinalizedCheckpoint           *Checkpoint
	InactivityScores              []uint64 `ssz-max:"1099511627776"`
	CurrentSyncCommittee          *SyncCommittee
	NextSyncCommittee             *SyncCommittee
	LatestBlockHash               []byte `ssz-size:"32"`
	NextWithdrawalIndex           uint64
	NextWithdrawalValidatorIndex  primitives.ValidatorIndex
	HistoricalSummaries           []*HistoricalSummary `ssz-max:"16777216"`
	DepositRequestsStartIndex     uint64
	DepositBalanceToConsume       primitives.Gwei
	ExitBalanceToConsume          primitives.Gwei
	EarliestExitEpoch             primitives.Epoch
	ConsolidationBalanceToConsume primitives.Gwei
	EarliestConsolidationEpoch    primitives.Epoch
	PendingDeposits               []*PendingDeposit           `ssz-max:"134217728"`
	PendingPartialWithdrawals     []*PendingPartialWithdrawal `ssz-max:"134217728"`
	PendingConsolidations         []*PendingConsolidation     `ssz-max:"262144"`
	ProposerLookahead             []primitives.ValidatorIndex `ssz-size:"64"`
	Builders                      []*Builder                  `ssz-max:"1099511627776"`
	NextWithdrawalBuilderIndex    primitives.BuilderIndex
	ExecutionPayloadAvailability  []byte                      `ssz-size:"1024"`
	BuilderPendingPayments        []*BuilderPendingPayment    `ssz-size:"64"`
	BuilderPendingWithdrawals     []*BuilderPendingWithdrawal `ssz-max:"1048576"`
	LatestExecutionPayloadBid     *ExecutionPayloadBid
	PayloadExpectedWithdrawals    []*enginev1.Withdrawal `ssz-max:"16"`
	PtcWindow                     []*PTCs                `ssz-size:"96"`
}
