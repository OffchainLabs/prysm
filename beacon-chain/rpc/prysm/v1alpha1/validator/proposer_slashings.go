package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	v "github.com/OffchainLabs/prysm/v6/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

func (vs *Server) getSlashings(ctx context.Context, head state.BeaconState) ([]*ethpb.ProposerSlashing, []ethpb.AttSlashing) {
	var err error
	proposerSlashings := vs.SlashingsPool.PendingProposerSlashings(ctx, head, false /*noLimit*/)
	attSlashings := vs.SlashingsPool.PendingAttesterSlashings(ctx, head, false /*noLimit*/)
	validProposerSlashings := make([]*ethpb.ProposerSlashing, 0, len(proposerSlashings))
	validAttSlashings := make([]ethpb.AttSlashing, 0, len(attSlashings))
	if len(proposerSlashings) == 0 && len(attSlashings) == 0 {
		return validProposerSlashings, validAttSlashings
	}
	// ExitInformation is expensive to compute, only do it if we need it.
	exitInfo := v.ExitInformation(head)
	if err := helpers.UpdateTotalActiveBalanceCache(head, exitInfo.TotalActiveBalance); err != nil {
		log.WithError(err).Warn("Could not update total active balance cache")
	}
	for _, slashing := range proposerSlashings {
		_, err = blocks.ProcessProposerSlashing(ctx, head, slashing, exitInfo)
		if err != nil {
			log.WithError(err).Warn("Could not validate proposer slashing for block inclusion")
			continue
		}
		validProposerSlashings = append(validProposerSlashings, slashing)
	}
	for _, slashing := range attSlashings {
		_, err = blocks.ProcessAttesterSlashing(ctx, head, slashing, exitInfo)
		if err != nil {
			log.WithError(err).Warn("Could not validate attester slashing for block inclusion")
			continue
		}
		validAttSlashings = append(validAttSlashings, slashing)
	}
	return validProposerSlashings, validAttSlashings
}
