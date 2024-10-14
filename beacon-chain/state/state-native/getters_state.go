package state_native

import (
	"github.com/pkg/errors"
	customtypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/custom-types"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// ToProtoUnsafe returns the pointer value of the underlying
// beacon state proto object, bypassing immutability. Use with care.
func (b *BeaconState) ToProtoUnsafe() interface{} {
	if b == nil {
		return nil
	}

	gvrCopy := b.genesisValidatorsRoot
	br := b.blockRootsVal().Slice()
	sr := b.stateRootsVal().Slice()
	rm := b.randaoMixesVal().Slice()
	var vals []*ethpb.Validator
	var bals []uint64
	if features.Get().EnableExperimentalState {
		vals = b.validatorsVal()
		bals = b.balancesVal()
	} else {
		vals = b.validators
		bals = b.balances
	}

	switch b.version {
	case version.Phase0:
		return &ethpb.BeaconState{
			GenesisTime:                 b.genesisTime,
			GenesisValidatorsRoot:       gvrCopy[:],
			Slot:                        b.slot,
			Fork:                        b.fork,
			LatestBlockHeader:           b.latestBlockHeader,
			BlockRoots:                  br,
			StateRoots:                  sr,
			HistoricalRoots:             b.historicalRoots.Slice(),
			Eth1Data:                    b.eth1Data,
			Eth1DataVotes:               b.eth1DataVotes,
			Eth1DepositIndex:            b.eth1DepositIndex,
			Validators:                  vals,
			Balances:                    bals,
			RandaoMixes:                 rm,
			Slashings:                   b.slashings,
			PreviousEpochAttestations:   b.previousEpochAttestations,
			CurrentEpochAttestations:    b.currentEpochAttestations,
			JustificationBits:           b.justificationBits,
			PreviousJustifiedCheckpoint: b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:  b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:         b.finalizedCheckpoint,
		}
	case version.Altair:
		return &ethpb.BeaconStateAltair{
			GenesisTime:                 b.genesisTime,
			GenesisValidatorsRoot:       gvrCopy[:],
			Slot:                        b.slot,
			Fork:                        b.fork,
			LatestBlockHeader:           b.latestBlockHeader,
			BlockRoots:                  br,
			StateRoots:                  sr,
			HistoricalRoots:             b.historicalRoots.Slice(),
			Eth1Data:                    b.eth1Data,
			Eth1DataVotes:               b.eth1DataVotes,
			Eth1DepositIndex:            b.eth1DepositIndex,
			Validators:                  vals,
			Balances:                    bals,
			RandaoMixes:                 rm,
			Slashings:                   b.slashings,
			PreviousEpochParticipation:  b.previousEpochParticipation,
			CurrentEpochParticipation:   b.currentEpochParticipation,
			JustificationBits:           b.justificationBits,
			PreviousJustifiedCheckpoint: b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:  b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:         b.finalizedCheckpoint,
			InactivityScores:            b.inactivityScoresVal(),
			CurrentSyncCommittee:        b.currentSyncCommittee,
			NextSyncCommittee:           b.nextSyncCommittee,
		}
	case version.Bellatrix:
		return &ethpb.BeaconStateBellatrix{
			GenesisTime:                  b.genesisTime,
			GenesisValidatorsRoot:        gvrCopy[:],
			Slot:                         b.slot,
			Fork:                         b.fork,
			LatestBlockHeader:            b.latestBlockHeader,
			BlockRoots:                   br,
			StateRoots:                   sr,
			HistoricalRoots:              b.historicalRoots.Slice(),
			Eth1Data:                     b.eth1Data,
			Eth1DataVotes:                b.eth1DataVotes,
			Eth1DepositIndex:             b.eth1DepositIndex,
			Validators:                   vals,
			Balances:                     bals,
			RandaoMixes:                  rm,
			Slashings:                    b.slashings,
			PreviousEpochParticipation:   b.previousEpochParticipation,
			CurrentEpochParticipation:    b.currentEpochParticipation,
			JustificationBits:            b.justificationBits,
			PreviousJustifiedCheckpoint:  b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:   b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:          b.finalizedCheckpoint,
			InactivityScores:             b.inactivityScoresVal(),
			CurrentSyncCommittee:         b.currentSyncCommittee,
			NextSyncCommittee:            b.nextSyncCommittee,
			LatestExecutionPayloadHeader: b.latestExecutionPayloadHeader,
		}
	case version.Capella:
		return &ethpb.BeaconStateCapella{
			GenesisTime:                  b.genesisTime,
			GenesisValidatorsRoot:        gvrCopy[:],
			Slot:                         b.slot,
			Fork:                         b.fork,
			LatestBlockHeader:            b.latestBlockHeader,
			BlockRoots:                   br,
			StateRoots:                   sr,
			HistoricalRoots:              b.historicalRoots.Slice(),
			Eth1Data:                     b.eth1Data,
			Eth1DataVotes:                b.eth1DataVotes,
			Eth1DepositIndex:             b.eth1DepositIndex,
			Validators:                   vals,
			Balances:                     bals,
			RandaoMixes:                  rm,
			Slashings:                    b.slashings,
			PreviousEpochParticipation:   b.previousEpochParticipation,
			CurrentEpochParticipation:    b.currentEpochParticipation,
			JustificationBits:            b.justificationBits,
			PreviousJustifiedCheckpoint:  b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:   b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:          b.finalizedCheckpoint,
			InactivityScores:             b.inactivityScoresVal(),
			CurrentSyncCommittee:         b.currentSyncCommittee,
			NextSyncCommittee:            b.nextSyncCommittee,
			LatestExecutionPayloadHeader: b.latestExecutionPayloadHeaderCapella,
			NextWithdrawalIndex:          b.nextWithdrawalIndex,
			NextWithdrawalValidatorIndex: b.nextWithdrawalValidatorIndex,
			HistoricalSummaries:          b.historicalSummaries,
		}
	case version.Deneb:
		return &ethpb.BeaconStateDeneb{
			GenesisTime:                  b.genesisTime,
			GenesisValidatorsRoot:        gvrCopy[:],
			Slot:                         b.slot,
			Fork:                         b.fork,
			LatestBlockHeader:            b.latestBlockHeader,
			BlockRoots:                   br,
			StateRoots:                   sr,
			HistoricalRoots:              b.historicalRoots.Slice(),
			Eth1Data:                     b.eth1Data,
			Eth1DataVotes:                b.eth1DataVotes,
			Eth1DepositIndex:             b.eth1DepositIndex,
			Validators:                   vals,
			Balances:                     bals,
			RandaoMixes:                  rm,
			Slashings:                    b.slashings,
			PreviousEpochParticipation:   b.previousEpochParticipation,
			CurrentEpochParticipation:    b.currentEpochParticipation,
			JustificationBits:            b.justificationBits,
			PreviousJustifiedCheckpoint:  b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:   b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:          b.finalizedCheckpoint,
			InactivityScores:             b.inactivityScoresVal(),
			CurrentSyncCommittee:         b.currentSyncCommittee,
			NextSyncCommittee:            b.nextSyncCommittee,
			LatestExecutionPayloadHeader: b.latestExecutionPayloadHeaderDeneb,
			NextWithdrawalIndex:          b.nextWithdrawalIndex,
			NextWithdrawalValidatorIndex: b.nextWithdrawalValidatorIndex,
			HistoricalSummaries:          b.historicalSummaries,
		}
	case version.Electra:
		return &ethpb.BeaconStateElectra{
			GenesisTime:                   b.genesisTime,
			GenesisValidatorsRoot:         gvrCopy[:],
			Slot:                          b.slot,
			Fork:                          b.fork,
			LatestBlockHeader:             b.latestBlockHeader,
			BlockRoots:                    br,
			StateRoots:                    sr,
			HistoricalRoots:               b.historicalRoots.Slice(),
			Eth1Data:                      b.eth1Data,
			Eth1DataVotes:                 b.eth1DataVotes,
			Eth1DepositIndex:              b.eth1DepositIndex,
			Validators:                    vals,
			Balances:                      bals,
			RandaoMixes:                   rm,
			Slashings:                     b.slashings,
			PreviousEpochParticipation:    b.previousEpochParticipation,
			CurrentEpochParticipation:     b.currentEpochParticipation,
			JustificationBits:             b.justificationBits,
			PreviousJustifiedCheckpoint:   b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:    b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:           b.finalizedCheckpoint,
			InactivityScores:              b.inactivityScoresVal(),
			CurrentSyncCommittee:          b.currentSyncCommittee,
			NextSyncCommittee:             b.nextSyncCommittee,
			LatestExecutionPayloadHeader:  b.latestExecutionPayloadHeaderDeneb,
			NextWithdrawalIndex:           b.nextWithdrawalIndex,
			NextWithdrawalValidatorIndex:  b.nextWithdrawalValidatorIndex,
			HistoricalSummaries:           b.historicalSummaries,
			DepositRequestsStartIndex:     b.depositRequestsStartIndex,
			DepositBalanceToConsume:       b.depositBalanceToConsume,
			ExitBalanceToConsume:          b.exitBalanceToConsume,
			EarliestExitEpoch:             b.earliestExitEpoch,
			ConsolidationBalanceToConsume: b.consolidationBalanceToConsume,
			EarliestConsolidationEpoch:    b.earliestConsolidationEpoch,
			PendingDeposits:               b.pendingDeposits,
			PendingPartialWithdrawals:     b.pendingPartialWithdrawals,
			PendingConsolidations:         b.pendingConsolidations,
		}
	case version.EPBS:
		return &ethpb.BeaconStateEPBS{
			GenesisTime:                   b.genesisTime,
			GenesisValidatorsRoot:         gvrCopy[:],
			Slot:                          b.slot,
			Fork:                          b.fork,
			LatestBlockHeader:             b.latestBlockHeader,
			BlockRoots:                    br,
			StateRoots:                    sr,
			HistoricalRoots:               b.historicalRoots.Slice(),
			Eth1Data:                      b.eth1Data,
			Eth1DataVotes:                 b.eth1DataVotes,
			Eth1DepositIndex:              b.eth1DepositIndex,
			Validators:                    vals,
			Balances:                      bals,
			RandaoMixes:                   rm,
			Slashings:                     b.slashings,
			PreviousEpochParticipation:    b.previousEpochParticipation,
			CurrentEpochParticipation:     b.currentEpochParticipation,
			JustificationBits:             b.justificationBits,
			PreviousJustifiedCheckpoint:   b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:    b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:           b.finalizedCheckpoint,
			InactivityScores:              b.inactivityScoresVal(),
			CurrentSyncCommittee:          b.currentSyncCommittee,
			NextSyncCommittee:             b.nextSyncCommittee,
			NextWithdrawalIndex:           b.nextWithdrawalIndex,
			NextWithdrawalValidatorIndex:  b.nextWithdrawalValidatorIndex,
			HistoricalSummaries:           b.historicalSummaries,
			DepositRequestsStartIndex:     b.depositRequestsStartIndex,
			DepositBalanceToConsume:       b.depositBalanceToConsume,
			ExitBalanceToConsume:          b.exitBalanceToConsume,
			EarliestExitEpoch:             b.earliestExitEpoch,
			ConsolidationBalanceToConsume: b.consolidationBalanceToConsume,
			EarliestConsolidationEpoch:    b.earliestConsolidationEpoch,
			PendingBalanceDeposits:        b.pendingBalanceDeposits,
			PendingPartialWithdrawals:     b.pendingPartialWithdrawals,
			PendingConsolidations:         b.pendingConsolidations,
			PreviousInclusionListProposer: b.previousInclusionListProposer,
			PreviousInclusionListSlot:     b.previousInclusionListSlot,
			LatestInclusionListProposer:   b.latestInclusionListProposer,
			LatestInclusionListSlot:       b.latestInclusionListSlot,
			LatestBlockHash:               b.latestBlockHash[:],
			LatestFullSlot:                b.latestFullSlot,
			ExecutionPayloadHeader:        b.executionPayloadHeader,
			LastWithdrawalsRoot:           b.lastWithdrawalsRoot[:],
		}
	default:
		return nil
	}
}

// ToProto the beacon state into a protobuf for usage.
func (b *BeaconState) ToProto() interface{} {
	if b == nil {
		return nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	gvrCopy := b.genesisValidatorsRoot
	br := b.blockRootsVal().Slice()
	sr := b.stateRootsVal().Slice()
	rm := b.randaoMixesVal().Slice()

	var inactivityScores []uint64
	if b.version > version.Phase0 {
		inactivityScores = b.inactivityScoresVal()
	}

	LatestBlockHashCopy := b.latestBlockHash
	lastWithdrawalsRootCopy := b.lastWithdrawalsRoot

	switch b.version {
	case version.Phase0:
		return &ethpb.BeaconState{
			GenesisTime:                 b.genesisTime,
			GenesisValidatorsRoot:       gvrCopy[:],
			Slot:                        b.slot,
			Fork:                        b.forkVal(),
			LatestBlockHeader:           b.latestBlockHeaderVal(),
			BlockRoots:                  br,
			StateRoots:                  sr,
			HistoricalRoots:             b.historicalRoots.Slice(),
			Eth1Data:                    b.eth1DataVal(),
			Eth1DataVotes:               b.eth1DataVotesVal(),
			Eth1DepositIndex:            b.eth1DepositIndex,
			Validators:                  b.validatorsVal(),
			Balances:                    b.balancesVal(),
			RandaoMixes:                 rm,
			Slashings:                   b.slashingsVal(),
			PreviousEpochAttestations:   b.previousEpochAttestationsVal(),
			CurrentEpochAttestations:    b.currentEpochAttestationsVal(),
			JustificationBits:           b.justificationBitsVal(),
			PreviousJustifiedCheckpoint: b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:  b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:         b.finalizedCheckpointVal(),
		}
	case version.Altair:
		return &ethpb.BeaconStateAltair{
			GenesisTime:                 b.genesisTime,
			GenesisValidatorsRoot:       gvrCopy[:],
			Slot:                        b.slot,
			Fork:                        b.forkVal(),
			LatestBlockHeader:           b.latestBlockHeaderVal(),
			BlockRoots:                  br,
			StateRoots:                  sr,
			HistoricalRoots:             b.historicalRoots.Slice(),
			Eth1Data:                    b.eth1DataVal(),
			Eth1DataVotes:               b.eth1DataVotesVal(),
			Eth1DepositIndex:            b.eth1DepositIndex,
			Validators:                  b.validatorsVal(),
			Balances:                    b.balancesVal(),
			RandaoMixes:                 rm,
			Slashings:                   b.slashingsVal(),
			PreviousEpochParticipation:  b.previousEpochParticipationVal(),
			CurrentEpochParticipation:   b.currentEpochParticipationVal(),
			JustificationBits:           b.justificationBitsVal(),
			PreviousJustifiedCheckpoint: b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:  b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:         b.finalizedCheckpointVal(),
			InactivityScores:            inactivityScores,
			CurrentSyncCommittee:        b.currentSyncCommitteeVal(),
			NextSyncCommittee:           b.nextSyncCommitteeVal(),
		}
	case version.Bellatrix:
		return &ethpb.BeaconStateBellatrix{
			GenesisTime:                  b.genesisTime,
			GenesisValidatorsRoot:        gvrCopy[:],
			Slot:                         b.slot,
			Fork:                         b.forkVal(),
			LatestBlockHeader:            b.latestBlockHeaderVal(),
			BlockRoots:                   br,
			StateRoots:                   sr,
			HistoricalRoots:              b.historicalRoots.Slice(),
			Eth1Data:                     b.eth1DataVal(),
			Eth1DataVotes:                b.eth1DataVotesVal(),
			Eth1DepositIndex:             b.eth1DepositIndex,
			Validators:                   b.validatorsVal(),
			Balances:                     b.balancesVal(),
			RandaoMixes:                  rm,
			Slashings:                    b.slashingsVal(),
			PreviousEpochParticipation:   b.previousEpochParticipationVal(),
			CurrentEpochParticipation:    b.currentEpochParticipationVal(),
			JustificationBits:            b.justificationBitsVal(),
			PreviousJustifiedCheckpoint:  b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:   b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:          b.finalizedCheckpointVal(),
			InactivityScores:             inactivityScores,
			CurrentSyncCommittee:         b.currentSyncCommitteeVal(),
			NextSyncCommittee:            b.nextSyncCommitteeVal(),
			LatestExecutionPayloadHeader: b.latestExecutionPayloadHeader.Copy(),
		}
	case version.Capella:
		return &ethpb.BeaconStateCapella{
			GenesisTime:                  b.genesisTime,
			GenesisValidatorsRoot:        gvrCopy[:],
			Slot:                         b.slot,
			Fork:                         b.forkVal(),
			LatestBlockHeader:            b.latestBlockHeaderVal(),
			BlockRoots:                   br,
			StateRoots:                   sr,
			HistoricalRoots:              b.historicalRoots.Slice(),
			Eth1Data:                     b.eth1DataVal(),
			Eth1DataVotes:                b.eth1DataVotesVal(),
			Eth1DepositIndex:             b.eth1DepositIndex,
			Validators:                   b.validatorsVal(),
			Balances:                     b.balancesVal(),
			RandaoMixes:                  rm,
			Slashings:                    b.slashingsVal(),
			PreviousEpochParticipation:   b.previousEpochParticipationVal(),
			CurrentEpochParticipation:    b.currentEpochParticipationVal(),
			JustificationBits:            b.justificationBitsVal(),
			PreviousJustifiedCheckpoint:  b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:   b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:          b.finalizedCheckpointVal(),
			InactivityScores:             inactivityScores,
			CurrentSyncCommittee:         b.currentSyncCommitteeVal(),
			NextSyncCommittee:            b.nextSyncCommitteeVal(),
			LatestExecutionPayloadHeader: b.latestExecutionPayloadHeaderCapella.Copy(),
			NextWithdrawalIndex:          b.nextWithdrawalIndex,
			NextWithdrawalValidatorIndex: b.nextWithdrawalValidatorIndex,
			HistoricalSummaries:          b.historicalSummariesVal(),
		}
	case version.Deneb:
		return &ethpb.BeaconStateDeneb{
			GenesisTime:                  b.genesisTime,
			GenesisValidatorsRoot:        gvrCopy[:],
			Slot:                         b.slot,
			Fork:                         b.forkVal(),
			LatestBlockHeader:            b.latestBlockHeaderVal(),
			BlockRoots:                   br,
			StateRoots:                   sr,
			HistoricalRoots:              b.historicalRoots.Slice(),
			Eth1Data:                     b.eth1DataVal(),
			Eth1DataVotes:                b.eth1DataVotesVal(),
			Eth1DepositIndex:             b.eth1DepositIndex,
			Validators:                   b.validatorsVal(),
			Balances:                     b.balancesVal(),
			RandaoMixes:                  rm,
			Slashings:                    b.slashingsVal(),
			PreviousEpochParticipation:   b.previousEpochParticipationVal(),
			CurrentEpochParticipation:    b.currentEpochParticipationVal(),
			JustificationBits:            b.justificationBitsVal(),
			PreviousJustifiedCheckpoint:  b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:   b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:          b.finalizedCheckpointVal(),
			InactivityScores:             b.inactivityScoresVal(),
			CurrentSyncCommittee:         b.currentSyncCommitteeVal(),
			NextSyncCommittee:            b.nextSyncCommitteeVal(),
			LatestExecutionPayloadHeader: b.latestExecutionPayloadHeaderDeneb.Copy(),
			NextWithdrawalIndex:          b.nextWithdrawalIndex,
			NextWithdrawalValidatorIndex: b.nextWithdrawalValidatorIndex,
			HistoricalSummaries:          b.historicalSummariesVal(),
		}
	case version.Electra:
		return &ethpb.BeaconStateElectra{
			GenesisTime:                   b.genesisTime,
			GenesisValidatorsRoot:         gvrCopy[:],
			Slot:                          b.slot,
			Fork:                          b.forkVal(),
			LatestBlockHeader:             b.latestBlockHeaderVal(),
			BlockRoots:                    br,
			StateRoots:                    sr,
			HistoricalRoots:               b.historicalRoots.Slice(),
			Eth1Data:                      b.eth1DataVal(),
			Eth1DataVotes:                 b.eth1DataVotesVal(),
			Eth1DepositIndex:              b.eth1DepositIndex,
			Validators:                    b.validatorsVal(),
			Balances:                      b.balancesVal(),
			RandaoMixes:                   rm,
			Slashings:                     b.slashingsVal(),
			PreviousEpochParticipation:    b.previousEpochParticipationVal(),
			CurrentEpochParticipation:     b.currentEpochParticipationVal(),
			JustificationBits:             b.justificationBitsVal(),
			PreviousJustifiedCheckpoint:   b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:    b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:           b.finalizedCheckpointVal(),
			InactivityScores:              b.inactivityScoresVal(),
			CurrentSyncCommittee:          b.currentSyncCommitteeVal(),
			NextSyncCommittee:             b.nextSyncCommitteeVal(),
			LatestExecutionPayloadHeader:  b.latestExecutionPayloadHeaderDeneb.Copy(),
			NextWithdrawalIndex:           b.nextWithdrawalIndex,
			NextWithdrawalValidatorIndex:  b.nextWithdrawalValidatorIndex,
			HistoricalSummaries:           b.historicalSummariesVal(),
			DepositRequestsStartIndex:     b.depositRequestsStartIndex,
			DepositBalanceToConsume:       b.depositBalanceToConsume,
			ExitBalanceToConsume:          b.exitBalanceToConsume,
			EarliestExitEpoch:             b.earliestExitEpoch,
			ConsolidationBalanceToConsume: b.consolidationBalanceToConsume,
			EarliestConsolidationEpoch:    b.earliestConsolidationEpoch,
			PendingDeposits:               b.pendingDepositsVal(),
			PendingPartialWithdrawals:     b.pendingPartialWithdrawalsVal(),
			PendingConsolidations:         b.pendingConsolidationsVal(),
		}
	case version.EPBS:
		return &ethpb.BeaconStateEPBS{
			GenesisTime:                   b.genesisTime,
			GenesisValidatorsRoot:         gvrCopy[:],
			Slot:                          b.slot,
			Fork:                          b.forkVal(),
			LatestBlockHeader:             b.latestBlockHeaderVal(),
			BlockRoots:                    br,
			StateRoots:                    sr,
			HistoricalRoots:               b.historicalRoots.Slice(),
			Eth1Data:                      b.eth1DataVal(),
			Eth1DataVotes:                 b.eth1DataVotesVal(),
			Eth1DepositIndex:              b.eth1DepositIndex,
			Validators:                    b.validatorsVal(),
			Balances:                      b.balancesVal(),
			RandaoMixes:                   rm,
			Slashings:                     b.slashingsVal(),
			PreviousEpochParticipation:    b.previousEpochParticipationVal(),
			CurrentEpochParticipation:     b.currentEpochParticipationVal(),
			JustificationBits:             b.justificationBitsVal(),
			PreviousJustifiedCheckpoint:   b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:    b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:           b.finalizedCheckpointVal(),
			InactivityScores:              b.inactivityScoresVal(),
			CurrentSyncCommittee:          b.currentSyncCommitteeVal(),
			NextSyncCommittee:             b.nextSyncCommitteeVal(),
			NextWithdrawalIndex:           b.nextWithdrawalIndex,
			NextWithdrawalValidatorIndex:  b.nextWithdrawalValidatorIndex,
			HistoricalSummaries:           b.historicalSummariesVal(),
			DepositRequestsStartIndex:     b.depositRequestsStartIndex,
			DepositBalanceToConsume:       b.depositBalanceToConsume,
			ExitBalanceToConsume:          b.exitBalanceToConsume,
			EarliestExitEpoch:             b.earliestExitEpoch,
			ConsolidationBalanceToConsume: b.consolidationBalanceToConsume,
			EarliestConsolidationEpoch:    b.earliestConsolidationEpoch,
			PendingBalanceDeposits:        b.pendingBalanceDepositsVal(),
			PendingPartialWithdrawals:     b.pendingPartialWithdrawalsVal(),
			PendingConsolidations:         b.pendingConsolidationsVal(),
			PreviousInclusionListProposer: b.previousInclusionListProposer,
			PreviousInclusionListSlot:     b.previousInclusionListSlot,
			LatestInclusionListProposer:   b.latestInclusionListProposer,
			LatestInclusionListSlot:       b.latestInclusionListSlot,
			LatestBlockHash:               LatestBlockHashCopy[:],
			LatestFullSlot:                b.latestFullSlot,
			ExecutionPayloadHeader:        b.executionPayloadHeaderVal(),
			LastWithdrawalsRoot:           lastWithdrawalsRootCopy[:],
		}
	default:
		return nil
	}
}

// StateRoots kept track of in the beacon state.
func (b *BeaconState) StateRoots() [][]byte {
	b.lock.RLock()
	defer b.lock.RUnlock()

	roots := b.stateRootsVal()
	if roots == nil {
		return nil
	}
	return roots.Slice()
}

func (b *BeaconState) stateRootsVal() customtypes.StateRoots {
	if features.Get().EnableExperimentalState {
		if b.stateRootsMultiValue == nil {
			return nil
		}
		return b.stateRootsMultiValue.Value(b)
	}
	return b.stateRoots
}

// StateRootAtIndex retrieves a specific state root based on an
// input index value.
func (b *BeaconState) StateRootAtIndex(idx uint64) ([]byte, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if features.Get().EnableExperimentalState {
		if b.stateRootsMultiValue == nil {
			return nil, nil
		}
		r, err := b.stateRootsMultiValue.At(b, idx)
		if err != nil {
			return nil, err
		}
		return r[:], nil
	}

	if b.stateRoots == nil {
		return nil, nil
	}
	r, err := b.stateRootAtIndex(idx)
	if err != nil {
		return nil, err
	}
	return r[:], nil
}

// stateRootAtIndex retrieves a specific state root based on an
// input index value.
// This assumes that a lock is already held on BeaconState.
//
// WARNING: This function does not work with the multi-value slice feature.
func (b *BeaconState) stateRootAtIndex(idx uint64) ([32]byte, error) {
	if uint64(len(b.stateRoots)) <= idx {
		return [32]byte{}, errors.Wrapf(consensus_types.ErrOutOfBounds, "state root index %d does not exist", idx)
	}
	return b.stateRoots[idx], nil
}

// ProtobufBeaconStatePhase0 transforms an input into beacon state in the form of protobuf.
// Error is returned if the input is not type protobuf beacon state.
func ProtobufBeaconStatePhase0(s interface{}) (*ethpb.BeaconState, error) {
	pbState, ok := s.(*ethpb.BeaconState)
	if !ok {
		return nil, errors.New("input is not type ethpb.BeaconState")
	}
	return pbState, nil
}

// ProtobufBeaconStateAltair transforms an input into beacon state Altair in the form of protobuf.
// Error is returned if the input is not type protobuf beacon state.
func ProtobufBeaconStateAltair(s interface{}) (*ethpb.BeaconStateAltair, error) {
	pbState, ok := s.(*ethpb.BeaconStateAltair)
	if !ok {
		return nil, errors.New("input is not type pb.BeaconStateAltair")
	}
	return pbState, nil
}

// ProtobufBeaconStateBellatrix transforms an input into beacon state Bellatrix in the form of protobuf.
// Error is returned if the input is not type protobuf beacon state.
func ProtobufBeaconStateBellatrix(s interface{}) (*ethpb.BeaconStateBellatrix, error) {
	pbState, ok := s.(*ethpb.BeaconStateBellatrix)
	if !ok {
		return nil, errors.New("input is not type pb.BeaconStateBellatrix")
	}
	return pbState, nil
}

// ProtobufBeaconStateCapella transforms an input into beacon state Capella in the form of protobuf.
// Error is returned if the input is not type protobuf beacon state.
func ProtobufBeaconStateCapella(s interface{}) (*ethpb.BeaconStateCapella, error) {
	pbState, ok := s.(*ethpb.BeaconStateCapella)
	if !ok {
		return nil, errors.New("input is not type pb.BeaconStateCapella")
	}
	return pbState, nil
}

// ProtobufBeaconStateDeneb transforms an input into beacon state Deneb in the form of protobuf.
// Error is returned if the input is not type protobuf beacon state.
func ProtobufBeaconStateDeneb(s interface{}) (*ethpb.BeaconStateDeneb, error) {
	pbState, ok := s.(*ethpb.BeaconStateDeneb)
	if !ok {
		return nil, errors.New("input is not type pb.BeaconStateDeneb")
	}
	return pbState, nil
}

// ProtobufBeaconStateElectra transforms an input into beacon state Electra in the form of protobuf.
// Error is returned if the input is not type protobuf beacon state.
func ProtobufBeaconStateElectra(s interface{}) (*ethpb.BeaconStateElectra, error) {
	pbState, ok := s.(*ethpb.BeaconStateElectra)
	if !ok {
		return nil, errors.New("input is not type pb.BeaconStateElectra")
	}
	return pbState, nil
}
