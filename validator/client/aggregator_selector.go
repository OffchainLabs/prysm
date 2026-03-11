package client

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
)

// aggregatorSelector abstracts selection proof generation and aggregation decisions.
// In local mode, proofs are signed with the local keymanager.
// In distributed mode, partial proofs are sent to DVT middleware for aggregation.
type aggregatorSelector interface {
	RefreshSelectionProofs(ctx context.Context, duties *ethpb.ValidatorDutiesContainer) error
	AttestationSelectionProof(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, validatorIndex primitives.ValidatorIndex) ([]byte, error)
	// ClaimAggregateSlot atomically checks and claims the right to aggregate for
	// a (slot, committee) pair. Returns false if already claimed. In distributed
	// mode the middleware handles dedup, so this always returns true.
	ClaimAggregateSlot(slot primitives.Slot, committeeIndex primitives.CommitteeIndex) bool
	SyncCommitteeAggregators(ctx context.Context, slot primitives.Slot, pubkeys [][fieldparams.BLSPubkeyLength]byte) ([][fieldparams.BLSPubkeyLength]byte, error)
	SyncCommitteeSelectionProofs(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, indexRes *ethpb.SyncSubcommitteeIndexResponse, validatorIndex primitives.ValidatorIndex) ([][]byte, error)
}

// localSelector computes selection proofs using the local keymanager.
type localSelector struct {
	v          *validator
	dedupLock  sync.Mutex
	dedupCache *lru.Cache
	proofLock  sync.Mutex
	proofCache map[attSelectionKey][]byte
}

func newLocalSelector(v *validator, dedupCache *lru.Cache) *localSelector {
	return &localSelector{v: v, dedupCache: dedupCache, proofCache: make(map[attSelectionKey][]byte)}
}

func (p *localSelector) RefreshSelectionProofs(context.Context, *ethpb.ValidatorDutiesContainer) error {
	p.proofLock.Lock()
	p.proofCache = make(map[attSelectionKey][]byte)
	p.proofLock.Unlock()
	return nil
}

func (p *localSelector) AttestationSelectionProof(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, validatorIndex primitives.ValidatorIndex) ([]byte, error) {
	key := attSelectionKey{slot: slot, index: validatorIndex}

	p.proofLock.Lock()
	if cached, ok := p.proofCache[key]; ok {
		p.proofLock.Unlock()
		return cached, nil
	}
	p.proofLock.Unlock()

	sig, err := p.v.signSlotWithSelectionProof(ctx, pubKey, slot)
	if err != nil {
		return nil, err
	}

	p.proofLock.Lock()
	p.proofCache[key] = sig
	p.proofLock.Unlock()

	return sig, nil
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

func (p *localSelector) SyncCommitteeAggregators(ctx context.Context, slot primitives.Slot, pubkeys [][fieldparams.BLSPubkeyLength]byte) ([][fieldparams.BLSPubkeyLength]byte, error) {
	ctx, span := trace.StartSpan(ctx, "localSelector.SyncCommitteeAggregators")
	defer span.End()

	type selectionWithPubkey struct {
		proof  []byte
		pubkey [fieldparams.BLSPubkeyLength]byte
	}
	var selections []selectionWithPubkey
	for _, pubKey := range pubkeys {
		res, err := p.v.validatorClient.SyncSubcommitteeIndex(ctx, &ethpb.SyncSubcommitteeIndexRequest{
			PublicKey: pubKey[:],
			Slot:      slot,
		})
		if err != nil {
			return nil, errors.Wrap(err, "can't fetch sync subcommittee index")
		}
		for _, index := range res.Indices {
			subCommitteeSize := params.BeaconConfig().SyncCommitteeSize / params.BeaconConfig().SyncCommitteeSubnetCount
			subnet := uint64(index) / subCommitteeSize
			sig, err := p.v.signSyncSelectionData(ctx, pubKey, subnet, slot)
			if err != nil {
				return nil, errors.Wrap(err, "can't sign selection data")
			}
			selections = append(selections, selectionWithPubkey{proof: sig, pubkey: pubKey})
		}
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

func (p *localSelector) SyncCommitteeSelectionProofs(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, indexRes *ethpb.SyncSubcommitteeIndexResponse, _ primitives.ValidatorIndex) ([][]byte, error) {
	ctx, span := trace.StartSpan(ctx, "localSelector.SyncCommitteeSelectionProofs")
	defer span.End()

	cfg := params.BeaconConfig()
	selectionProofs := make([][]byte, len(indexRes.Indices))
	for i, index := range indexRes.Indices {
		subnet := uint64(index) / (cfg.SyncCommitteeSize / cfg.SyncCommitteeSubnetCount)
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

type attSelectionKey struct {
	slot  primitives.Slot
	index primitives.ValidatorIndex
}

func (p *distributedSelector) RefreshSelectionProofs(ctx context.Context, duties *ethpb.ValidatorDutiesContainer) error {
	ctx, span := trace.StartSpan(ctx, "distributedSelector.RefreshSelectionProofs")
	defer span.End()

	p.attSelLock.Lock()
	defer p.attSelLock.Unlock()

	p.attSelections = make(map[attSelectionKey]iface.BeaconCommitteeSelection)

	var req []iface.BeaconCommitteeSelection
	for _, duty := range duties.CurrentEpochDuties {
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}
		pk := bytesutil.ToBytes48(duty.PublicKey)
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

	for _, s := range resp {
		p.attSelections[attSelectionKey{
			slot:  s.Slot,
			index: s.ValidatorIndex,
		}] = s
	}

	return nil
}

func (p *distributedSelector) AttestationSelectionProof(_ context.Context, slot primitives.Slot, _ [fieldparams.BLSPubkeyLength]byte, validatorIndex primitives.ValidatorIndex) ([]byte, error) {
	p.attSelLock.Lock()
	defer p.attSelLock.Unlock()

	s, ok := p.attSelections[attSelectionKey{slot: slot, index: validatorIndex}]
	if !ok {
		return nil, errors.Errorf("selection proof not found for slot=%d validator_index=%d", slot, validatorIndex)
	}
	return s.SelectionProof, nil
}

func (p *distributedSelector) ClaimAggregateSlot(_ primitives.Slot, _ primitives.CommitteeIndex) bool {
	return true
}

func (p *distributedSelector) SyncCommitteeAggregators(_ context.Context, _ primitives.Slot, pubkeys [][fieldparams.BLSPubkeyLength]byte) ([][fieldparams.BLSPubkeyLength]byte, error) {
	return pubkeys, nil
}

func (p *distributedSelector) SyncCommitteeSelectionProofs(ctx context.Context, slot primitives.Slot, pubKey [fieldparams.BLSPubkeyLength]byte, indexRes *ethpb.SyncSubcommitteeIndexResponse, validatorIndex primitives.ValidatorIndex) ([][]byte, error) {
	ctx, span := trace.StartSpan(ctx, "distributedSelector.SyncCommitteeSelectionProofs")
	defer span.End()

	cfg := params.BeaconConfig()
	selectionProofs := make([][]byte, len(indexRes.Indices))
	selections := make([]iface.SyncCommitteeSelection, len(indexRes.Indices))
	for i, index := range indexRes.Indices {
		subnet := uint64(index) / (cfg.SyncCommitteeSize / cfg.SyncCommitteeSubnetCount)
		sig, err := p.v.signSyncSelectionData(ctx, pubKey, subnet, slot)
		if err != nil {
			return nil, err
		}
		selectionProofs[i] = sig
		selections[i] = iface.SyncCommitteeSelection{
			SelectionProof:    sig,
			Slot:              slot,
			SubcommitteeIndex: primitives.CommitteeIndex(subnet),
			ValidatorIndex:    validatorIndex,
		}
	}

	if len(selections) > 0 {
		aggregated, err := p.v.validatorClient.AggregatedSyncSelections(ctx, selections)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get aggregated sync selections")
		}
		for i, s := range aggregated {
			selectionProofs[i] = s.SelectionProof
		}
	}

	return selectionProofs, nil
}
