package client

import (
	"context"
	"slices"
	"sync"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

const batchAttestationCollectionTimeout = 150 * time.Millisecond

type batchAttestationKey struct {
	slot           primitives.Slot
	committeeIndex primitives.CommitteeIndex
	dataRoot       [32]byte
}

type localBatchAttesterDuty struct {
	pubKey                  [fieldparams.BLSPubkeyLength]byte
	validatorIndex          primitives.ValidatorIndex
	validatorCommitteeIndex uint64
}

type batchAttestationRequest struct {
	key             batchAttestationKey
	expected        int
	committeeLength uint64
	batcher         primitives.ValidatorIndex
	batcherPubKey   [fieldparams.BLSPubkeyLength]byte
	data            *ethpb.AttestationData
	contribution    BatchContribution
}

type batchAttestationSnapshot struct {
	committeeIndex  primitives.CommitteeIndex
	committeeLength uint64
	batcher         primitives.ValidatorIndex
	batcherPubKey   [fieldparams.BLSPubkeyLength]byte
	data            *ethpb.AttestationData
	contributions   []BatchContribution
}

type batchAttestationEntry struct {
	expected        int
	committeeIndex  primitives.CommitteeIndex
	committeeLength uint64
	batcher         primitives.ValidatorIndex
	batcherPubKey   [fieldparams.BLSPubkeyLength]byte
	data            *ethpb.AttestationData
	contributions   map[primitives.ValidatorIndex]BatchContribution
	done            chan struct{}
	submitting      bool
	completed       bool
	resp            *ethpb.AttestResponse
	err             error
}

type batchAttestationCoordinator struct {
	mu      sync.Mutex
	entries map[batchAttestationKey]*batchAttestationEntry
}

func newBatchAttestationCoordinator() *batchAttestationCoordinator {
	return &batchAttestationCoordinator{
		entries: make(map[batchAttestationKey]*batchAttestationEntry),
	}
}

func (c *batchAttestationCoordinator) contribute(
	ctx context.Context,
	req batchAttestationRequest,
	submit func(context.Context, batchAttestationSnapshot) (*ethpb.AttestResponse, error),
) (*ethpb.AttestResponse, bool, error) {
	if req.expected < 2 {
		return nil, false, nil
	}
	if c == nil {
		return nil, false, errors.New("batch attestation coordinator is nil")
	}

	entry, shouldSubmit, snapshot := c.add(req)
	if shouldSubmit {
		go c.submit(ctx, req.key, snapshot, submit)
	}

	timer := time.NewTimer(batchAttestationCollectionTimeout)
	defer timer.Stop()

	select {
	case <-entry.done:
		return entry.resp, entry.err == nil && entry.resp != nil, entry.err
	case <-timer.C:
		if c.failIfPending(req.key, errors.New("batch attestation collection timed out")) {
			return nil, false, nil
		}
		<-entry.done
		return entry.resp, entry.err == nil && entry.resp != nil, entry.err
	case <-ctx.Done():
		if c.failIfPending(req.key, ctx.Err()) {
			return nil, false, ctx.Err()
		}
		<-entry.done
		return entry.resp, entry.err == nil && entry.resp != nil, entry.err
	}
}

func (c *batchAttestationCoordinator) add(req batchAttestationRequest) (*batchAttestationEntry, bool, batchAttestationSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[batchAttestationKey]*batchAttestationEntry)
	}
	c.prune(req.key.slot)

	entry := c.entries[req.key]
	if entry == nil {
		entry = &batchAttestationEntry{
			expected:        req.expected,
			committeeIndex:  req.key.committeeIndex,
			committeeLength: req.committeeLength,
			batcher:         req.batcher,
			batcherPubKey:   req.batcherPubKey,
			data:            req.data,
			contributions:   make(map[primitives.ValidatorIndex]BatchContribution, req.expected),
			done:            make(chan struct{}),
		}
		c.entries[req.key] = entry
	}
	if entry.completed {
		return entry, false, batchAttestationSnapshot{}
	}
	entry.contributions[req.contribution.AttesterIndex] = req.contribution

	if len(entry.contributions) < entry.expected || entry.submitting {
		return entry, false, batchAttestationSnapshot{}
	}
	entry.submitting = true
	return entry, true, entry.snapshot()
}

func (e *batchAttestationEntry) snapshot() batchAttestationSnapshot {
	contributions := make([]BatchContribution, 0, len(e.contributions))
	for _, contribution := range e.contributions {
		contributions = append(contributions, contribution)
	}
	slices.SortFunc(contributions, func(a, b BatchContribution) int {
		return a.AttesterCommitteePos - b.AttesterCommitteePos
	})
	return batchAttestationSnapshot{
		committeeIndex:  e.committeeIndex,
		committeeLength: e.committeeLength,
		batcher:         e.batcher,
		batcherPubKey:   e.batcherPubKey,
		data:            e.data,
		contributions:   contributions,
	}
}

func (c *batchAttestationCoordinator) submit(
	ctx context.Context,
	key batchAttestationKey,
	snapshot batchAttestationSnapshot,
	submit func(context.Context, batchAttestationSnapshot) (*ethpb.AttestResponse, error),
) {
	resp, err := submit(ctx, snapshot)

	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if entry == nil || entry.completed {
		return
	}
	entry.resp = resp
	entry.err = err
	entry.completed = true
	close(entry.done)
}

func (c *batchAttestationCoordinator) failIfPending(key batchAttestationKey, err error) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if entry == nil || entry.completed || entry.submitting {
		return false
	}
	entry.err = err
	entry.completed = true
	close(entry.done)
	return true
}

func (c *batchAttestationCoordinator) prune(currentSlot primitives.Slot) {
	if currentSlot == 0 {
		return
	}
	for key := range c.entries {
		if key.slot+1 < currentSlot {
			delete(c.entries, key)
		}
	}
}
