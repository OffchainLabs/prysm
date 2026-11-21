package validator

import (
	"context"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq/timing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations/pqpool"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

type pqPreparedProposal struct {
	mu     sync.Mutex
	latest *pqpool.Prepared
}

// Merging is a proving step, so it runs before the slot and the block is not held up by it.
func (vs *Server) PreparePQProposals() {
	for vs.TimeFetcher.GenesisTime().IsZero() {
		select {
		case <-time.After(time.Second):
		case <-vs.Ctx.Done():
			return
		}
	}
	cfg := params.BeaconConfig()
	ticker := slots.NewSlotTicker(vs.TimeFetcher.GenesisTime(), cfg.SlotDuration())
	defer ticker.Done()
	for {
		select {
		case <-vs.Ctx.Done():
			return
		case slot := <-ticker.C():
			target := slot + cfg.MinAttestationInclusionDelayPQ
			if slots.ToEpoch(target) < cfg.PQForkEpoch || !vs.SubscribedValidatorsCache.Validating() {
				continue
			}
			go vs.preparePQProposal(target)
		}
	}
}

func (vs *Server) preparePQProposal(slot primitives.Slot) {
	preparer := &pqpool.Preparer{
		Head:        vs.HeadFetcher,
		Pool:        vs.PQAttPool,
		GenesisTime: vs.TimeFetcher.GenesisTime(),
		Wanted: func(ctx context.Context, st state.ReadOnlyBeaconState) bool {
			proposer, err := helpers.BeaconProposerIndexAtSlot(ctx, st, slot)
			return err == nil && vs.SubscribedValidatorsCache.Has(proposer)
		},
		OnReady: func(_ context.Context, _ state.ReadOnlyBeaconState, prepared *pqpool.Prepared) {
			vs.pqPrepared.mu.Lock()
			vs.pqPrepared.latest = prepared
			vs.pqPrepared.mu.Unlock()
		},
	}
	preparer.Run(vs.Ctx, slot)
}

func (vs *Server) packPQ(ctx context.Context, slot primitives.Slot, parentRoot [32]byte, head state.ReadOnlyBeaconState) ([]ethpb.Att, []byte, error) {
	vs.pqPrepared.mu.Lock()
	prepared := vs.pqPrepared.latest
	vs.pqPrepared.mu.Unlock()
	reference := slot - min(slot, params.BeaconConfig().MinAttestationInclusionDelayPQ)
	snapshot := timing.NewJob(ctx, "initial", uint64(slot), uint64(reference))
	if prepared != nil && prepared.Slot == slot && prepared.ParentRoot == parentRoot {
		snapshot.Emit("proposal_snapshot", "candidate", map[string]any{
			"proof_id": timing.ProofID(prepared.Proof), "records": len(prepared.Atts), "claims": prepared.Validators, "round": prepared.Round,
		})
		return prepared.Atts, prepared.Proof, nil
	}
	snapshot.Emit("proposal_snapshot", "no_candidate", nil)
	start, err := slots.StartTime(vs.TimeFetcher.GenesisTime(), slot)
	if err != nil {
		return nil, nil, err
	}
	type result struct {
		atts  []ethpb.Att
		proof []byte
		err   error
	}
	done := make(chan result, 1)
	go func() {
		atts, proof, err := pqpool.Pack(ctx, head, vs.PQAttPool.All(), vs.PQAttPool.Aggregates())
		done <- result{atts, proof, err}
	}()
	select {
	case r := <-done:
		return r.atts, r.proof, r.err
	case <-time.After(time.Until(pqpool.FallbackDeadline(start, slot))):
		// A block without attestations beats a block that misses the attestation deadline.
		log.WithField("slot", slot).Warn("PQ block proof was not ready in time, proposing without attestations")
		return nil, nil, nil
	}
}
