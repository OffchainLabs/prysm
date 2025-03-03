package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	v "github.com/OffchainLabs/prysm/v6/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

func (vs *Server) getSlashings(ctx context.Context, head state.BeaconState) ([]*ethpb.ProposerSlashing, []ethpb.AttSlashing) {
	proposerSlashings := vs.SlashingsPool.PendingProposerSlashings(ctx, head, false /*noLimit*/)
	attSlashings := vs.SlashingsPool.PendingAttesterSlashings(ctx, head, false /*noLimit*/)

	if len(proposerSlashings) == 0 && len(attSlashings) == 0 {
		return []*ethpb.ProposerSlashing{}, []ethpb.AttSlashing{}
	}

	exitInfo := v.ExitInformation(head)
	validProposerSlashings := make([]*ethpb.ProposerSlashing, 0, len(proposerSlashings))
	for _, slashing := range proposerSlashings {
		_, _, err := blocks.ProcessProposerSlashing(ctx, head, slashing, v.SlashValidator, exitInfo)
		if err != nil {
			log.WithError(err).Warn("Could not validate proposer slashing for block inclusion")
			continue
		}
		validProposerSlashings = append(validProposerSlashings, slashing)
	}
	validAttSlashings := make([]ethpb.AttSlashing, 0, len(attSlashings))
	for _, slashing := range attSlashings {
		_, _, err := blocks.ProcessAttesterSlashing(ctx, head, slashing, v.SlashValidator, exitInfo)
		if err != nil {
			log.WithError(err).Warn("Could not validate attester slashing for block inclusion")
			continue
		}
		validAttSlashings = append(validAttSlashings, slashing)
	}
	return validProposerSlashings, validAttSlashings
}
