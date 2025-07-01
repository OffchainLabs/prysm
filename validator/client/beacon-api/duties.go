package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/api/apiutil"
	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/consensus-types/validator"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type dutiesProvider interface {
	AttesterDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) (*structs.GetAttesterDutiesResponse, error)
	ProposerDuties(ctx context.Context, epoch primitives.Epoch) (*structs.GetProposerDutiesResponse, error)
	SyncDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*structs.SyncCommitteeDuty, error)
	Committees(ctx context.Context, epoch primitives.Epoch) ([]*structs.Committee, error)
}

type beaconApiDutiesProvider struct {
	jsonRestHandler RestHandler
}

type attesterDuty struct {
	committeeIndex          primitives.CommitteeIndex
	slot                    primitives.Slot
	committeeLength         uint64
	validatorCommitteeIndex uint64
	committeesAtSlot        uint64
}

type validatorForDuty struct {
	pubkey []byte
	index  primitives.ValidatorIndex
	status ethpb.ValidatorStatus
}

func (c *beaconApiValidatorClient) duties(ctx context.Context, in *ethpb.DutiesRequest) (*ethpb.ValidatorDutiesContainer, error) {
	vals, err := c.validatorsForDuties(ctx, in.PublicKeys)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validators for duties")
	}

	// Sync committees are an Altair feature
	fetchSyncDuties := in.Epoch >= params.BeaconConfig().AltairForkEpoch

	errCh := make(chan error, 1)

	currentEpochDuties := &ethpb.ValidatorDutiesContainer{}
	go func() {
		if err := c.dutiesForEpoch(ctx, currentEpochDuties, in.Epoch, vals, fetchSyncDuties); err != nil {
			errCh <- errors.Wrapf(err, "failed to get duties for current epoch `%d`", in.Epoch)
			return
		}
		errCh <- nil
	}()

	nextEpochDuties := &ethpb.ValidatorDutiesContainer{}
	if err := c.dutiesForEpoch(ctx, nextEpochDuties, in.Epoch+1, vals, fetchSyncDuties); err != nil {
		return nil, errors.Wrapf(err, "failed to get duties for next epoch `%d`", in.Epoch+1)
	}

	if err = <-errCh; err != nil {
		return nil, err
	}

	return &ethpb.ValidatorDutiesContainer{
		PrevDependentRoot:  currentEpochDuties.PrevDependentRoot,
		CurrDependentRoot:  currentEpochDuties.CurrDependentRoot,
		CurrentEpochDuties: currentEpochDuties.CurrentEpochDuties,
		NextEpochDuties:    nextEpochDuties.CurrentEpochDuties,
	}, nil
}

func (c *beaconApiValidatorClient) dutiesForEpoch(
	ctx context.Context,
	dutiesContainer *ethpb.ValidatorDutiesContainer,
	epoch primitives.Epoch,
	vals []validatorForDuty,
	fetchSyncDuties bool,
) error {
	indices := make([]primitives.ValidatorIndex, len(vals))
	for i, v := range vals {
		indices[i] = v.index
	}

	// Below variables MUST NOT be used in the main function before wg.Wait().
	// This is because they are populated in goroutines and wg.Wait()
	// will return only once all goroutines finish their execution.

	// Mapping from a validator index to its attesting committee's index and slot
	attesterDutiesMapping := make(map[primitives.ValidatorIndex]attesterDuty)
	// Set containing all validator indices that are part of a sync committee for this epoch
	syncDutiesMapping := make(map[primitives.ValidatorIndex]bool)
	// Mapping from a validator index to its proposal slot
	proposerDutySlots := make(map[primitives.ValidatorIndex][]primitives.Slot)

	var wg errgroup.Group

	var attesterDutiesContainer *structs.GetAttesterDutiesResponse
	var err error
	wg.Go(func() error {
		attesterDutiesContainer, err = c.dutiesProvider.AttesterDuties(ctx, epoch, indices)
		if err != nil {
			return errors.Wrapf(err, "failed to get attester duties for epoch `%d`", epoch)
		}

		for _, duty := range attesterDutiesContainer.Data {
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
			attesterDutiesMapping[primitives.ValidatorIndex(validatorIndex)] = attesterDuty{
				slot:                    primitives.Slot(slot),
				committeeIndex:          primitives.CommitteeIndex(committeeIndex),
				committeeLength:         committeeLength,
				validatorCommitteeIndex: validatorCommitteeIndex,
				committeesAtSlot:        committeesAtSlot,
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

	var proposerDutiesContainer *structs.GetProposerDutiesResponse
	wg.Go(func() error {
		proposerDutiesContainer, err = c.dutiesProvider.ProposerDuties(ctx, epoch)
		if err != nil {
			return errors.Wrapf(err, "failed to get proposer duties for epoch `%d`", epoch)
		}

		for _, proposerDuty := range proposerDutiesContainer.Data {
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
		return err
	}

	duties := make([]*ethpb.ValidatorDuty, len(vals))
	for i, v := range vals {
		att, ok := attesterDutiesMapping[v.index]
		if !ok {
			log.Debugf("failed to find attester duty for validator `%d`", v.index)
		}

		duties[i] = &ethpb.ValidatorDuty{
			ValidatorCommitteeIndex: att.validatorCommitteeIndex,
			CommitteeLength:         att.committeeLength,
			CommitteeIndex:          att.committeeIndex,
			AttesterSlot:            att.slot,
			CommitteesAtSlot:        att.committeesAtSlot,
			ProposerSlots:           proposerDutySlots[v.index],
			PublicKey:               v.pubkey,
			Status:                  v.status,
			ValidatorIndex:          v.index,
			IsSyncCommittee:         syncDutiesMapping[v.index],
		}
	}

	dutiesContainer.CurrentEpochDuties = duties
	dutiesContainer.CurrDependentRoot, err = hexutil.Decode(proposerDutiesContainer.DependentRoot)
	if err != nil {
		return errors.Wrap(err, "failed to decode current dependent root")
	}
	dutiesContainer.PrevDependentRoot, err = hexutil.Decode(attesterDutiesContainer.DependentRoot)
	if err != nil {
		return errors.Wrap(err, "failed to decode previous dependent root")
	}
	return nil
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

// Committees retrieves the committees for the given epoch
func (c beaconApiDutiesProvider) Committees(ctx context.Context, epoch primitives.Epoch) ([]*structs.Committee, error) {
	committeeParams := url.Values{}
	committeeParams.Add("epoch", strconv.FormatUint(uint64(epoch), 10))
	committeesRequest := apiutil.BuildURL("/eth/v1/beacon/states/head/committees", committeeParams)

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

// AttesterDuties retrieves the attester duties for the given epoch and validatorIndices
func (c beaconApiDutiesProvider) AttesterDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) (*structs.GetAttesterDutiesResponse, error) {
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

	return attesterDuties, nil
}

// ProposerDuties retrieves the proposer duties for the given epoch
func (c beaconApiDutiesProvider) ProposerDuties(ctx context.Context, epoch primitives.Epoch) (*structs.GetProposerDutiesResponse, error) {
	proposerDuties := &structs.GetProposerDutiesResponse{}
	if err := c.jsonRestHandler.Get(ctx, fmt.Sprintf("/eth/v1/validator/duties/proposer/%d", epoch), proposerDuties); err != nil {
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

	return proposerDuties, nil
}

// SyncDuties retrieves the sync committee duties for the given epoch and validatorIndices
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
