package validator

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq/timing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations/pqpool"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// Merging is a proving step, so it runs before the slot and the block is not held up by it.
func (vs *Server) PreparePQProposals() {
	for vs.TimeFetcher.GenesisTime().IsZero() {
		select {
		case <-time.After(time.Second):
		case <-vs.Ctx.Done():
			return
		}
	}
	builder := &pqpool.Builder{
		Head:        vs.HeadFetcher,
		Pool:        vs.PQAttPool,
		GenesisTime: vs.TimeFetcher.GenesisTime(),
		Local: func(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot) bool {
			proposer, err := helpers.BeaconProposerIndexAtSlot(ctx, st, slot)
			return err == nil && vs.SubscribedValidatorsCache.Has(proposer)
		},
	}
	vs.pqBuilder.Store(builder)
	cfg := params.BeaconConfig()
	ticker := slots.NewSlotTicker(vs.TimeFetcher.GenesisTime(), cfg.SlotDuration())
	defer ticker.Done()
	for {
		select {
		case <-vs.Ctx.Done():
			return
		case slot := <-ticker.C():
			if slots.ToEpoch(slot+1) < cfg.PQForkEpoch || !vs.SubscribedValidatorsCache.Validating() {
				continue
			}
			go builder.Slot(vs.Ctx, slot)
		}
	}
}

func (vs *Server) packPQ(ctx context.Context, slot primitives.Slot, head state.ReadOnlyBeaconState) ([]ethpb.Att, []byte, error) {
	reference := slot - min(slot, params.BeaconConfig().MinAttestationInclusionDelayPQ)
	snapshot := timing.NewJob(ctx, "initial", uint64(slot), uint64(reference))
	start, err := slots.StartTime(vs.TimeFetcher.GenesisTime(), slot)
	if err != nil {
		return nil, nil, err
	}
	builder := vs.pqBuilder.Load()
	var prepared *pqpool.Prepared
	if builder != nil {
		prepared = builder.Pack(ctx, slot, head, pqpool.FallbackDeadline(start, slot))
	}
	if prepared == nil {
		snapshot.Emit("proposal_snapshot", "no_candidate", nil)
		if builder != nil && builder.Has(reference) {
			log.WithFields(logrus.Fields{"slot": slot, "voteSlot": reference}).Info("Nothing to pack in the PQ block, the ready votes are already on chain")
			return nil, nil, nil
		}
		// Proving inside the proposal slot delays the block past the attestation deadline.
		log.WithFields(logrus.Fields{"slot": slot, "voteSlot": reference}).Warn("No PQ vote slot proof was ready, proposing without attestations")
		return nil, nil, nil
	}
	snapshot.Emit("proposal_snapshot", "candidate", map[string]any{
		"proof_id": timing.ProofID(prepared.Proof), "records": len(prepared.Atts), "claims": prepared.Validators, "round": prepared.Round,
	})
	log.WithFields(logrus.Fields{
		"slot":           slot,
		"voteSlots":      prepared.VoteSlots,
		"attestations":   len(prepared.Atts),
		"validators":     prepared.Validators,
		"newVoters":      prepared.Gain,
		"rewardScore":    prepared.Score,
		"joined":         len(prepared.VoteSlots) > 1,
		"joinTime":       prepared.JoinTime,
		"sinceSlotStart": time.Since(start),
	}).Info("Packed PQ block with vote slot proofs")
	return prepared.Atts, prepared.Proof, nil
}
