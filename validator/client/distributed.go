package client

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/pkg/errors"
)

type attSelectionKey struct {
	slot  primitives.Slot
	index primitives.ValidatorIndex
}

func (v *validator) aggregatedSelectionProofs(ctx context.Context, duties *ethpb.ValidatorDutiesContainer) error {
	ctx, span := trace.StartSpan(ctx, "validator.aggregatedSelectionProofs")
	defer span.End()

	v.attSelectionLock.Lock()
	defer v.attSelectionLock.Unlock()

	v.attSelections = make(map[attSelectionKey]iface.BeaconCommitteeSelection)

	var req []iface.BeaconCommitteeSelection
	for _, duty := range duties.CurrentEpochDuties {
		if duty.Status != ethpb.ValidatorStatus_ACTIVE && duty.Status != ethpb.ValidatorStatus_EXITING {
			continue
		}

		pk := bytesutil.ToBytes48(duty.PublicKey)
		slotSig, err := v.signSlotWithSelectionProof(ctx, pk, duty.AttesterSlot)
		if err != nil {
			return err
		}

		req = append(req, iface.BeaconCommitteeSelection{
			SelectionProof: slotSig,
			Slot:           duty.AttesterSlot,
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

func (v *validator) attSelection(key attSelectionKey) ([]byte, error) {
	v.attSelectionLock.Lock()
	defer v.attSelectionLock.Unlock()

	s, ok := v.attSelections[key]
	if !ok {
		return nil, errors.Errorf("selection proof not found for the given slot=%d and validator_index=%d", key.slot, key.index)
	}

	return s.SelectionProof, nil
}
