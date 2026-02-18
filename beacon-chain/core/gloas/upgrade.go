package gloas

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// UpgradeToGloas returns the Gloas state based on the generic input state.
// https://github.com/ethereum/consensus-specs/blob/master/specs/gloas/fork.md#upgrading-the-state
func UpgradeToGloas(beaconState state.BeaconState) (state.BeaconState, error) {
	s, err := ConvertToGloas(beaconState)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert to gloas")
	}
	return s, nil
}

func ConvertToGloas(beaconState state.BeaconState) (state.BeaconState, error) {
	currentSyncCommittee, err := beaconState.CurrentSyncCommittee()
	if err != nil {
		return nil, err
	}
	nextSyncCommittee, err := beaconState.NextSyncCommittee()
	if err != nil {
		return nil, err
	}
	prevEpochParticipation, err := beaconState.PreviousEpochParticipation()
	if err != nil {
		return nil, err
	}
	currentEpochParticipation, err := beaconState.CurrentEpochParticipation()
	if err != nil {
		return nil, err
	}
	inactivityScores, err := beaconState.InactivityScores()
	if err != nil {
		return nil, err
	}
	payloadHeader, err := beaconState.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, err
	}
	wi, err := beaconState.NextWithdrawalIndex()
	if err != nil {
		return nil, err
	}
	vi, err := beaconState.NextWithdrawalValidatorIndex()
	if err != nil {
		return nil, err
	}
	summaries, err := beaconState.HistoricalSummaries()
	if err != nil {
		return nil, err
	}
	depositRequestsStartIndex, err := beaconState.DepositRequestsStartIndex()
	if err != nil {
		return nil, err
	}
	depositBalanceToConsume, err := beaconState.DepositBalanceToConsume()
	if err != nil {
		return nil, err
	}
	exitBalanceToConsume, err := beaconState.ExitBalanceToConsume()
	if err != nil {
		return nil, err
	}
	earliestExitEpoch, err := beaconState.EarliestExitEpoch()
	if err != nil {
		return nil, err
	}
	consolidationBalanceToConsume, err := beaconState.ConsolidationBalanceToConsume()
	if err != nil {
		return nil, err
	}
	earliestConsolidationEpoch, err := beaconState.EarliestConsolidationEpoch()
	if err != nil {
		return nil, err
	}
	pendingDeposits, err := beaconState.PendingDeposits()
	if err != nil {
		return nil, err
	}
	pendingPartialWithdrawals, err := beaconState.PendingPartialWithdrawals()
	if err != nil {
		return nil, err
	}
	pendingConsolidations, err := beaconState.PendingConsolidations()
	if err != nil {
		return nil, err
	}
	pl, err := beaconState.ProposerLookahead()
	if err != nil {
		return nil, err
	}
	var proposerLookahead []uint64
	for _, v := range pl {
		proposerLookahead = append(proposerLookahead, uint64(v))
	}
	cfg := params.BeaconConfig()
	executionPayloadAvailability := make([]byte, cfg.SlotsPerHistoricalRoot/8)
	for i := range executionPayloadAvailability {
		executionPayloadAvailability[i] = 0xFF
	}
	s := &ethpb.BeaconStateGloas{
		GenesisTime:           uint64(beaconState.GenesisTime().Unix()),
		GenesisValidatorsRoot: beaconState.GenesisValidatorsRoot(),
		Slot:                  beaconState.Slot(),
		Fork: &ethpb.Fork{
			PreviousVersion: beaconState.Fork().CurrentVersion,
			CurrentVersion:  params.BeaconConfig().GloasForkVersion,
			Epoch:           time.CurrentEpoch(beaconState),
		},
		LatestBlockHeader:           beaconState.LatestBlockHeader(),
		BlockRoots:                  beaconState.BlockRoots(),
		StateRoots:                  beaconState.StateRoots(),
		HistoricalRoots:             beaconState.HistoricalRoots(),
		Eth1Data:                    beaconState.Eth1Data(),
		Eth1DataVotes:               beaconState.Eth1DataVotes(),
		Eth1DepositIndex:            beaconState.Eth1DepositIndex(),
		Validators:                  beaconState.Validators(),
		Balances:                    beaconState.Balances(),
		RandaoMixes:                 beaconState.RandaoMixes(),
		Slashings:                   beaconState.Slashings(),
		PreviousEpochParticipation:  prevEpochParticipation,
		CurrentEpochParticipation:   currentEpochParticipation,
		JustificationBits:           beaconState.JustificationBits(),
		PreviousJustifiedCheckpoint: beaconState.PreviousJustifiedCheckpoint(),
		CurrentJustifiedCheckpoint:  beaconState.CurrentJustifiedCheckpoint(),
		FinalizedCheckpoint:         beaconState.FinalizedCheckpoint(),
		InactivityScores:            inactivityScores,
		CurrentSyncCommittee:        currentSyncCommittee,
		NextSyncCommittee:           nextSyncCommittee,

		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
			ParentBlockHash: make([]byte, 32),
			ParentBlockRoot: make([]byte, 32),
			BlockHash:       payloadHeader.BlockHash(),
			PrevRandao:      make([]byte, 32),
			FeeRecipient:    make([]byte, 20),
		},
		NextWithdrawalIndex:           wi,
		NextWithdrawalValidatorIndex:  vi,
		HistoricalSummaries:           summaries,
		DepositRequestsStartIndex:     depositRequestsStartIndex,
		DepositBalanceToConsume:       depositBalanceToConsume,
		ExitBalanceToConsume:          exitBalanceToConsume,
		EarliestExitEpoch:             earliestExitEpoch,
		ConsolidationBalanceToConsume: consolidationBalanceToConsume,
		EarliestConsolidationEpoch:    earliestConsolidationEpoch,
		PendingDeposits:               pendingDeposits,
		PendingPartialWithdrawals:     pendingPartialWithdrawals,
		PendingConsolidations:         pendingConsolidations,
		ProposerLookahead:             proposerLookahead,

		Builders:                     make([]*ethpb.Builder, 0),
		NextWithdrawalBuilderIndex:   primitives.BuilderIndex(0),
		ExecutionPayloadAvailability: executionPayloadAvailability,
		BuilderPendingPayments:       make([]*ethpb.BuilderPendingPayment, cfg.SlotsPerEpoch*2),
		BuilderPendingWithdrawals:    make([]*ethpb.BuilderPendingWithdrawal, 0),
		LatestBlockHash:              payloadHeader.BlockHash(),
		PayloadExpectedWithdrawals:   make([]*enginev1.Withdrawal, 0),
	}
	return state_native.InitializeFromProtoUnsafeGloas(s)
}
