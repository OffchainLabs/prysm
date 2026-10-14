package validator

import (
	"errors"
	"fmt"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/decoupled"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Spec: propose_block, B.attestations from proposal_attestations, B.votes and B.support_votes from ac_proposal.
func (vs *Server) setDecoupledFields(sBlk interfaces.SignedBeaconBlock, head state.BeaconState) error {
	if err := vs.setDecoupledAttestations(sBlk, head); err != nil {
		return fmt.Errorf("decoupled attestations: %w", err)
	}
	if err := vs.setAvailableAttestations(sBlk, head); err != nil {
		return fmt.Errorf("available attestations: %w", err)
	}
	return nil
}

// Spec: proposal_attestations. Rows in first-acceptance order, valid against the parent σ at the proposal slot, each setting a bit still unset in one working copy so overlapping aggregates cost nothing. Rows go in whole, never truncated.
func (vs *Server) setDecoupledAttestations(sBlk interfaces.SignedBeaconBlock, head state.BeaconState) error {
	p, err := decoupled.NewParticipation(head)
	if err != nil {
		return err
	}
	atts := vs.DecoupledFGPool.Select(func(a *ethpb.IndexedAttestationDecoupled) bool {
		if a.Data == nil || !p.Valid(a.Data) {
			return false
		}
		added, err := p.Add(a)
		return err == nil && added
	})
	return sBlk.SetAttestationsDecoupled(atts)
}

// Spec: B.votes and B.support_votes. One aggregate per root over exactly the engine's snapshot, the support bit marks a root the engine supported. The parent was chosen from the whole snapshot, so a vote without its signed copy aborts the proposal rather than carrying a smaller view, which could hide an equivocation.
func (vs *Server) setAvailableAttestations(sBlk interfaces.SignedBeaconBlock, head state.BeaconState) error {
	slot := sBlk.Block().Slot()
	_, votes, support, ok := vs.ForkchoiceFetcher.DecoupledProposal(slot)
	if !ok {
		return errors.New("no decoupled proposal")
	}
	groups := make(map[[32]byte][]primitives.ValidatorIndex)
	var roots [][32]byte
	for _, v := range votes {
		if groups[v.Root] == nil {
			roots = append(roots, v.Root)
		}
		groups[v.Root] = append(groups[v.Root], v.Validator)
	}
	supported := make(map[[32]byte]bool, len(support))
	for _, v := range support {
		supported[v.Root] = true
	}
	size := uint64(head.NumValidators())
	atts := make([]*ethpb.AvailableAttestation, 0, len(roots))
	bits := bitfield.NewBitlist(uint64(len(roots)))
	for i, root := range roots {
		att, err := vs.DecoupledVotePool.Aggregate(slot-1, root, groups[root], size)
		if err != nil {
			return err
		}
		atts = append(atts, att)
		if supported[root] {
			bits.SetBitAt(uint64(i), true)
		}
	}
	if err := sBlk.SetAvailableAttestations(atts); err != nil {
		return err
	}
	return sBlk.SetSupportVotes(bits)
}
