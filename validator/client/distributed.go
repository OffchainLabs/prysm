package client

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/pkg/errors"
)

type attSelectionKey struct {
	slot  primitives.Slot
	index primitives.ValidatorIndex
}

// aggregatedSelectionProofs pre-computes selection proofs for distributed validators
// by batch-sending partial signatures to DVT middleware (e.g. Charon) which returns
// aggregated threshold signatures. Must run before subscribeToSubnets so that
// isAggregator can look up the pre-computed proofs.
func (v *validator) aggregatedSelectionProofs(ctx context.Context, ds *dutyStore) error {
	ctx, span := trace.StartSpan(ctx, "validator.aggregatedSelectionProofs")
	defer span.End()

	v.attSelectionLock.Lock()
	defer v.attSelectionLock.Unlock()

	v.attSelections = make(map[attSelectionKey]iface.BeaconCommitteeSelection)

	var req []iface.BeaconCommitteeSelection
	for _, duty := range ds.CurrentEpochDuties() {
		pk := bytesutil.ToBytes48(duty.Pubkey)
		slotSig, err := v.signSlotWithSelectionProof(ctx, pk, duty.Slot)
		if err != nil {
			return err
		}

		req = append(req, iface.BeaconCommitteeSelection{
			SelectionProof: slotSig,
			Slot:           duty.Slot,
			ValidatorIndex: duty.ValidatorIndex,
		})
	}

	resp, err := v.validatorClient.AggregatedSelections(ctx, req)
	if err != nil {
		return err
	}

	for _, s := range resp {
		v.attSelections[attSelectionKey{
			slot:  s.Slot,
			index: s.ValidatorIndex,
		}] = s
	}

	return nil
}

// attSelection returns the pre-computed selection proof for a distributed validator.
func (v *validator) attSelection(key attSelectionKey) ([]byte, error) {
	v.attSelectionLock.Lock()
	defer v.attSelectionLock.Unlock()

	s, ok := v.attSelections[key]
	if !ok {
		return nil, errors.Errorf("selection proof not found for the given slot=%d and validator_index=%d", key.slot, key.index)
	}

	return s.SelectionProof, nil
}
