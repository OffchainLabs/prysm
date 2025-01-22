package fulu

import (
	"sort"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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

	earliestExitEpoch := helpers.ActivationExitEpoch(time.CurrentEpoch(beaconState))
	preActivationIndices := make([]primitives.ValidatorIndex, 0)
	compoundWithdrawalIndices := make([]primitives.ValidatorIndex, 0)
	if err = beaconState.ReadFromEveryValidator(func(index int, val state.ReadOnlyValidator) error {
		if val.ExitEpoch() != params.BeaconConfig().FarFutureEpoch && val.ExitEpoch() > earliestExitEpoch {
			earliestExitEpoch = val.ExitEpoch()
		}
		if val.ActivationEpoch() == params.BeaconConfig().FarFutureEpoch {
			preActivationIndices = append(preActivationIndices, primitives.ValidatorIndex(index))
		}
		if val.HasCompoundingWithdrawalCredentials() {
			compoundWithdrawalIndices = append(compoundWithdrawalIndices, primitives.ValidatorIndex(index))
		}
		return nil
	}); err != nil {
		return nil, err
	}

	earliestExitEpoch++ // Increment to find the earliest possible exit epoch

	// note: should be the same in prestate and post beaconState.
	// we are deviating from the specs a bit as it calls for using the post beaconState
	tab, err := helpers.TotalActiveBalance(beaconState)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get total active balance")
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
		DepositBalanceToConsume:       0,
		ExitBalanceToConsume:          helpers.ActivationExitChurnLimit(primitives.Gwei(tab)),
		EarliestExitEpoch:             earliestExitEpoch,
		ConsolidationBalanceToConsume: helpers.ConsolidationChurnLimit(primitives.Gwei(tab)),
		EarliestConsolidationEpoch:    helpers.ActivationExitEpoch(slots.ToEpoch(beaconState.Slot())),
		PendingDeposits:               make([]*ethpb.PendingDeposit, 0),
		PendingPartialWithdrawals:     make([]*ethpb.PendingPartialWithdrawal, 0),
		PendingConsolidations:         make([]*ethpb.PendingConsolidation, 0),
	}

	// Sorting preActivationIndices based on a custom criteria
	sort.Slice(preActivationIndices, func(i, j int) bool {
		// Comparing based on ActivationEligibilityEpoch and then by index if the epochs are the same
		if s.Validators[preActivationIndices[i]].ActivationEligibilityEpoch == s.Validators[preActivationIndices[j]].ActivationEligibilityEpoch {
			return preActivationIndices[i] < preActivationIndices[j]
		}
		return s.Validators[preActivationIndices[i]].ActivationEligibilityEpoch < s.Validators[preActivationIndices[j]].ActivationEligibilityEpoch
	})

	// Need to cast the beaconState to use in helper functions
	post, err := state_native.InitializeFromProtoUnsafeFulu(s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize post fulu beaconState")
	}

	return post, nil
}
