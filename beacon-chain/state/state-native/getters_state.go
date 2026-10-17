package state_native

import (
	"fmt"

	customtypes "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/custom-types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// ToProtoUnsafe returns the pointer value of the underlying
// beacon state proto object, bypassing immutability. Use with care.
func (b *BeaconState) ToProtoUnsafe() any {
	if b == nil {
		return nil
	}

	gvrCopy := b.genesisValidatorsRoot
	br := b.blockRootsVal().Slice()
	sr := b.stateRootsVal().Slice()
	rm := b.randaoMixesVal().Slice()
	var vals []*ethpb.Validator
	var bals []uint64
	var inactivityScores []uint64

	if b.balancesMultiValue != nil {
		bals = b.balancesMultiValue.Value(b)
	}
	if b.inactivityScoresMultiValue != nil {
		inactivityScores = b.inactivityScoresMultiValue.Value(b)
	}
	if b.validatorsMultiValue != nil {
		vals = stateutil.CompactValidatorsToProto(b.validatorsMultiValue.Value(b))
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
			InactivityScores:            inactivityScores,
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
			InactivityScores:             inactivityScores,
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
			InactivityScores:             inactivityScores,
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
			InactivityScores:             inactivityScores,
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
			InactivityScores:              inactivityScores,
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
	case version.Fulu:

		return &ethpb.BeaconStateFulu{
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
			InactivityScores:              inactivityScores,
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
			ProposerLookahead:             b.proposerLookahead,
		}
	case version.Gloas:

		return &ethpb.BeaconStateGloas{
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
			InactivityScores:              inactivityScores,
			CurrentSyncCommittee:          b.currentSyncCommittee,
			NextSyncCommittee:             b.nextSyncCommittee,
			LatestExecutionPayloadBid:     b.latestExecutionPayloadBid,
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
			ProposerLookahead:             b.proposerLookahead,
			ExecutionPayloadAvailability:  b.executionPayloadAvailability,
			Builders:                      b.builders,
			NextWithdrawalBuilderIndex:    b.nextWithdrawalBuilderIndex,
			BuilderPendingPayments:        b.builderPendingPayments,
			BuilderPendingWithdrawals:     b.builderPendingWithdrawals,
			LatestBlockHash:               b.latestBlockHash,
			PayloadExpectedWithdrawals:    b.payloadExpectedWithdrawals,
			PtcWindow:                     b.ptcWindow,
		}
	default:
		return nil
	}
}

// ToProto the beacon state into a protobuf for usage.
func (b *BeaconState) ToProto() any {
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
	case version.Fulu:

		return &ethpb.BeaconStateFulu{
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
			ProposerLookahead:             b.proposerLookaheadVal(),
		}
	case version.Gloas:

		return &ethpb.BeaconStateGloas{
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
			LatestExecutionPayloadBid:     b.latestExecutionPayloadBid.Copy(),
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
			ProposerLookahead:             b.proposerLookaheadVal(),
			ExecutionPayloadAvailability:  b.executionPayloadAvailabilityVal(),
			Builders:                      b.buildersVal(),
			NextWithdrawalBuilderIndex:    b.nextWithdrawalBuilderIndex,
			BuilderPendingPayments:        b.builderPendingPaymentsVal(),
			BuilderPendingWithdrawals:     b.builderPendingWithdrawalsVal(),
			LatestBlockHash:               b.latestBlockHashVal(),
			PayloadExpectedWithdrawals:    b.payloadExpectedWithdrawalsVal(),
			PtcWindow:                     b.ptcWindowVal(),
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
	if b.stateRootsMultiValue == nil {
		return nil
	}
	return b.stateRootsMultiValue.Value(b)
}

// StateRootAtIndex retrieves a specific state root based on an
// input index value.
func (b *BeaconState) StateRootAtIndex(idx uint64) ([]byte, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.stateRootsMultiValue == nil {
		return nil, nil
	}
	r, err := b.stateRootsMultiValue.At(b, idx)
	if err != nil {
		return nil, err
	}
	return r[:], nil
}

// ContainerFrom returns v as the state container type T, such as the result of
// ToProto or ToProtoUnsafe. It returns an error if v is not a T.
func ContainerFrom[T Container](v any) (T, error) {
	c, ok := v.(T)
	if !ok {
		return c, fmt.Errorf("input is %T, not %T", v, c)
	}
	return c, nil
}
