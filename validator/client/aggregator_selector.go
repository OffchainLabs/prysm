package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
)

// aggregatorSelector abstracts selection proof generation and aggregation decisions.
// In local mode, proofs are signed with the local keymanager.
// In distributed mode, partial proofs are sent to DVT middleware for aggregation.
type aggregatorSelector interface {
	RefreshSelectionProofs(ctx context.Context) error
	AttestationSelectionProof(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error)
	// ClaimAggregateSlot atomically checks and claims the right to aggregate for
	// a (slot, committee) pair. Returns false if already claimed. In distributed
	// mode the middleware handles dedup, so this always returns true.
	ClaimAggregateSlot(slot primitives.Slot, committeeIndex primitives.CommitteeIndex) bool
	SyncCommitteeAggregators(ctx context.Context, slot primitives.Slot, pubkeys [][fieldparams.BLSPubkeyLength]byte) ([][fieldparams.BLSPubkeyLength]byte, error)
	SyncCommitteeSelectionProofs(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, indexRes *ethpb.SyncSubcommitteeIndexResponse) ([][]byte, error)
}

type attSelectionKey struct {
	slot  primitives.Slot
	index primitives.ValidatorIndex
}

// localSelector computes selection proofs using the local keymanager.
type localSelector struct {
	v          *validator
	dedupLock  sync.Mutex
	dedupCache *lru.Cache
	proofLock  sync.Mutex
	proofCache map[attSelectionKey][]byte
	proofGroup singleflight.Group
}

func syncSubnet(index uint64) uint64 {
	cfg := params.BeaconConfig()
	return index / (cfg.SyncCommitteeSize / cfg.SyncCommitteeSubnetCount)
}

func newLocalSelector(v *validator) (*localSelector, error) {
	cache, err := lru.New(int(params.BeaconConfig().MaxCommitteesPerSlot))
	if err != nil {
		return nil, errors.Wrap(err, "could not create dedup cache")
	}
	return &localSelector{v: v, dedupCache: cache, proofCache: make(map[attSelectionKey][]byte)}, nil
}

func (p *localSelector) getCachedProof(key attSelectionKey) ([]byte, bool) {
	p.proofLock.Lock()
	defer p.proofLock.Unlock()
	proof, ok := p.proofCache[key]
	return proof, ok
}

func (p *localSelector) cacheProof(key attSelectionKey, proof []byte) {
	p.proofLock.Lock()
	defer p.proofLock.Unlock()
	p.proofCache[key] = proof
}

func (p *localSelector) RefreshSelectionProofs(context.Context) error {
	p.proofLock.Lock()
	defer p.proofLock.Unlock()
	p.proofCache = make(map[attSelectionKey][]byte)
	return nil
}

func (p *localSelector) AttestationSelectionProof(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	idx, err := p.v.indexFromPubkey(pubKey)
	if err != nil {
		return nil, err
	}
	key := attSelectionKey{slot: slot, index: idx}
	sfKey := fmt.Sprintf("%d_%d", slot, idx)

	// Deduplicate concurrent signing for the same (slot, validator) — subscribeToSubnets
	// and RolesAt can race, and signing may involve a remote signer round-trip.
	result, err, _ := p.proofGroup.Do(sfKey, func() (any, error) {
		if cached, ok := p.getCachedProof(key); ok {
			return cached, nil
		}
		sig, err := p.v.signSlotWithSelectionProof(ctx, pubKey, slot)
		if err != nil {
			return nil, err
		}
		p.cacheProof(key, sig)
		return sig, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

func (p *localSelector) ClaimAggregateSlot(slot primitives.Slot, committeeIndex primitives.CommitteeIndex) bool {
	k := validatorSubnetSubscriptionKey(slot, committeeIndex)
	p.dedupLock.Lock()
	defer p.dedupLock.Unlock()
	if p.dedupCache.Contains(k) {
		return false
	}
	p.dedupCache.Add(k, true)
	return true
}

type syncSelectionProof struct {
	proof  []byte
	pubkey [fieldparams.BLSPubkeyLength]byte
}

// signSyncSelectionProofs fetches subcommittee indices and signs selection data for a single pubkey.
func (p *localSelector) signSyncSelectionProofs(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) ([]syncSelectionProof, error) {
	res, err := p.v.validatorClient.SyncSubcommitteeIndex(ctx, &ethpb.SyncSubcommitteeIndexRequest{
		PublicKey: pubKey[:],
		Slot:      slot,
	})
	if err != nil {
		return nil, errors.Wrap(err, "can't fetch sync subcommittee index")
	}
	proofs := make([]syncSelectionProof, 0, len(res.Indices))
	for _, index := range res.Indices {
		sig, err := p.v.signSyncSelectionData(ctx, pubKey, syncSubnet(uint64(index)), slot)
		if err != nil {
			return nil, errors.Wrap(err, "can't sign selection data")
		}
		proofs = append(proofs, syncSelectionProof{proof: sig, pubkey: pubKey})
	}
	return proofs, nil
}

func (p *localSelector) SyncCommitteeAggregators(ctx context.Context, slot primitives.Slot, pubkeys [][fieldparams.BLSPubkeyLength]byte) ([][fieldparams.BLSPubkeyLength]byte, error) {
	ctx, span := trace.StartSpan(ctx, "localSelector.SyncCommitteeAggregators")
	defer span.End()

	var selections []syncSelectionProof
	for _, pubKey := range pubkeys {
		proofs, err := p.signSyncSelectionProofs(ctx, slot, pubKey)
		if err != nil {
			return nil, err
		}
		selections = append(selections, proofs...)
	}

	var aggregators [][fieldparams.BLSPubkeyLength]byte
	for _, s := range selections {
		isAggregator, err := altair.IsSyncCommitteeAggregator(s.proof)
		if err != nil {
			return nil, errors.Wrap(err, "can't detect sync committee aggregator")
		}
		if isAggregator {
			aggregators = append(aggregators, s.pubkey)
		}
	}
	return aggregators, nil
}

func (p *localSelector) SyncCommitteeSelectionProofs(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, indexRes *ethpb.SyncSubcommitteeIndexResponse) ([][]byte, error) {
	ctx, span := trace.StartSpan(ctx, "localSelector.SyncCommitteeSelectionProofs")
	defer span.End()

	selectionProofs := make([][]byte, len(indexRes.Indices))
	for i, index := range indexRes.Indices {
		subnet := syncSubnet(uint64(index))
		sig, err := p.v.signSyncSelectionData(ctx, pubKey, subnet, slot)
		if err != nil {
			return nil, err
		}
		selectionProofs[i] = sig
	}
	return selectionProofs, nil
}

// distributedSelector coordinates with DVT middleware for selection proofs.
type distributedSelector struct {
	v             *validator
	attSelLock    sync.Mutex
	attSelections map[attSelectionKey]iface.BeaconCommitteeSelection
}

func newDistributedSelector(v *validator) *distributedSelector {
	return &distributedSelector{v: v, attSelections: make(map[attSelectionKey]iface.BeaconCommitteeSelection)}
}

func (p *distributedSelector) RefreshSelectionProofs(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "distributedSelector.RefreshSelectionProofs")
	defer span.End()

	var req []iface.BeaconCommitteeSelection
	for pk, duty := range p.v.duties.CurrentEpochDuties() {
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}
		slotSig, err := p.v.signSlotWithSelectionProof(ctx, pk, duty.AttesterSlot)
		if err != nil {
			return err
		}
		req = append(req, iface.BeaconCommitteeSelection{
			SelectionProof: slotSig,
			Slot:           duty.AttesterSlot,
			ValidatorIndex: duty.ValidatorIndex,
		})
	}

	resp, err := p.v.validatorClient.AggregatedSelections(ctx, req)
	if err != nil {
		return err
	}

	newSelections := make(map[attSelectionKey]iface.BeaconCommitteeSelection, len(resp))
	for _, s := range resp {
		newSelections[attSelectionKey{
			slot:  s.Slot,
			index: s.ValidatorIndex,
		}] = s
	}

	p.attSelLock.Lock()
	p.attSelections = newSelections
	p.attSelLock.Unlock()

	return nil
}

func (p *distributedSelector) AttestationSelectionProof(_ context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	idx, err := p.v.indexFromPubkey(pubKey)
	if err != nil {
		return nil, err
	}

	p.attSelLock.Lock()
	defer p.attSelLock.Unlock()

	s, ok := p.attSelections[attSelectionKey{slot: slot, index: idx}]
	if !ok {
		return nil, errors.Errorf("selection proof not found for slot=%d validator_index=%d", slot, idx)
	}
	return s.SelectionProof, nil
}

func (p *distributedSelector) ClaimAggregateSlot(_ primitives.Slot, _ primitives.CommitteeIndex) bool {
	return true
}

// SyncCommitteeAggregators returns all pubkeys immediately so that RolesAt does
// not block on DV middleware calls. The actual aggregated selection proof exchange
// happens later when SyncCommitteeSelectionProofs is called during duty execution.
// See https://github.com/OffchainLabs/prysm/issues/16362.
func (p *distributedSelector) SyncCommitteeAggregators(_ context.Context, _ primitives.Slot, pubkeys [][fieldparams.BLSPubkeyLength]byte) ([][fieldparams.BLSPubkeyLength]byte, error) {
	return pubkeys, nil
}

func (p *distributedSelector) SyncCommitteeSelectionProofs(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, indexRes *ethpb.SyncSubcommitteeIndexResponse) ([][]byte, error) {
	ctx, span := trace.StartSpan(ctx, "distributedSelector.SyncCommitteeSelectionProofs")
	defer span.End()

	idx, err := p.v.indexFromPubkey(pubKey)
	if err != nil {
		return nil, err
	}

	selectionProofs := make([][]byte, len(indexRes.Indices))
	selections := make([]iface.SyncCommitteeSelection, len(indexRes.Indices))
	for i, index := range indexRes.Indices {
		subnet := syncSubnet(uint64(index))
		sig, err := p.v.signSyncSelectionData(ctx, pubKey, subnet, slot)
		if err != nil {
			return nil, err
		}
		selectionProofs[i] = sig
		selections[i] = iface.SyncCommitteeSelection{
			SelectionProof:    sig,
			Slot:              slot,
			SubcommitteeIndex: primitives.CommitteeIndex(subnet),
			ValidatorIndex:    idx,
		}
	}

	if len(selections) > 0 {
		aggregated, err := p.v.validatorClient.AggregatedSyncSelections(ctx, selections)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get aggregated sync selections")
		}
		if len(aggregated) != len(selections) {
			return nil, errors.Errorf("aggregated sync selections length mismatch: got %d, want %d", len(aggregated), len(selections))
		}
		for i, s := range aggregated {
			selectionProofs[i] = s.SelectionProof
		}
	}

	return selectionProofs, nil
}
