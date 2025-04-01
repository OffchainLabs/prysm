package validator_api

import (
	"context"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/shared_providers"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"golang.org/x/sync/errgroup"
)

func (c *beaconApiValidatorClient) duties(ctx context.Context, in *ethpb.DutiesRequest) (*ethpb.ValidatorDutiesContainer, error) {
	vals, err := c.validatorsForDuties(ctx, in.PublicKeys)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validators for duties")
	}

	// Sync committees are an Altair feature
	fetchSyncDuties := in.Epoch >= params.BeaconConfig().AltairForkEpoch

	errCh := make(chan error, 1)

	var currentEpochDuties []*ethpb.ValidatorDuty
	go func() {
		currentEpochDuties, err = c.dutiesForEpoch(ctx, in.Epoch, vals, fetchSyncDuties)
		if err != nil {
			errCh <- errors.Wrapf(err, "failed to get duties for current epoch `%d`", in.Epoch)
			return
		}
		errCh <- nil
	}()

	nextEpochDuties, err := c.dutiesForEpoch(ctx, in.Epoch+1, vals, fetchSyncDuties)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get duties for next epoch `%d`", in.Epoch+1)
	}

	if err = <-errCh; err != nil {
		return nil, err
	}

	return &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: currentEpochDuties,
		NextEpochDuties:    nextEpochDuties,
	}, nil
}

func (c *beaconApiValidatorClient) dutiesForEpoch(
	ctx context.Context,
	epoch primitives.Epoch,
	vals []shared_providers.ValidatorForDuty,
	fetchSyncDuties bool,
) ([]*ethpb.ValidatorDuty, error) {
	indices := make([]primitives.ValidatorIndex, len(vals))
	for i, v := range vals {
		indices[i] = v.Index
	}

	// Below variables MUST NOT be used in the main function before wg.Wait().
	// This is because they are populated in goroutines and wg.Wait()
	// will return only once all goroutines finish their execution.

	// Mapping from a validator index to its attesting committee's index and slot
	attesterDutiesMapping := make(map[primitives.ValidatorIndex]shared_providers.AttesterDuty)
	// Set containing all validator indices that are part of a sync committee for this epoch
	syncDutiesMapping := make(map[primitives.ValidatorIndex]bool)
	// Mapping from a validator index to its proposal slot
	proposerDutySlots := make(map[primitives.ValidatorIndex][]primitives.Slot)

	var wg errgroup.Group

	wg.Go(func() error {
		attesterDuties, err := c.dutiesProvider.AttesterDuties(ctx, epoch, indices)
		if err != nil {
			return errors.Wrapf(err, "failed to get attester duties for epoch `%d`", epoch)
		}

		for _, duty := range attesterDuties {
			validatorIndex, err := strconv.ParseUint(duty.ValidatorIndex, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse attester validator index `%s`", duty.ValidatorIndex)
			}
			slot, err := strconv.ParseUint(duty.Slot, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse attester slot `%s`", duty.Slot)
			}
			committeeIndex, err := strconv.ParseUint(duty.CommitteeIndex, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse attester committee index `%s`", duty.CommitteeIndex)
			}
			committeeLength, err := strconv.ParseUint(duty.CommitteeLength, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse attester committee length `%s`", duty.CommitteeLength)
			}
			validatorCommitteeIndex, err := strconv.ParseUint(duty.ValidatorCommitteeIndex, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse attester validator committee index `%s`", duty.ValidatorCommitteeIndex)
			}
			committeesAtSlot, err := strconv.ParseUint(duty.CommitteesAtSlot, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse attester committees at slot `%s`", duty.CommitteesAtSlot)
			}
			attesterDutiesMapping[primitives.ValidatorIndex(validatorIndex)] = shared_providers.AttesterDuty{
				Slot:                    primitives.Slot(slot),
				CommitteeIndex:          primitives.CommitteeIndex(committeeIndex),
				CommitteeLength:         committeeLength,
				ValidatorCommitteeIndex: validatorCommitteeIndex,
				CommitteesAtSlot:        committeesAtSlot,
			}
		}
		return nil
	})

	if fetchSyncDuties {
		wg.Go(func() error {
			syncDuties, err := c.dutiesProvider.SyncDuties(ctx, epoch, indices)
			if err != nil {
				return errors.Wrapf(err, "failed to get sync duties for epoch `%d`", epoch)
			}
			for _, syncDuty := range syncDuties {
				validatorIndex, err := strconv.ParseUint(syncDuty.ValidatorIndex, 10, 64)
				if err != nil {
					return errors.Wrapf(err, "failed to parse sync validator index `%s`", syncDuty.ValidatorIndex)
				}
				syncDutiesMapping[primitives.ValidatorIndex(validatorIndex)] = true
			}
			return nil
		})
	}

	wg.Go(func() error {
		proposerDuties, err := c.dutiesProvider.ProposerDuties(ctx, epoch)
		if err != nil {
			return errors.Wrapf(err, "failed to get proposer duties for epoch `%d`", epoch)
		}

		for _, proposerDuty := range proposerDuties {
			validatorIndex, err := strconv.ParseUint(proposerDuty.ValidatorIndex, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse proposer validator index `%s`", proposerDuty.ValidatorIndex)
			}
			slot, err := strconv.ParseUint(proposerDuty.Slot, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse proposer slot `%s`", proposerDuty.Slot)
			}
			proposerDutySlots[primitives.ValidatorIndex(validatorIndex)] =
				append(proposerDutySlots[primitives.ValidatorIndex(validatorIndex)], primitives.Slot(slot))
		}
		return nil
	})

	if err := wg.Wait(); err != nil {
		return nil, err
	}

	duties := make([]*ethpb.ValidatorDuty, len(vals))
	for i, v := range vals {
		att, ok := attesterDutiesMapping[v.Index]
		if !ok {
			log.Debugf("failed to find attester duty for validator `%d`", v.Index)
		}

		duties[i] = &ethpb.ValidatorDuty{
			ValidatorCommitteeIndex: att.ValidatorCommitteeIndex,
			CommitteeLength:         att.CommitteeLength,
			CommitteeIndex:          att.CommitteeIndex,
			AttesterSlot:            att.Slot,
			CommitteesAtSlot:        att.CommitteesAtSlot,
			ProposerSlots:           proposerDutySlots[v.Index],
			PublicKey:               v.Pubkey,
			Status:                  v.Status,
			ValidatorIndex:          v.Index,
			IsSyncCommittee:         syncDutiesMapping[v.Index],
		}
	}

	return duties, nil
}

func (c *beaconApiValidatorClient) validatorsForDuties(ctx context.Context, pubkeys [][]byte) ([]shared_providers.ValidatorForDuty, error) {
	vals := make([]shared_providers.ValidatorForDuty, 0, len(pubkeys))
	stringPubkeysToPubkeys := make(map[string][]byte, len(pubkeys))
	stringPubkeys := make([]string, len(pubkeys))

	for i, pk := range pubkeys {
		stringPk := hexutil.Encode(pk)
		stringPubkeysToPubkeys[stringPk] = pk
		stringPubkeys[i] = stringPk
	}

	statusesWithDuties := []string{validator.ActiveOngoing.String(), validator.ActiveExiting.String()}
	stateValidatorsResponse, err := c.stateValidatorsProvider.StateValidators(ctx, stringPubkeys, nil, statusesWithDuties)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get state validators")
	}

	for _, validatorContainer := range stateValidatorsResponse.Data {
		val := shared_providers.ValidatorForDuty{}

		validatorIndex, err := strconv.ParseUint(validatorContainer.Index, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator index %s", validatorContainer.Index)
		}
		val.Index = primitives.ValidatorIndex(validatorIndex)

		stringPubkey := validatorContainer.Validator.Pubkey
		pubkey, ok := stringPubkeysToPubkeys[stringPubkey]
		if !ok {
			return nil, errors.Wrapf(err, "returned public key %s not requested", stringPubkey)
		}
		val.Pubkey = pubkey

		status, ok := beaconAPITogRPCValidatorStatus[validatorContainer.Status]
		if !ok {
			return nil, errors.New("invalid validator status " + validatorContainer.Status)
		}
		val.Status = status

		vals = append(vals, val)
	}

	return vals, nil
}
