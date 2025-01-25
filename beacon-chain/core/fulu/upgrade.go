package fulu

import (
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// UpgradeToFulu updates inputs a generic state to return the version Fulu state.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/fulu/fork.md#upgrading-the-state
func UpgradeToFulu(beaconState state.BeaconState) (state.BeaconState, error) {
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
	txRoot, err := payloadHeader.TransactionsRoot()
	if err != nil {
		return nil, err
	}
	wdRoot, err := payloadHeader.WithdrawalsRoot()
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
	historicalRoots, err := beaconState.HistoricalRoots()
	if err != nil {
		return nil, err
	}
	excessBlobGas, err := payloadHeader.ExcessBlobGas()
	if err != nil {
		return nil, err
	}
	blobGasUsed, err := payloadHeader.BlobGasUsed()
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

	s := &ethpb.BeaconStateFulu{
		GenesisTime:           beaconState.GenesisTime(),
		GenesisValidatorsRoot: beaconState.GenesisValidatorsRoot(),
		Slot:                  beaconState.Slot(),
		Fork: &ethpb.Fork{
			PreviousVersion: beaconState.Fork().CurrentVersion,
			CurrentVersion:  params.BeaconConfig().FuluForkVersion,
			Epoch:           time.CurrentEpoch(beaconState),
		},
		LatestBlockHeader:           beaconState.LatestBlockHeader(),
		BlockRoots:                  beaconState.BlockRoots(),
		StateRoots:                  beaconState.StateRoots(),
		HistoricalRoots:             historicalRoots,
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
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderDeneb{
			ParentHash:       payloadHeader.ParentHash(),
			FeeRecipient:     payloadHeader.FeeRecipient(),
			StateRoot:        payloadHeader.StateRoot(),
			ReceiptsRoot:     payloadHeader.ReceiptsRoot(),
			LogsBloom:        payloadHeader.LogsBloom(),
			PrevRandao:       payloadHeader.PrevRandao(),
			BlockNumber:      payloadHeader.BlockNumber(),
			GasLimit:         payloadHeader.GasLimit(),
			GasUsed:          payloadHeader.GasUsed(),
			Timestamp:        payloadHeader.Timestamp(),
			ExtraData:        payloadHeader.ExtraData(),
			BaseFeePerGas:    payloadHeader.BaseFeePerGas(),
			BlockHash:        payloadHeader.BlockHash(),
			TransactionsRoot: txRoot,
			WithdrawalsRoot:  wdRoot,
			ExcessBlobGas:    excessBlobGas,
			BlobGasUsed:      blobGasUsed,
		},
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
	}

	// Need to cast the beaconState to use in helper functions
	post, err := state_native.InitializeFromProtoUnsafeFulu(s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize post fulu beaconState")
	}

	return post, nil
}
