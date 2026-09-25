package decoupled

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/pkg/errors"
)

// Spec: process_epoch. Justification, inactivity, rewards and participation rotation moved to process_round and the height outcomes.
func ProcessEpoch(ctx context.Context, st state.BeaconState) error {
	ctx, span := trace.StartSpan(ctx, "decoupled.ProcessEpoch")
	defer span.End()
	if st == nil || st.IsNil() {
		return errors.New("nil state")
	}
	if err := electra.ProcessRegistryUpdates(ctx, st); err != nil {
		return errors.Wrap(err, "could not process registry updates")
	}
	if err := electra.ProcessSlashings(ctx, st); err != nil {
		return err
	}
	st, err := electra.ProcessEth1DataReset(st)
	if err != nil {
		return err
	}
	activeBalance, err := helpers.TotalActiveBalance(ctx, st)
	if err != nil {
		return err
	}
	if err := electra.ProcessPendingDeposits(ctx, st, primitives.Gwei(activeBalance)); err != nil {
		return err
	}
	if err := electra.ProcessPendingConsolidations(ctx, st); err != nil {
		return err
	}
	if err := ProcessBuilderPendingPayments(ctx, st); err != nil {
		return err
	}
	if err := electra.ProcessEffectiveBalanceUpdates(st); err != nil {
		return err
	}
	if st, err = electra.ProcessSlashingsReset(st); err != nil {
		return err
	}
	if st, err = electra.ProcessRandaoMixesReset(st); err != nil {
		return err
	}
	if st, err = electra.ProcessHistoricalDataUpdate(st); err != nil {
		return err
	}
	if _, err = electra.ProcessSyncCommitteeUpdates(ctx, st); err != nil {
		return err
	}
	if err := gloas.ProcessProposerLookahead(ctx, st); err != nil {
		return err
	}
	if err := gloas.ProcessPTCWindow(ctx, st); err != nil {
		return err
	}
	return ProcessAvailableCommitteeWindow(ctx, st)
}
