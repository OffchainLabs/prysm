package slasher

import (
	"context"

	"github.com/pkg/errors"
	slashertypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/slasher/types"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// detectProposerSlashings takes in signed block header wrappers and returns a list of proposer slashings detected.
func (s *Service) detectProposerSlashings(
	ctx context.Context,
	incomingProposals []*slashertypes.SignedBlockHeaderWrapper,
) ([]*ethpb.ProposerSlashing, error) {
	ctx, span := trace.StartSpan(ctx, "slasher.detectProposerSlashings")
	defer span.End()

	// internalSlashings will contain any slashable double proposals in the input list
	// of proposals with respect to each other.
	internalSlashings := []*ethpb.ProposerSlashing{}

	existingProposals := make(map[string]*slashertypes.SignedBlockHeaderWrapper)

	// We check if there are any slashable double proposals in the input list
	// of proposals with respect to each other.
	for _, incomingProposal := range incomingProposals {
		key := proposalKey(incomingProposal)
		existingProposal, ok := existingProposals[key]

		// If we have not seen this proposal before, we add it to our map of existing proposals
		// and we continue to the next proposal.
		if !ok {
			existingProposals[key] = incomingProposal
			continue
		}

		// If we have seen this proposal before, we check if it is a double proposal.
		if isDoubleProposal(incomingProposal.HeaderRoot, existingProposal.HeaderRoot) {
			doubleProposalsTotal.Inc()

			slashing := &ethpb.ProposerSlashing{
				Header_1: existingProposal.SignedBeaconBlockHeader,
				Header_2: incomingProposal.SignedBeaconBlockHeader,
			}

			internalSlashings = append(internalSlashings, slashing)
		}
	}

	// We check if there are any slashable double proposals in the input list
	// of proposals with respect to the slasher database.
	databaseSlashings, err := s.serviceCfg.Database.CheckDoubleBlockProposals(ctx, incomingProposals)
	if err != nil {
		return nil, errors.Wrap(err, "could not check for double proposals on disk")
	}

	// We save the safe proposals (with respect to the database) to our database.
	// If some proposals in incomingProposals are slashable with respect to each other,
	// we (arbitrarily) save the last one to the database.
	if err := s.serviceCfg.Database.SaveBlockProposals(ctx, incomingProposals); err != nil {
		return nil, errors.Wrap(err, "could not save safe proposals")
	}

	// totalSlashings contain all slashings we have detected.
	totalSlashings := append(internalSlashings, databaseSlashings...)
	return totalSlashings, nil
}

// proposalKey build a key which is a combination of the slot and the proposer index.
// If a validator proposes several blocks for the same slot, then several (potentially slashable)
// proposals will correspond to the same key.
func proposalKey(proposal *slashertypes.SignedBlockHeaderWrapper) string {
	header := proposal.SignedBeaconBlockHeader.Header

	slotKey := uintToString(uint64(header.Slot))
	proposerIndexKey := uintToString(uint64(header.ProposerIndex))

	return slotKey + ":" + proposerIndexKey
}
