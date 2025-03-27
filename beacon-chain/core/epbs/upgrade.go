package epbs

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// UpgradeToEIP7732 updates inputs a generic state to return the version EIP-7732 state.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/_features/eip7732/fork.md
func UpgradeToEIP7732(beaconState state.BeaconState) (state.BeaconState, error) {
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
	historicalRoots := beaconState.HistoricalRoots()
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

	s := &ethpb.BeaconStateEPBS{
		GenesisTime:           uint64(beaconState.GenesisTime().Unix()),
		GenesisValidatorsRoot: beaconState.GenesisValidatorsRoot(),
		Slot:                  beaconState.Slot(),
		Fork: &ethpb.Fork{
			PreviousVersion: beaconState.Fork().CurrentVersion,
			CurrentVersion:  params.BeaconConfig().EPBSForkVersion,
			Epoch:           time.CurrentEpoch(beaconState),
		},
		LatestBlockHeader:            beaconState.LatestBlockHeader(),
		BlockRoots:                   beaconState.BlockRoots(),
		StateRoots:                   beaconState.StateRoots(),
		HistoricalRoots:              historicalRoots,
		Eth1Data:                     beaconState.Eth1Data(),
		Eth1DataVotes:                beaconState.Eth1DataVotes(),
		Eth1DepositIndex:             beaconState.Eth1DepositIndex(),
		Validators:                   beaconState.Validators(),
		Balances:                     beaconState.Balances(),
		RandaoMixes:                  beaconState.RandaoMixes(),
		Slashings:                    beaconState.Slashings(),
		PreviousEpochParticipation:   prevEpochParticipation,
		CurrentEpochParticipation:    currentEpochParticipation,
		JustificationBits:            beaconState.JustificationBits(),
		PreviousJustifiedCheckpoint:  beaconState.PreviousJustifiedCheckpoint(),
		CurrentJustifiedCheckpoint:   beaconState.CurrentJustifiedCheckpoint(),
		FinalizedCheckpoint:          beaconState.FinalizedCheckpoint(),
		InactivityScores:             inactivityScores,
		CurrentSyncCommittee:         currentSyncCommittee,
		NextSyncCommittee:            nextSyncCommittee,
		NextWithdrawalIndex:          wi,
		NextWithdrawalValidatorIndex: vi,
		HistoricalSummaries:          summaries,

		DepositRequestsStartIndex:     params.BeaconConfig().UnsetDepositRequestsStartIndex,
		DepositBalanceToConsume:       depositBalanceToConsume,
		ExitBalanceToConsume:          exitBalanceToConsume,
		EarliestExitEpoch:             earliestExitEpoch,
		ConsolidationBalanceToConsume: consolidationBalanceToConsume,
		EarliestConsolidationEpoch:    earliestConsolidationEpoch,
		PendingDeposits:               pendingDeposits,
		PendingPartialWithdrawals:     pendingPartialWithdrawals,
		PendingConsolidations:         pendingConsolidations,

		// Newly added for EIP7732
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderEPBS{
			ParentBlockHash:        make([]byte, 32),
			ParentBlockRoot:        make([]byte, 32),
			BlockHash:              make([]byte, 32),
			BlobKzgCommitmentsRoot: make([]byte, 32),
		},
		LatestBlockHash:       payloadHeader.BlockHash(),
		LatestFullSlot:        beaconState.Slot(),
		LatestWithdrawalsRoot: make([]byte, 32),
	}

	post, err := state_native.InitializeFromProtoUnsafeEpbs(s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize post EIP-7732 beaconState")
	}

	return post, nil
}
