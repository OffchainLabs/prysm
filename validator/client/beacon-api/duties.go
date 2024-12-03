package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/validator"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"golang.org/x/sync/errgroup"
)

type dutiesProvider interface {
	AttesterDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*structs.AttesterDuty, error)
	ProposerDuties(ctx context.Context, epoch primitives.Epoch) ([]*structs.ProposerDuty, error)
	SyncDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*structs.SyncCommitteeDuty, error)
	Committees(ctx context.Context, epoch primitives.Epoch) ([]*structs.Committee, error)
}

type beaconApiDutiesProvider struct {
	jsonRestHandler JsonRestHandler
}

type committeeIndexSlotPair struct {
	committeeIndex primitives.CommitteeIndex
	slot           primitives.Slot
}

type validatorForDuty struct {
	pubkey []byte
	index  primitives.ValidatorIndex
	status ethpb.ValidatorStatus
}

func (c *beaconApiValidatorClient) duties(ctx context.Context, in *ethpb.DutiesRequest) (*ethpb.DutiesResponse, error) {
	vals, err := c.validatorsForDuties(ctx, in.PublicKeys)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validators for duties")
	}

	// Sync committees are an Altair feature
	fetchSyncDuties := in.Epoch >= params.BeaconConfig().AltairForkEpoch

	errCh := make(chan error, 1)

	var currentEpochDuties []*ethpb.DutiesResponse_Duty
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

	return &ethpb.DutiesResponse{
		CurrentEpochDuties: currentEpochDuties,
		NextEpochDuties:    nextEpochDuties,
	}, nil
}

func (c *beaconApiValidatorClient) dutiesForEpoch(
	ctx context.Context,
	epoch primitives.Epoch,
	vals []validatorForDuty,
	fetchSyncDuties bool,
) ([]*ethpb.DutiesResponse_Duty, error) {
	indices := make([]primitives.ValidatorIndex, len(vals))
	for i, v := range vals {
		indices[i] = v.index
	}

	// Below variables MUST NOT be used in the main function before wg.Wait().
	// This is because they are populated in goroutines and wg.Wait()
	// will return only once all goroutines finish their execution.

	// Mapping from a validator index to its attesting committee's index and slot
	attesterDutiesMapping := make(map[primitives.ValidatorIndex]committeeIndexSlotPair)
	// Set containing all validator indices that are part of a sync committee for this epoch
	syncDutiesMapping := make(map[primitives.ValidatorIndex]bool)
	// Mapping from a validator index to its proposal slot
	proposerDutySlots := make(map[primitives.ValidatorIndex][]primitives.Slot)
	// Mapping from the {committeeIndex, slot} to each of the committee's validator indices
	committeeMapping := make(map[committeeIndexSlotPair][]primitives.ValidatorIndex)

	var wg errgroup.Group

	wg.Go(func() error {
		attesterDuties, err := c.dutiesProvider.AttesterDuties(ctx, epoch, indices)
		if err != nil {
			return errors.Wrapf(err, "failed to get attester duties for epoch `%d`", epoch)
		}

		for _, attesterDuty := range attesterDuties {
			validatorIndex, err := strconv.ParseUint(attesterDuty.ValidatorIndex, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse attester validator index `%s`", attesterDuty.ValidatorIndex)
			}
			slot, err := strconv.ParseUint(attesterDuty.Slot, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse attester slot `%s`", attesterDuty.Slot)
			}
			committeeIndex, err := strconv.ParseUint(attesterDuty.CommitteeIndex, 10, 64)
			if err != nil {
				return errors.Wrapf(err, "failed to parse attester committee index `%s`", attesterDuty.CommitteeIndex)
			}
			attesterDutiesMapping[primitives.ValidatorIndex(validatorIndex)] = committeeIndexSlotPair{
				slot:           primitives.Slot(slot),
				committeeIndex: primitives.CommitteeIndex(committeeIndex),
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

	committees, err := c.dutiesProvider.Committees(ctx, epoch)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get committees for epoch `%d`", epoch)
	}
	slotCommittees := make(map[string]uint64)
	for _, c := range committees {
		n, ok := slotCommittees[c.Slot]
		if !ok {
			n = 0
		}
		slotCommittees[c.Slot] = n + 1
	}

	for _, committee := range committees {
		committeeIndex, err := strconv.ParseUint(committee.Index, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse committee index `%s`", committee.Index)
		}
		slot, err := strconv.ParseUint(committee.Slot, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse slot `%s`", committee.Slot)
		}
		validatorIndices := make([]primitives.ValidatorIndex, len(committee.Validators))
		for index, validatorIndexString := range committee.Validators {
			validatorIndex, err := strconv.ParseUint(validatorIndexString, 10, 64)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse committee validator index `%s`", validatorIndexString)
			}
			validatorIndices[index] = primitives.ValidatorIndex(validatorIndex)
		}
		key := committeeIndexSlotPair{
			committeeIndex: primitives.CommitteeIndex(committeeIndex),
			slot:           primitives.Slot(slot),
		}
		committeeMapping[key] = validatorIndices
	}

	if err = wg.Wait(); err != nil {
		return nil, err
	}

	// TODO: add EIP7805 duty

	duties := make([]*ethpb.DutiesResponse_Duty, len(vals))
	for i, v := range vals {
		var (
			attesterSlot              primitives.Slot
			committeeIndex            primitives.CommitteeIndex
			committeeValidatorIndices []primitives.ValidatorIndex
		)

		if committeeMappingKey, ok := attesterDutiesMapping[v.index]; ok {
			committeeIndex = committeeMappingKey.committeeIndex
			attesterSlot = committeeMappingKey.slot

			if committeeValidatorIndices, ok = committeeMapping[committeeMappingKey]; !ok {
				return nil, errors.Errorf("failed to find validators for committee index `%d` and slot `%d`", committeeIndex, attesterSlot)
			}
		}

		duties[i] = &ethpb.DutiesResponse_Duty{
			Committee:        committeeValidatorIndices,
			CommitteeIndex:   committeeIndex,
			AttesterSlot:     attesterSlot,
			ProposerSlots:    proposerDutySlots[v.index],
			PublicKey:        v.pubkey,
			Status:           v.status,
			ValidatorIndex:   v.index,
			IsSyncCommittee:  syncDutiesMapping[v.index],
			CommitteesAtSlot: slotCommittees[strconv.FormatUint(uint64(attesterSlot), 10)],
		}
	}

	return duties, nil
}

func (c *beaconApiValidatorClient) validatorsForDuties(ctx context.Context, pubkeys [][]byte) ([]validatorForDuty, error) {
	vals := make([]validatorForDuty, 0, len(pubkeys))
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
		val := validatorForDuty{}

		validatorIndex, err := strconv.ParseUint(validatorContainer.Index, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator index %s", validatorContainer.Index)
		}
		val.index = primitives.ValidatorIndex(validatorIndex)

		stringPubkey := validatorContainer.Validator.Pubkey
		pubkey, ok := stringPubkeysToPubkeys[stringPubkey]
		if !ok {
			return nil, errors.Wrapf(err, "returned public key %s not requested", stringPubkey)
		}
		val.pubkey = pubkey

		status, ok := beaconAPITogRPCValidatorStatus[validatorContainer.Status]
		if !ok {
			return nil, errors.New("invalid validator status " + validatorContainer.Status)
		}
		val.status = status

		vals = append(vals, val)
	}

	return vals, nil
}

// GetCommittees retrieves the committees for the given epoch
func (c beaconApiDutiesProvider) Committees(ctx context.Context, epoch primitives.Epoch) ([]*structs.Committee, error) {
	committeeParams := url.Values{}
	committeeParams.Add("epoch", strconv.FormatUint(uint64(epoch), 10))
	committeesRequest := buildURL("/eth/v1/beacon/states/head/committees", committeeParams)

	var stateCommittees structs.GetCommitteesResponse
	if err := c.jsonRestHandler.Get(ctx, committeesRequest, &stateCommittees); err != nil {
		return nil, err
	}

	if stateCommittees.Data == nil {
		return nil, errors.New("state committees data is nil")
	}

	for index, committee := range stateCommittees.Data {
		if committee == nil {
			return nil, errors.Errorf("committee at index `%d` is nil", index)
		}
	}

	return stateCommittees.Data, nil
}

// GetAttesterDuties retrieves the attester duties for the given epoch and validatorIndices
func (c beaconApiDutiesProvider) AttesterDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*structs.AttesterDuty, error) {
	jsonValidatorIndices := make([]string, len(validatorIndices))
	for index, validatorIndex := range validatorIndices {
		jsonValidatorIndices[index] = strconv.FormatUint(uint64(validatorIndex), 10)
	}

	validatorIndicesBytes, err := json.Marshal(jsonValidatorIndices)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal validator indices")
	}

	attesterDuties := &structs.GetAttesterDutiesResponse{}
	if err = c.jsonRestHandler.Post(
		ctx,
		fmt.Sprintf("/eth/v1/validator/duties/attester/%d", epoch),
		nil,
		bytes.NewBuffer(validatorIndicesBytes),
		attesterDuties,
	); err != nil {
		return nil, err
	}

	for index, attesterDuty := range attesterDuties.Data {
		if attesterDuty == nil {
			return nil, errors.Errorf("attester duty at index `%d` is nil", index)
		}
	}

	return attesterDuties.Data, nil
}

// GetProposerDuties retrieves the proposer duties for the given epoch
func (c beaconApiDutiesProvider) ProposerDuties(ctx context.Context, epoch primitives.Epoch) ([]*structs.ProposerDuty, error) {
	proposerDuties := structs.GetProposerDutiesResponse{}
	if err := c.jsonRestHandler.Get(ctx, fmt.Sprintf("/eth/v1/validator/duties/proposer/%d", epoch), &proposerDuties); err != nil {
		return nil, err
	}

	if proposerDuties.Data == nil {
		return nil, errors.New("proposer duties data is nil")
	}

	for index, proposerDuty := range proposerDuties.Data {
		if proposerDuty == nil {
			return nil, errors.Errorf("proposer duty at index `%d` is nil", index)
		}
	}

	return proposerDuties.Data, nil
}

// GetSyncDuties retrieves the sync committee duties for the given epoch and validatorIndices
func (c beaconApiDutiesProvider) SyncDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*structs.SyncCommitteeDuty, error) {
	jsonValidatorIndices := make([]string, len(validatorIndices))
	for index, validatorIndex := range validatorIndices {
		jsonValidatorIndices[index] = strconv.FormatUint(uint64(validatorIndex), 10)
	}

	validatorIndicesBytes, err := json.Marshal(jsonValidatorIndices)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal validator indices")
	}

	syncDuties := structs.GetSyncCommitteeDutiesResponse{}
	if err = c.jsonRestHandler.Post(
		ctx,
		fmt.Sprintf("/eth/v1/validator/duties/sync/%d", epoch),
		nil,
		bytes.NewBuffer(validatorIndicesBytes),
		&syncDuties,
	); err != nil {
		return nil, err
	}

	if syncDuties.Data == nil {
		return nil, errors.New("sync duties data is nil")
	}

	for index, syncDuty := range syncDuties.Data {
		if syncDuty == nil {
			return nil, errors.Errorf("sync duty at index `%d` is nil", index)
		}
	}

	return syncDuties.Data, nil
}
