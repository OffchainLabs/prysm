package decoupled

import (
	"context"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Spec: upgrade_to_simplex from the old spec. σ starts as the PDF's initial state (PDF §4) with the fork's latest block as T_h, after the final Gloas interval is settled on a copy.
func UpgradeToDecoupled(ctx context.Context, st state.BeaconState) (state.BeaconState, error) {
	cfg := params.BeaconConfig()
	accounting := st.Copy()
	vp, bp, err := electra.InitializePrecomputeValidators(ctx, accounting)
	if err != nil {
		return nil, err
	}
	vp, bp, err = electra.ProcessEpochParticipation(ctx, accounting, bp, vp)
	if err != nil {
		return nil, err
	}
	accounting, vp, err = electra.ProcessInactivityScores(ctx, accounting, vp)
	if err != nil {
		return nil, err
	}
	accounting, err = electra.ProcessRewardsAndPenaltiesPrecompute(accounting, bp, vp)
	if err != nil {
		return nil, err
	}
	settledScores, err := accounting.InactivityScores()
	if err != nil {
		return nil, err
	}

	pre, ok := st.ToProto().(*ethpb.BeaconStateGloas)
	if !ok {
		return nil, errors.New("pre-state is not a Gloas state")
	}
	epoch := time.CurrentEpoch(st)
	justifiedSlot, err := slots.EpochStart(pre.CurrentJustifiedCheckpoint.Epoch)
	if err != nil {
		return nil, err
	}
	finalizedSlot, err := slots.EpochStart(pre.FinalizedCheckpoint.Epoch)
	if err != nil {
		return nil, err
	}
	n := uint64(len(pre.Validators))
	latestRoot, err := pre.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	payments := make([]*ethpb.BuilderPendingPaymentDecoupled, len(pre.BuilderPendingPayments))
	for i, p := range pre.BuilderPendingPayments {
		payments[i] = &ethpb.BuilderPendingPaymentDecoupled{
			Weight:                  p.Weight,
			AvailableParticipation:  make([]byte, fieldparams.AvailableCommitteeSize/8),
			TimelyHeadParticipation: make([]byte, fieldparams.AvailableCommitteeSize/8),
			Withdrawal:              p.Withdrawal,
		}
	}
	placeholder := make([]*ethpb.AvailableCommittee, uint64(cfg.SlotsPerEpoch)*uint64(2+cfg.MinSeedLookahead))
	for i := range placeholder {
		placeholder[i] = &ethpb.AvailableCommittee{ValidatorIndices: make([]primitives.ValidatorIndex, fieldparams.AvailableCommitteeSize)}
	}
	post := &ethpb.BeaconStateDecoupled{
		GenesisTime:                   pre.GenesisTime,
		GenesisValidatorsRoot:         pre.GenesisValidatorsRoot,
		Slot:                          pre.Slot,
		Fork:                          &ethpb.Fork{PreviousVersion: pre.Fork.CurrentVersion, CurrentVersion: cfg.DecoupledForkVersion, Epoch: epoch},
		LatestBlockHeader:             pre.LatestBlockHeader,
		BlockRoots:                    pre.BlockRoots,
		StateRoots:                    pre.StateRoots,
		HistoricalRoots:               pre.HistoricalRoots,
		Eth1Data:                      pre.Eth1Data,
		Eth1DataVotes:                 pre.Eth1DataVotes,
		Eth1DepositIndex:              pre.Eth1DepositIndex,
		Validators:                    pre.Validators,
		Balances:                      accounting.Balances(),
		RandaoMixes:                   pre.RandaoMixes,
		Slashings:                     pre.Slashings,
		PreviousRoundParticipation:    make([]byte, n),
		CurrentRoundParticipation:     make([]byte, n),
		JustifiedCheckpoint:           &ethpb.CheckpointDecoupled{Slot: justifiedSlot, Root: pre.CurrentJustifiedCheckpoint.Root},
		FinalizedCheckpoint:           &ethpb.CheckpointDecoupled{Slot: finalizedSlot, Root: pre.FinalizedCheckpoint.Root},
		InactivityScores:              settledScores,
		CurrentSyncCommittee:          pre.CurrentSyncCommittee,
		NextSyncCommittee:             pre.NextSyncCommittee,
		LatestExecutionPayloadBid:     pre.LatestExecutionPayloadBid,
		NextWithdrawalIndex:           pre.NextWithdrawalIndex,
		NextWithdrawalValidatorIndex:  pre.NextWithdrawalValidatorIndex,
		HistoricalSummaries:           pre.HistoricalSummaries,
		DepositRequestsStartIndex:     pre.DepositRequestsStartIndex,
		DepositBalanceToConsume:       pre.DepositBalanceToConsume,
		ExitBalanceToConsume:          pre.ExitBalanceToConsume,
		EarliestExitEpoch:             pre.EarliestExitEpoch,
		ConsolidationBalanceToConsume: pre.ConsolidationBalanceToConsume,
		EarliestConsolidationEpoch:    pre.EarliestConsolidationEpoch,
		PendingDeposits:               pre.PendingDeposits,
		PendingPartialWithdrawals:     pre.PendingPartialWithdrawals,
		PendingConsolidations:         pre.PendingConsolidations,
		ProposerLookahead:             pre.ProposerLookahead,
		Builders:                      pre.Builders,
		NextWithdrawalBuilderIndex:    pre.NextWithdrawalBuilderIndex,
		ExecutionPayloadAvailability:  pre.ExecutionPayloadAvailability,
		BuilderPendingPayments:        payments,
		BuilderPendingWithdrawals:     pre.BuilderPendingWithdrawals,
		LatestBlockHash:               pre.LatestBlockHash,
		PayloadExpectedWithdrawals:    pre.PayloadExpectedWithdrawals,
		PtcWindow:                     pre.PtcWindow,
		AvailableCommitteeWindow:      placeholder,
		JustifiedHeight:               0,
		FinalizedHeight:               0,
		CurrentHeight:                 cfg.GenesisHeight,
		CurrentHeightNonjustifiable:   false,
		CurrentHeightTarget:           &ethpb.CheckpointDecoupled{Slot: pre.LatestBlockHeader.Slot, Root: latestRoot[:]},
		TargetParticipation:           bitfield.NewBitlist(n),
		Progress:                      bitfield.NewBitlist(n),
		FinalityParticipation:         bitfield.NewBitlist(n),
	}
	s, err := state_native.InitializeFromProtoUnsafeDecoupled(post)
	if err != nil {
		return nil, errors.Wrap(err, "could not initialize Decoupled state")
	}
	window, err := InitializeAvailableCommitteeWindow(ctx, s)
	if err != nil {
		return nil, errors.Wrap(err, "could not initialize available committee window")
	}
	if err := s.SetAvailableCommitteeWindow(window); err != nil {
		return nil, err
	}
	return s, nil
}
