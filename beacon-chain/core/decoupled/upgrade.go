package decoupled

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Spec: the initial σ with the Gloas finalized checkpoint as its genesis, height 1 with entry, J and F all that block. The justified checkpoint mirrors J from here on.
func UpgradeToDecoupled(st state.BeaconState) (state.BeaconState, error) {
	pre, ok := st.ToProto().(*ethpb.BeaconStateGloas)
	if !ok {
		return nil, errors.New("pre-state is not a Gloas state")
	}
	finalizedSlot, err := slots.EpochStart(pre.FinalizedCheckpoint.Epoch)
	if err != nil {
		return nil, err
	}
	genesis := &ethpb.CheckpointDecoupled{Slot: finalizedSlot, Root: bytesutil.SafeCopyBytes(pre.FinalizedCheckpoint.Root)}
	n := uint64(len(pre.Validators))
	post := &ethpb.BeaconStateDecoupled{
		GenesisTime:           pre.GenesisTime,
		GenesisValidatorsRoot: pre.GenesisValidatorsRoot,
		Slot:                  pre.Slot,
		Fork: &ethpb.Fork{
			PreviousVersion: pre.Fork.CurrentVersion,
			CurrentVersion:  params.BeaconConfig().DecoupledForkVersion,
			Epoch:           slots.ToEpoch(pre.Slot),
		},
		LatestBlockHeader:             pre.LatestBlockHeader,
		BlockRoots:                    pre.BlockRoots,
		StateRoots:                    pre.StateRoots,
		HistoricalRoots:               pre.HistoricalRoots,
		Eth1Data:                      pre.Eth1Data,
		Eth1DataVotes:                 pre.Eth1DataVotes,
		Eth1DepositIndex:              pre.Eth1DepositIndex,
		Validators:                    pre.Validators,
		Balances:                      pre.Balances,
		RandaoMixes:                   pre.RandaoMixes,
		Slashings:                     pre.Slashings,
		PreviousEpochParticipation:    pre.PreviousEpochParticipation,
		CurrentEpochParticipation:     pre.CurrentEpochParticipation,
		JustificationBits:             pre.JustificationBits,
		PreviousJustifiedCheckpoint:   pre.PreviousJustifiedCheckpoint,
		CurrentJustifiedCheckpoint:    pre.FinalizedCheckpoint.Copy(),
		FinalizedCheckpoint:           pre.FinalizedCheckpoint,
		InactivityScores:              pre.InactivityScores,
		CurrentSyncCommittee:          pre.CurrentSyncCommittee,
		NextSyncCommittee:             pre.NextSyncCommittee,
		LatestBlockHash:               pre.LatestBlockHash,
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
		BuilderPendingPayments:        pre.BuilderPendingPayments,
		BuilderPendingWithdrawals:     pre.BuilderPendingWithdrawals,
		LatestExecutionPayloadBid:     pre.LatestExecutionPayloadBid,
		PayloadExpectedWithdrawals:    pre.PayloadExpectedWithdrawals,
		PtcWindow:                     pre.PtcWindow,
		Height:                        1,
		Entry:                         genesis,
		TargetParticipation:           bitfield.NewBitlist(n),
		Progress:                      bitfield.NewBitlist(n),
		Justified:                     genesis.Copy(),
		Finalized:                     genesis.Copy(),
	}
	return state_native.InitializeFromProtoUnsafeDecoupled(post)
}
