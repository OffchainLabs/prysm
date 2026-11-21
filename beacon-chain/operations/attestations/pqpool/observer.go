package pqpool

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq/timing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// Spec: AGGREGATION_CUTOFF_BPS at 4595f0fc9
const collectionCutoffBPS = primitives.BP(4000)

var scenarios = [3]string{"committee_only", "proposer_only", "combined"}

type ObserverHead interface {
	ChainHead
	HeadStateReadOnly(ctx context.Context) (state.ReadOnlyBeaconState, error)
}

type ObserverConfig struct {
	Pool        *Pool
	Head        ObserverHead
	ClockWaiter startup.ClockWaiter
	Syncing     func() bool
	Peers       func() int
	Output      string
	Measure     bool
	Network     string
	Version     string
	Backend     string
}

type Observer struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    *ObserverConfig
	clock  *startup.Clock
	draws  [3]uint64
}

type experiment struct {
	anchor    primitives.Slot
	scenario  int
	committee primitives.CommitteeIndex
	timing.Experiment
}

func NewObserver(ctx context.Context, cfg *ObserverConfig) *Observer {
	ctx, cancel := context.WithCancel(ctx)
	return &Observer{ctx: ctx, cancel: cancel, cfg: cfg}
}

func (o *Observer) Start() {
	go o.run()
}

func (o *Observer) Stop() error {
	o.cancel()
	timing.Close()
	return nil
}

func (*Observer) Status() error {
	return nil
}

func (o *Observer) run() {
	clock, err := o.cfg.ClockWaiter.WaitForClock(o.ctx)
	if err != nil {
		return
	}
	o.clock = clock
	cfg := params.BeaconConfig()
	pqStart, err := slots.EpochStart(cfg.PQForkEpoch)
	if err != nil {
		log.WithError(err).Error("PQ timing needs a scheduled PQ fork")
		return
	}
	seed, err := timing.Open(timing.Run{
		Output:                o.cfg.Output,
		Observer:              o.cfg.Measure,
		Network:               o.cfg.Network,
		Version:               o.cfg.Version,
		Backend:               o.cfg.Backend,
		GenesisTime:           clock.GenesisTime(),
		GenesisValidatorsRoot: clock.GenesisValidatorsRoot(),
		Slot:                  cfg.SlotDuration(),
		AttestationDue:        cfg.SlotComponentDuration(cfg.AttestationDueBPSAtSlot(pqStart)),
		CommitteeCollection:   cfg.SlotComponentDuration(collectionCutoffBPS),
		AggregateDue:          cfg.SlotComponentDuration(cfg.AggregateDueBPSGloas),
		InitialMerge:          InitialMergeDelay,
	})
	if err != nil {
		log.WithError(err).Error("Could not start the PQ timing export")
		return
	}
	if !o.cfg.Measure {
		return
	}
	phase := seed % timing.Stride
	ticker := slots.NewSlotTicker(clock.GenesisTime(), cfg.SlotDuration())
	defer ticker.Done()
	var lastSynced bool
	lastPeers := -1
	for {
		select {
		case <-o.ctx.Done():
			return
		case slot := <-ticker.C():
			synced, peers := !o.cfg.Syncing(), o.cfg.Peers()
			if synced != lastSynced || peers != lastPeers {
				timing.Status("network_status", map[string]any{"synced": synced, "connected_peers": peers, "slot": uint64(slot)})
				lastSynced, lastPeers = synced, peers
			}
			// Planned a slot ahead so the inputs are chosen before any of them arrive.
			if anchor := slot + 1; uint64(anchor)%timing.Stride == phase {
				o.plan(anchor, seed, synced, pqStart)
			}
		}
	}
}

func (o *Observer) skip(anchor primitives.Slot, scenario int, reason string) {
	timing.Status("experiment_planned", map[string]any{
		"experiment_id": uint64(anchor), "reference_slot": uint64(anchor), "scenario": scenarios[scenario], "outcome": reason,
	})
}

func (o *Observer) plan(anchor primitives.Slot, seed uint64, synced bool, pqStart primitives.Slot) {
	scenario := int(uint64(anchor) / timing.Stride % 3) // #nosec G115 -- below 3
	if !synced {
		o.skip(anchor, scenario, "syncing")
		return
	}
	if anchor < pqStart {
		o.skip(anchor, scenario, "pre_pq")
		return
	}
	committeeSlot := anchor
	if scenarios[scenario] == "combined" {
		committeeSlot = anchor + 1
	}
	st, err := o.cfg.Head.HeadStateReadOnly(o.ctx)
	if err != nil {
		o.skip(anchor, scenario, "missing_geometry")
		return
	}
	root, err := o.cfg.Head.HeadRoot(o.ctx)
	if err != nil {
		o.skip(anchor, scenario, "missing_geometry")
		return
	}
	epoch := slots.ToEpoch(committeeSlot)
	active, err := helpers.ActiveValidatorCount(o.ctx, st, epoch)
	if err != nil {
		o.skip(anchor, scenario, "missing_geometry")
		return
	}
	count := helpers.SlotCommitteeCountAtEpoch(active, epoch)
	if count == 0 {
		o.skip(anchor, scenario, "missing_geometry")
		return
	}
	e := &experiment{anchor: anchor, scenario: scenario}
	e.ID, e.Scenario, e.CommitteeCount, e.ActiveValidators = uint64(anchor), scenarios[scenario], count, active
	fields := map[string]any{
		"experiment_id": e.ID, "sequence_id": sequenceID(e.ID), "reference_slot": e.ID, "scenario": e.Scenario, "outcome": "planned",
		"committee_count": count, "active_validators": active, "head_root": hexRoot(root),
		"target_slot": nil, "committee_slot": nil, "committee": nil,
	}
	withCommittee, withProposal := e.Scenario != "proposer_only", e.Scenario != "committee_only"
	if withCommittee {
		// Rotates through every committee so each one is sampled equally often.
		e.committee = primitives.CommitteeIndex((seed + o.draws[scenario]) % count)
		fields["committee_draw"] = o.draws[scenario]
		o.draws[scenario]++
		fields["committee_slot"], fields["committee"] = uint64(committeeSlot), uint64(e.committee)
	}
	if withProposal {
		fields["target_slot"] = uint64(anchor + 2)
	}
	timing.Status("experiment_planned", fields)
	go func() {
		if withCommittee {
			o.proveCommittee(e, committeeSlot)
		}
		if withProposal {
			o.mergeProposal(e, anchor+2)
		}
	}()
}

func (o *Observer) waitUntil(at time.Time) bool {
	select {
	case <-time.After(time.Until(at)):
		return true
	case <-o.ctx.Done():
		return false
	}
}

func (o *Observer) outcome(e *experiment, slot primitives.Slot, work, outcome string, err error) {
	fields := map[string]any{"experiment_id": e.ID, "slot": uint64(slot), "work": work, "outcome": outcome}
	if err != nil {
		fields["error"] = err.Error()
	}
	timing.Status("job_outcome", fields)
}

func (o *Observer) cutoff(e *experiment, slot primitives.Slot, work string, at time.Time) {
	timing.Status("input_cutoff", map[string]any{
		"experiment_id": e.ID, "slot": uint64(slot), "work": work, "scheduled_cutoff_wall_ns": at.UnixNano(),
	})
}

func (o *Observer) proveCommittee(e *experiment, slot primitives.Slot) {
	cfg := params.BeaconConfig()
	start, err := o.clock.SlotStart(slot)
	if err != nil {
		return
	}
	cutoff := start.Add(cfg.SlotComponentDuration(collectionCutoffBPS))
	if !o.waitUntil(cutoff) {
		return
	}
	o.cutoff(e, slot, "committee", cutoff)
	scoped := e.Experiment
	scoped.Deadline = start.Add(cfg.SlotComponentDuration(cfg.AggregateDueBPSGloas))
	ctx := timing.WithExperiment(o.ctx, &scoped)
	st, err := o.cfg.Head.HeadStateReadOnly(ctx)
	if err != nil {
		o.outcome(e, slot, "committee", "missing_context", err)
		return
	}
	aggregate, err := BuildAggregate(ctx, st, o.cfg.Pool.All(), slot, e.committee)
	if err != nil {
		o.outcome(e, slot, "committee", "failed", err)
		return
	}
	if aggregate == nil {
		o.outcome(e, slot, "committee", "no_useful_inputs", nil)
		return
	}
	verifyStart := time.Now()
	if err := pq.VerifyAttestationProof(ctx, st, []ethpb.Att{aggregate.Att}, aggregate.Proof); err != nil {
		o.outcome(e, slot, "committee", "invalid_proof", err)
		return
	}
	timing.NewJob(ctx, "committee", uint64(slot), uint64(slot)).Emit("candidate_ready", "ready", map[string]any{
		"proof_id": timing.ProofID(aggregate.Proof), "claims": aggregate.Att.AggregationBits.Count(), "records": 1,
		"recursive_depth": 0, "backend_call": true, "verification_duration_ns": time.Since(verifyStart).Nanoseconds(),
	})
}

func (o *Observer) mergeProposal(e *experiment, target primitives.Slot) {
	start, err := o.clock.SlotStart(target)
	if err != nil {
		return
	}
	scoped := e.Experiment
	scoped.Deadline = start
	ctx := timing.WithExperiment(o.ctx, &scoped)
	announced := false
	preparer := &Preparer{
		Head:        o.cfg.Head,
		Pool:        o.cfg.Pool,
		GenesisTime: o.clock.GenesisTime(),
		Wanted: func(context.Context, state.ReadOnlyBeaconState) bool {
			if !announced {
				o.cutoff(e, target, "initial", time.Now())
				announced = true
			}
			return true
		},
		OnReady: func(ctx context.Context, st state.ReadOnlyBeaconState, prepared *Prepared) {
			verifyStart := time.Now()
			if err := pq.VerifyAttestationProof(ctx, st, prepared.Atts, prepared.Proof); err != nil {
				o.outcome(e, target, "initial", "invalid_proof", err)
				return
			}
			timing.NewJob(timing.WithRound(ctx, prepared.Round), "initial", uint64(target), uint64(e.anchor)).Emit("candidate_ready", "ready", map[string]any{
				"proof_id": timing.ProofID(prepared.Proof), "claims": prepared.Validators, "records": len(prepared.Atts), "recursive_depth": prepared.Round,
				"backend_call": true, "verification_duration_ns": time.Since(verifyStart).Nanoseconds(),
			})
		},
	}
	prepared := preparer.Run(ctx, target)
	if !o.waitUntil(start) {
		return
	}
	fields := map[string]any{"nominal_snapshot_wall_ns": start.UnixNano()}
	outcome := "no_candidate"
	round := 1
	if prepared != nil {
		outcome, round = "ready", prepared.Round
		fields["proof_id"], fields["claims"], fields["records"] = timing.ProofID(prepared.Proof), prepared.Validators, len(prepared.Atts)
		fields["recursive_depth"], fields["backend_call_count"] = prepared.Round, prepared.Round
		fields["parent_root"] = hexRoot(prepared.ParentRoot[:])
	}
	timing.NewJob(timing.WithRound(ctx, round), "initial", uint64(target), uint64(e.anchor)).Emit("proposal_snapshot", outcome, fields)
}

func sequenceID(id uint64) string {
	return fmt.Sprintf("measurement:%d", id)
}

func hexRoot(root []byte) string {
	return "0x" + hex.EncodeToString(root)
}
