package pqpool

import (
	"cmp"
	"context"
	"slices"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// Spec: MAX_STARK_PROOF_SIZE
const maxProofSize = 1 << 20

type group struct {
	data       *ethpb.AttestationData
	signatures map[primitives.CommitteeIndex]map[primitives.ValidatorIndex][]byte
}

// Pack builds the attestations for a block at the slot of st, and the proof of their signatures.
func Pack(ctx context.Context, st state.ReadOnlyBeaconState, singles []*ethpb.SingleAttestationPQ) ([]ethpb.Att, []byte, error) {
	cfg := params.BeaconConfig()
	slot := st.Slot()
	currentEpoch := slots.ToEpoch(slot)
	previousEpoch := currentEpoch
	if currentEpoch > 0 {
		previousEpoch = currentEpoch - 1
	}
	currentParticipation, err := st.CurrentEpochParticipation()
	if err != nil {
		return nil, nil, err
	}
	previousParticipation, err := st.PreviousEpochParticipation()
	if err != nil {
		return nil, nil, err
	}

	groups := make(map[[32]byte]*group)
	for _, single := range singles {
		data := single.Data
		if data.Slot+cfg.MinAttestationInclusionDelayPQ > slot {
			continue
		}
		participation := currentParticipation
		if data.Target.Epoch != currentEpoch {
			if data.Target.Epoch != previousEpoch {
				continue
			}
			participation = previousParticipation
		}
		// A validator whose target vote is on chain earns nothing from another inclusion.
		if uint64(single.AttesterIndex) >= uint64(len(participation)) || participation[single.AttesterIndex]>>cfg.TimelyTargetFlagIndex&1 == 1 {
			continue
		}
		root, err := data.HashTreeRoot()
		if err != nil {
			return nil, nil, err
		}
		g, ok := groups[root]
		if !ok {
			g = &group{data: data, signatures: make(map[primitives.CommitteeIndex]map[primitives.ValidatorIndex][]byte)}
			groups[root] = g
		}
		if g.signatures[single.CommitteeId] == nil {
			g.signatures[single.CommitteeId] = make(map[primitives.ValidatorIndex][]byte)
		}
		g.signatures[single.CommitteeId][single.AttesterIndex] = single.Signature
	}

	ordered := make([]*group, 0, len(groups))
	for _, g := range groups {
		ordered = append(ordered, g)
	}
	// Oldest first, they are the closest to expiring.
	slices.SortFunc(ordered, func(a, b *group) int {
		if c := cmp.Compare(a.data.Slot, b.data.Slot); c != 0 {
			return c
		}
		return slices.Compare(a.data.BeaconBlockRoot, b.data.BeaconBlockRoot)
	})

	budget := maxProofSize / pq.XMSSSignatureLength
	var atts []ethpb.Att
	var statements []pq.Statement
	var signatures [][][]byte
	for _, g := range ordered {
		if uint64(len(atts)) >= cfg.MaxAttestationsPQ || budget == 0 {
			break
		}
		committeeIndices := make([]primitives.CommitteeIndex, 0, len(g.signatures))
		for ci := range g.signatures {
			committeeIndices = append(committeeIndices, ci)
		}
		slices.Sort(committeeIndices)

		committeeBits := ethpb.NewCommitteeBitsPQ()
		var bits []bool
		var groupSignatures [][][]byte
		used := 0
		for _, ci := range committeeIndices {
			committee, err := helpers.BeaconCommitteeFromState(ctx, st, g.data.Slot, ci)
			if err != nil {
				continue
			}
			committeeBitsSet := make([]bool, len(committee))
			var committeeSignatures [][]byte
			for i, index := range committee {
				if sig, ok := g.signatures[ci][index]; ok {
					committeeBitsSet[i] = true
					committeeSignatures = append(committeeSignatures, sig)
				}
			}
			if len(committeeSignatures) == 0 || len(committeeSignatures) > budget-used {
				continue
			}
			committeeBits.SetBitAt(uint64(ci), true)
			bits = append(bits, committeeBitsSet...)
			groupSignatures = append(groupSignatures, committeeSignatures)
			used += len(committeeSignatures)
		}
		if used == 0 {
			continue
		}
		aggregationBits := bitfield.NewBitlist(uint64(len(bits)))
		for i, set := range bits {
			if set {
				aggregationBits.SetBitAt(uint64(i), true)
			}
		}
		att := &ethpb.AttestationPQ{AggregationBits: aggregationBits, Data: g.data, CommitteeBits: committeeBits}
		if err := blocks.VerifyAttestationNoVerifySignature(ctx, st, att); err != nil {
			log.WithError(err).WithField("slot", g.data.Slot).Debug("Skipping attestation that does not fit the block")
			continue
		}
		attStatements, err := pq.AttestationStatements(ctx, st, att)
		if err != nil {
			return nil, nil, err
		}
		atts = append(atts, att)
		statements = append(statements, attStatements...)
		signatures = append(signatures, groupSignatures...)
		budget -= used
	}

	proof, err := pq.Proofs.Prove(statements, signatures)
	if err != nil {
		return nil, nil, err
	}
	return atts, proof, nil
}
