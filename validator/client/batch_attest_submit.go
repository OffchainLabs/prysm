package client

import (
	"context"
	"slices"

	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

func (v *validator) batchCoordinator() *batchAttestationCoordinator {
	v.batchAttestationsLock.Lock()
	defer v.batchAttestationsLock.Unlock()
	if v.batchAttestations == nil {
		v.batchAttestations = newBatchAttestationCoordinator()
	}
	return v.batchAttestations
}

func (v *validator) localBatchAttesterDuties(slot primitives.Slot, committeeIndex primitives.CommitteeIndex) []localBatchAttesterDuty {
	v.dutiesLock.RLock()
	defer v.dutiesLock.RUnlock()
	if v.duties == nil || !v.duties.IsInitialized() {
		return nil
	}

	duties := make([]localBatchAttesterDuty, 0)
	for pk, duty := range v.duties.CurrentEpochDuties() {
		if duty == nil ||
			duty.AttesterSlot != slot ||
			duty.CommitteeIndex != committeeIndex ||
			duty.CommitteeLength == 0 {
			continue
		}
		duties = append(duties, localBatchAttesterDuty{
			pubKey:                  pk,
			validatorIndex:          duty.ValidatorIndex,
			validatorCommitteeIndex: duty.ValidatorCommitteeIndex,
		})
	}
	slices.SortFunc(duties, func(a, b localBatchAttesterDuty) int {
		switch {
		case a.validatorIndex < b.validatorIndex:
			return -1
		case a.validatorIndex > b.validatorIndex:
			return 1
		default:
			return 0
		}
	})
	return duties
}

func (v *validator) trySubmitBatchAttestation(
	ctx context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	duty *ethpb.ValidatorDuty,
	data *ethpb.AttestationData,
	sig []byte,
) (*ethpb.AttestResponse, bool, error) {
	if data == nil || duty == nil {
		return nil, false, nil
	}
	if !features.Get().EnableBatchAttestations {
		return nil, false, nil
	}
	cfg := params.BeaconConfig()
	if slots.ToEpoch(data.Slot) < cfg.BatchAttestationForkEpoch {
		return nil, false, nil
	}

	cohort := v.localBatchAttesterDuties(data.Slot, duty.CommitteeIndex)
	if len(cohort) < 2 {
		return nil, false, nil
	}
	batcher := cohort[0]
	seal, err := v.SignBatchSeal(ctx, pubKey, data.Slot, duty.CommitteeIndex, batcher.validatorIndex)
	if err != nil {
		return nil, false, errors.Wrap(err, "could not sign batch attestation seal")
	}
	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		return nil, false, errors.Wrap(err, "could not hash attestation data")
	}

	req := batchAttestationRequest{
		key: batchAttestationKey{
			slot:           data.Slot,
			committeeIndex: duty.CommitteeIndex,
			dataRoot:       dataRoot,
		},
		expected:        len(cohort),
		committeeLength: duty.CommitteeLength,
		batcher:         batcher.validatorIndex,
		batcherPubKey:   batcher.pubKey,
		data:            data,
		contribution: BatchContribution{
			AttesterIndex:        duty.ValidatorIndex,
			AttesterCommitteePos: int(duty.ValidatorCommitteeIndex),
			AttestationSignature: sig,
			Seal:                 seal,
		},
	}
	return v.batchCoordinator().contribute(ctx, req, func(ctx context.Context, snapshot batchAttestationSnapshot) (*ethpb.AttestResponse, error) {
		return v.SubmitBatchedAttestation(
			ctx,
			snapshot.batcherPubKey,
			snapshot.batcher,
			snapshot.committeeIndex,
			snapshot.committeeLength,
			snapshot.data,
			snapshot.contributions,
		)
	})
}
