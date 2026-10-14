package decoupled

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

// Gloas epoch processing minus justification, inactivity and rewards, which the FG replaces or the paper omits.
func ProcessEpoch(ctx context.Context, st state.BeaconState) error {
	activeBalance, err := helpers.TotalActiveBalance(ctx, st)
	if err != nil {
		return err
	}
	if err := electra.ProcessRegistryUpdates(ctx, st); err != nil {
		return errors.Wrap(err, "could not process registry updates")
	}
	if err := electra.ProcessSlashings(ctx, st); err != nil {
		return err
	}
	if _, err := electra.ProcessEth1DataReset(st); err != nil {
		return err
	}
	if err := electra.ProcessPendingDeposits(ctx, st, primitives.Gwei(activeBalance)); err != nil {
		return err
	}
	if err := electra.ProcessPendingConsolidations(ctx, st); err != nil {
		return err
	}
	if err := gloas.ProcessBuilderPendingPayments(ctx, st); err != nil {
		return err
	}
	if err := electra.ProcessEffectiveBalanceUpdates(st); err != nil {
		return err
	}
	if _, err := electra.ProcessSlashingsReset(st); err != nil {
		return err
	}
	if _, err := electra.ProcessRandaoMixesReset(st); err != nil {
		return err
	}
	if _, err := electra.ProcessHistoricalDataUpdate(st); err != nil {
		return err
	}
	if _, err := electra.ProcessParticipationFlagUpdates(st); err != nil {
		return err
	}
	if _, err := electra.ProcessSyncCommitteeUpdates(ctx, st); err != nil {
		return err
	}
	if err := gloas.ProcessProposerLookahead(ctx, st); err != nil {
		return err
	}
	return gloas.ProcessPTCWindow(ctx, st)
}
