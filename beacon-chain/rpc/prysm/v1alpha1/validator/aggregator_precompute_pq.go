package validator

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations/pqpool"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

type pqAggregateKey struct {
	slot  primitives.Slot
	index primitives.CommitteeIndex
}

type pqAggregateEntry struct {
	position  uint64
	done      chan struct{}
	aggregate *pqpool.Aggregate
	err       error
}

type pqAggregateCache struct {
	// Held from the redundancy check to the save, so two submissions cannot both broadcast the same attesters.
	submit sync.Mutex
	mu     sync.Mutex
	// Sorted by position.
	entries map[pqAggregateKey][]*pqAggregateEntry
}

// The aggregator API carries no position, so it is read from the aggregation committee, and zero if that fails.
func (vs *Server) aggregatorPositionPQ(slot primitives.Slot, index primitives.CommitteeIndex, validator primitives.ValidatorIndex) uint64 {
	st, err := vs.HeadFetcher.HeadStateReadOnly(vs.Ctx)
	if err != nil {
		return 0
	}
	position, err := pq.AggregatorPosition(st, slot, index, validator)
	if err != nil {
		return 0
	}
	return uint64(max(position, 0))
}

// Proving starts at the cutoff of the aggregator so the aggregate is ready when the validator client asks for it.
func (vs *Server) RegisterAggregationDutyPQ(slot primitives.Slot, index primitives.CommitteeIndex, validator primitives.ValidatorIndex) {
	vs.registerAggregationPositionPQ(slot, index, vs.aggregatorPositionPQ(slot, index, validator))
}

func (vs *Server) registerAggregationPositionPQ(slot primitives.Slot, index primitives.CommitteeIndex, position uint64) {
	key := pqAggregateKey{slot: slot, index: index}
	c := &vs.pqAggregates
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[pqAggregateKey][]*pqAggregateEntry)
	}
	// Duties are registered an epoch ahead, so old entries are measured against the clock.
	current := vs.TimeFetcher.CurrentSlot()
	for k := range c.entries {
		if k.slot+2 < current {
			delete(c.entries, k)
		}
	}
	at, ok := slices.BinarySearchFunc(c.entries[key], position, func(e *pqAggregateEntry, p uint64) int { return cmp.Compare(e.position, p) })
	if ok {
		c.mu.Unlock()
		return
	}
	entry := &pqAggregateEntry{position: position, done: make(chan struct{})}
	c.entries[key] = slices.Insert(c.entries[key], at, entry)
	c.mu.Unlock()

	go func() {
		defer close(entry.done)
		start, err := slots.StartTime(vs.TimeFetcher.GenesisTime(), slot)
		if err != nil {
			entry.err = err
			return
		}
		cutoff := start.Add(params.BeaconConfig().AggregationCutoffPQ(position))
		select {
		case <-time.After(time.Until(cutoff)):
		case <-vs.Ctx.Done():
			entry.err = vs.Ctx.Err()
			return
		}
		st, err := vs.HeadFetcher.HeadStateReadOnly(vs.Ctx)
		if err != nil {
			entry.err = err
			return
		}
		// Subscriptions come in any order, so the earlier positions are read at the cutoff.
		c.mu.Lock()
		var previous *pqAggregateEntry
		if at := slices.Index(c.entries[key], entry); at > 0 {
			previous = c.entries[key][at-1]
		}
		c.mu.Unlock()
		singles := vs.PQAttPool.All()
		entry.aggregate, entry.err = pqpool.BuildAggregate(vs.Ctx, st, singles, slot, index, func(att *ethpb.AttestationPQ) (*pqpool.Aggregate, error) {
			// An earlier local aggregator with the same attesters has the proof already, or is building it.
			if previous != nil {
				select {
				case <-previous.done:
				case <-vs.Ctx.Done():
					return nil, vs.Ctx.Err()
				}
				if a := previous.aggregate; a != nil && pqpool.Contains(a.Att, att) {
					return a, nil
				}
			}
			return vs.PQAttPool.Covering(att)
		})
		if entry.err != nil {
			log.WithError(entry.err).WithField("slot", slot).Error("Could not build PQ aggregate ahead of time")
		}
		if entry.aggregate != nil {
			margin := time.Until(start.Add(params.BeaconConfig().AggregateDuePQ(position)))
			pqAggregateReadyOffset.Observe(time.Since(start).Seconds())
			pqAggregateDueMargin.Observe(margin.Seconds())
			if margin < 0 {
				pqAggregateLate.Inc()
			}
		}
	}()
}

// The request names no aggregator, so it gets the aggregate of the last position that is due and has the data asked for.
func (vs *Server) precomputedAggregatePQ(ctx context.Context, slot primitives.Slot, index primitives.CommitteeIndex, dataRoot [32]byte) (*pqpool.Aggregate, error) {
	cfg := params.BeaconConfig()
	start, err := slots.StartTime(vs.TimeFetcher.GenesisTime(), slot)
	if err != nil {
		return nil, err
	}
	// Half a delay of slack for a validator client whose clock runs ahead.
	now := time.Now().Add(cfg.SlotComponentDuration(cfg.AggregatorDelayBPS) / 2)
	vs.pqAggregates.mu.Lock()
	var due []*pqAggregateEntry
	for _, e := range vs.pqAggregates.entries[pqAggregateKey{slot: slot, index: index}] {
		if len(due) == 0 || !start.Add(cfg.AggregateDuePQ(e.position)).After(now) {
			due = append(due, e)
		}
	}
	vs.pqAggregates.mu.Unlock()
	matching := func(entry *pqAggregateEntry) (*pqpool.Aggregate, error) {
		if entry.err != nil || entry.aggregate == nil {
			return nil, entry.err
		}
		root, err := entry.aggregate.Att.Data.HashTreeRoot()
		if err != nil || root != dataRoot {
			return nil, err
		}
		return entry.aggregate, nil
	}
	for _, entry := range slices.Backward(due) {
		select {
		case <-entry.done:
		case <-ctx.Done():
			// A later position may hold more attesters, so an earlier proof is only used once the request runs out of time.
			for _, earlier := range slices.Backward(due) {
				select {
				case <-earlier.done:
					if aggregate, _ := matching(earlier); aggregate != nil {
						return aggregate, nil
					}
				default:
				}
			}
			return nil, ctx.Err()
		}
		aggregate, matchErr := matching(entry)
		if aggregate != nil {
			return aggregate, nil
		}
		if matchErr != nil {
			err = matchErr
		}
	}
	return nil, err
}
