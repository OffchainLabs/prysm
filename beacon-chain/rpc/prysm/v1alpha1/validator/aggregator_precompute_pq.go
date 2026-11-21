package validator

import (
	"context"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations/pqpool"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Spec: AGGREGATION_CUTOFF_BPS at 4595f0fc9, position 0 only since duties carry no position yet.
const aggregationCutoffBPS = primitives.BP(4000)

type pqAggregateKey struct {
	slot  primitives.Slot
	index primitives.CommitteeIndex
}

type pqAggregateEntry struct {
	done      chan struct{}
	aggregate *pqpool.Aggregate
	err       error
}

type pqAggregateCache struct {
	mu      sync.Mutex
	entries map[pqAggregateKey]*pqAggregateEntry
}

// Proving starts at the cutoff so the aggregate is ready when the validator client asks for it.
func (vs *Server) RegisterAggregationDutyPQ(slot primitives.Slot, index primitives.CommitteeIndex) {
	key := pqAggregateKey{slot: slot, index: index}
	c := &vs.pqAggregates
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[pqAggregateKey]*pqAggregateEntry)
	}
	if _, ok := c.entries[key]; ok {
		c.mu.Unlock()
		return
	}
	// Duties are registered an epoch ahead, so old entries are measured against the clock.
	current := vs.TimeFetcher.CurrentSlot()
	for k := range c.entries {
		if k.slot+2 < current {
			delete(c.entries, k)
		}
	}
	entry := &pqAggregateEntry{done: make(chan struct{})}
	c.entries[key] = entry
	c.mu.Unlock()

	go func() {
		defer close(entry.done)
		start, err := slots.StartTime(vs.TimeFetcher.GenesisTime(), slot)
		if err != nil {
			entry.err = err
			return
		}
		cutoff := start.Add(params.BeaconConfig().SlotComponentDuration(aggregationCutoffBPS))
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
		entry.aggregate, entry.err = pqpool.BuildAggregate(vs.Ctx, st, vs.PQAttPool.All(), slot, index)
		if entry.err != nil {
			log.WithError(entry.err).WithField("slot", slot).Error("Could not build PQ aggregate ahead of time")
		}
	}()
}

func (vs *Server) precomputedAggregatePQ(ctx context.Context, slot primitives.Slot, index primitives.CommitteeIndex) (*pqpool.Aggregate, error) {
	vs.pqAggregates.mu.Lock()
	entry, ok := vs.pqAggregates.entries[pqAggregateKey{slot: slot, index: index}]
	vs.pqAggregates.mu.Unlock()
	if !ok {
		return nil, errors.New("aggregate was not registered")
	}
	select {
	case <-entry.done:
		return entry.aggregate, entry.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
