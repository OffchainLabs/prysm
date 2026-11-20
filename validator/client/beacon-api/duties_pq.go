package beacon_api

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/validator/pqkeys"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const pqAggregatorSlotsKept = 4 * 32 // Slots of aggregators kept: the current and next epochs of the largest preset, with a margin.

var _ dutiesProvider = pqDutiesProvider{}

type (
	// pqDutiesProvider is the duties provider of the PQ fork.
	//
	// In the PQ fork, the beacon node identifies the validators of its duties by XMSS public key. This provider wraps
	// `inner`, and translates the XMSS public keys of the validators of the validator client to their BLS public keys.
	// It keeps the XMSS public key of the other validators.
	pqDutiesProvider struct {
		inner       dutiesProvider
		keys        *pqkeys.Registry
		aggregators *pqAggregators
	}

	// pqAggregators holds, by slot, the validators the beacon node selects as aggregators in their attester duties.
	// It is safe for concurrent use.
	pqAggregators struct {
		mu     sync.RWMutex
		bySlot map[primitives.Slot]map[primitives.ValidatorIndex]bool
	}
)

// AttesterDuties returns the attester duties of the validators of indices `validatorIndices` at `epoch`, identified
// by BLS public key for the validators of the validator client. It records the aggregators the beacon node selects.
func (p pqDutiesProvider) AttesterDuties(
	ctx context.Context,
	epoch primitives.Epoch,
	validatorIndices []primitives.ValidatorIndex,
) (*structs.GetAttesterDutiesResponse, error) {
	resp, err := p.inner.AttesterDuties(ctx, epoch, validatorIndices)
	if err != nil {
		return nil, fmt.Errorf("attester duties: %w", err)
	}

	for _, duty := range resp.Data {
		if duty.Pubkey, err = p.blsPubkey(duty.Pubkey); err != nil {
			return nil, fmt.Errorf("attester duty BLS public key: %w", err)
		}

		if err := p.aggregators.set(duty); err != nil {
			return nil, fmt.Errorf("set aggregator: %w", err)
		}
	}

	return resp, nil
}

// ProposerDuties returns the proposer duties of `epoch`, identified by BLS public key for the validators of the
// validator client.
func (p pqDutiesProvider) ProposerDuties(ctx context.Context, epoch primitives.Epoch) (*structs.GetProposerDutiesResponse, error) {
	resp, err := p.inner.ProposerDuties(ctx, epoch)
	if err != nil {
		return nil, fmt.Errorf("proposer duties: %w", err)
	}

	for _, duty := range resp.Data {
		if duty.Pubkey, err = p.blsPubkey(duty.Pubkey); err != nil {
			return nil, fmt.Errorf("proposer duty BLS public key: %w", err)
		}
	}

	return resp, nil
}

// SyncDuties returns no duty: the PQ fork has no sync committee.
func (pqDutiesProvider) SyncDuties(context.Context, primitives.Epoch, []primitives.ValidatorIndex) ([]*structs.SyncCommitteeDuty, error) {
	return []*structs.SyncCommitteeDuty{}, nil
}

// PTCDuties returns the PTC duties of the validators of indices `validatorIndices` at `epoch`, identified by BLS
// public key for the validators of the validator client.
func (p pqDutiesProvider) PTCDuties(
	ctx context.Context,
	epoch primitives.Epoch,
	validatorIndices []primitives.ValidatorIndex,
) (*structs.GetPTCDutiesResponse, error) {
	resp, err := p.inner.PTCDuties(ctx, epoch, validatorIndices)
	if err != nil {
		return nil, fmt.Errorf("PTC duties: %w", err)
	}

	for _, duty := range resp.Data {
		if duty.Pubkey, err = p.blsPubkey(duty.Pubkey); err != nil {
			return nil, fmt.Errorf("PTC duty BLS public key: %w", err)
		}
	}

	return resp, nil
}

// Committees returns the beacon committees of `epoch`. They identify the validators by index only.
func (p pqDutiesProvider) Committees(ctx context.Context, epoch primitives.Epoch) ([]*structs.Committee, error) {
	committees, err := p.inner.Committees(ctx, epoch)
	if err != nil {
		return nil, fmt.Errorf("committees: %w", err)
	}

	return committees, nil
}

// blsPubkey returns the hex encoded BLS public key of the validator whose hex encoded XMSS public key is `xmssPubkey`,
// if the validator client knows it, and `xmssPubkey` otherwise.
func (p pqDutiesProvider) blsPubkey(xmssPubkey string) (string, error) {
	xmssKey, err := bytesutil.DecodeHexWithLength(xmssPubkey, fieldparams.XMSSPubkeyLength)
	if err != nil {
		return "", fmt.Errorf("decode XMSS public key %s: %w", xmssPubkey, err)
	}

	blsKey, ok := p.keys.BLSPubkey([fieldparams.XMSSPubkeyLength]byte(xmssKey))
	if !ok {
		return xmssPubkey, nil
	}

	return hexutil.Encode(blsKey[:]), nil
}

// newPQAggregators returns an empty set of aggregators.
func newPQAggregators() *pqAggregators {
	return &pqAggregators{bySlot: make(map[primitives.Slot]map[primitives.ValidatorIndex]bool)}
}

// set records whether the validator of the attester duty `duty` aggregates at the slot of the duty.
func (a *pqAggregators) set(duty *structs.AttesterDuty) error {
	slot, err := strconv.ParseUint(duty.Slot, 10, 64)
	if err != nil {
		return fmt.Errorf("parse attester slot %s: %w", duty.Slot, err)
	}

	index, err := strconv.ParseUint(duty.ValidatorIndex, 10, 64)
	if err != nil {
		return fmt.Errorf("parse attester validator index %s: %w", duty.ValidatorIndex, err)
	}

	isAggregator := duty.IsAggregator != nil && *duty.IsAggregator

	a.mu.Lock()
	defer a.mu.Unlock()

	validators, ok := a.bySlot[primitives.Slot(slot)]
	if !ok {
		validators = make(map[primitives.ValidatorIndex]bool)
		a.bySlot[primitives.Slot(slot)] = validators
	}

	validators[primitives.ValidatorIndex(index)] = isAggregator

	// Forget the slots too old to be asked about.
	for known := range a.bySlot {
		if uint64(known)+pqAggregatorSlotsKept < slot {
			delete(a.bySlot, known)
		}
	}

	return nil
}

// isAggregator returns whether the beacon node selected the validator of index `index` as an aggregator at `slot`.
func (a *pqAggregators) isAggregator(slot primitives.Slot, index primitives.ValidatorIndex) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.bySlot[slot][index]
}
