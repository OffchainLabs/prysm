package pqpool

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq/timing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// Same policy and values as the Lighthouse pq-devnet-0 prover scheduler.
const (
	InitialMergeDelay       = 8 * time.Second
	mergeSafetyMargin       = 500 * time.Millisecond
	mergeEstimateMultiplier = 1.6
	preparePoll             = 250 * time.Millisecond
)

type ChainHead interface {
	HeadRoot(ctx context.Context) ([]byte, error)
	HeadState(ctx context.Context) (state.BeaconState, error)
}

type Prepared struct {
	Slot       primitives.Slot
	ParentRoot [32]byte
	Atts       []ethpb.Att
	Proof      []byte
	Validators uint64
	Gain       int
	Round      int
	VoteSlots  []primitives.Slot
	Score      uint64
	JoinTime   time.Duration
}

type Preparer struct {
	Head        ChainHead
	Pool        *Pool
	GenesisTime time.Time
	// Asked once per head, a false answer ends the preparation.
	Wanted  func(ctx context.Context, st state.ReadOnlyBeaconState) bool
	OnReady func(ctx context.Context, st state.ReadOnlyBeaconState, prepared *Prepared)
}

// Past this point a merge inside the proposal slot leaves no time to gossip the block.
func FallbackDeadline(slotStart time.Time, slot primitives.Slot) time.Time {
	cfg := params.BeaconConfig()
	return slotStart.Add(cfg.SlotComponentDuration(cfg.AttestationDueBPSAtSlot(slot)) * 3 / 4)
}

func (p *Preparer) Run(ctx context.Context, slot primitives.Slot) *Prepared {
	delay := params.BeaconConfig().MinAttestationInclusionDelayPQ
	if slot < delay {
		return nil
	}
	voteStart, err := slots.StartTime(p.GenesisTime, slot-delay)
	if err != nil {
		return nil
	}
	deadline, err := slots.StartTime(p.GenesisTime, slot)
	if err != nil {
		return nil
	}
	if !sleepUntil(ctx, voteStart.Add(InitialMergeDelay)) {
		return nil
	}
	var (
		ready     *Prepared
		st        state.BeaconState
		root      [32]byte
		lastMerge time.Duration
		round     int
	)
	for time.Now().Before(deadline) {
		headRoot, err := p.Head.HeadRoot(ctx)
		if err != nil {
			return ready
		}
		if st == nil || root != [32]byte(headRoot) {
			st, err = p.Head.HeadState(ctx)
			if err != nil {
				return nil
			}
			if st.Slot() < slot {
				if st, err = transition.ProcessSlotsUsingNextSlotCache(ctx, st, headRoot, slot); err != nil {
					log.WithError(err).WithField("slot", slot).Debug("Could not advance the state to prepare a PQ proposal")
					return nil
				}
			}
			if p.Wanted != nil && !p.Wanted(ctx, st) {
				return nil
			}
			// A proof packed for another parent may hold votes this chain already has.
			root, ready = [32]byte(headRoot), nil
		}
		packing, err := Select(ctx, st, p.Pool.All(), p.Pool.Aggregates())
		if err != nil {
			log.WithError(err).WithField("slot", slot).Debug("Could not select attestations for a PQ proposal")
			return ready
		}
		improves := len(packing.Atts) > 0 && (ready == nil || packing.Gain > ready.Gain)
		estimate := time.Duration(float64(lastMerge) * mergeEstimateMultiplier)
		fits := ready == nil || time.Now().Add(estimate).Add(mergeSafetyMargin).Before(deadline)
		if improves && fits {
			round++
			start := time.Now()
			proof, err := packing.Merge(timing.WithRound(ctx, round))
			if err != nil {
				log.WithError(err).WithField("slot", slot).Warn("Could not merge the PQ block proof ahead of time")
				return ready
			}
			lastMerge = time.Since(start)
			ready = &Prepared{Slot: slot, ParentRoot: root, Atts: packing.Atts, Proof: proof, Validators: packing.Validators, Gain: packing.Gain, Round: round}
			log.WithFields(logrus.Fields{
				"slot":       slot,
				"round":      round,
				"validators": packing.Validators,
				"mergeTime":  lastMerge,
				"beforeSlot": time.Until(deadline),
			}).Info("Prepared PQ block proof ahead of the slot")
			if p.OnReady != nil {
				p.OnReady(ctx, st, ready)
			}
		}
		if !sleepUntil(ctx, time.Now().Add(preparePoll)) {
			return ready
		}
	}
	return ready
}

func sleepUntil(ctx context.Context, at time.Time) bool {
	select {
	case <-time.After(time.Until(at)):
		return true
	case <-ctx.Done():
		return false
	}
}
