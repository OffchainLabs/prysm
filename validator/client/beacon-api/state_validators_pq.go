package beacon_api

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/validator/pqkeys"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// pqStateValidatorsProvider is the state validators provider of the PQ fork.
//
// In the PQ fork, the validator client identifies its validators by BLS public key, and the beacon node by XMSS public key. This
// provider takes and returns BLS public keys, and translates them to and from XMSS public keys with `keys`. It also
// hands the VRF root of each of its validators in the state to `keys`, which builds their VRF trees.
type pqStateValidatorsProvider struct {
	handler rest.Handler
	keys    *pqkeys.Registry
}

var _ StateValidatorsProvider = pqStateValidatorsProvider{}

func (p pqStateValidatorsProvider) StateValidators(
	ctx context.Context,
	pubkeys []string,
	indices []primitives.ValidatorIndex,
	statuses []string,
) (*structs.GetValidatorsResponse, error) {
	return p.stateValidators(ctx, "/eth/v1/beacon/states/head/validators", pubkeys, indices, statuses)
}

func (p pqStateValidatorsProvider) StateValidatorsForSlot(
	ctx context.Context,
	slot primitives.Slot,
	pubkeys []string,
	indices []primitives.ValidatorIndex,
	statuses []string,
) (*structs.GetValidatorsResponse, error) {
	return p.stateValidators(ctx, fmt.Sprintf("/eth/v1/beacon/states/%d/validators", slot), pubkeys, indices, statuses)
}

func (p pqStateValidatorsProvider) StateValidatorsForHead(
	ctx context.Context,
	pubkeys []string,
	indices []primitives.ValidatorIndex,
	statuses []string,
) (*structs.GetValidatorsResponse, error) {
	return p.stateValidators(ctx, "/eth/v1/beacon/states/head/validators", pubkeys, indices, statuses)
}

// stateValidators fetches from `endpoint` the validators whose hex encoded BLS public key is in `pubkeys` or whose
// index is in `indices`, with a status in `statuses`.
func (p pqStateValidatorsProvider) stateValidators(
	ctx context.Context,
	endpoint string,
	pubkeys []string,
	indices []primitives.ValidatorIndex,
	statuses []string,
) (*structs.GetValidatorsResponse, error) {
	ids := convertValidatorIndicesToStrings(indices)
	for _, pubkey := range pubkeys {
		blsKey, err := bytesutil.DecodeHexWithLength(pubkey, fieldparams.BLSPubkeyLength)
		if err != nil {
			return nil, fmt.Errorf("decode BLS public key %s: %w", pubkey, err)
		}

		xmssKey, ok := p.keys.XMSSPubkey([fieldparams.BLSPubkeyLength]byte(blsKey))
		if !ok {
			// The beacon node knows validators by XMSS public key only. Without its keys, this one cannot be found.
			log.WithField("pubkey", pubkey).Debug("No post-quantum keys for this validator")
			continue
		}

		ids = append(ids, hexutil.Encode(xmssKey[:]))
	}

	// Without any identifier, the beacon node returns every validator. Return none if none of the asked ones is known.
	if len(ids) == 0 && len(pubkeys)+len(indices) > 0 {
		return &structs.GetValidatorsResponse{Data: []*structs.ValidatorContainer{}}, nil
	}

	resp, err := fetchStateValidators(ctx, p.handler, endpoint, ids, statuses, func(resp *structs.GetValidatorsResponsePQ) bool {
		return resp.Data != nil
	})
	if err != nil {
		return nil, err
	}

	validators := make([]*structs.ValidatorContainer, 0, len(resp.Data))
	for _, container := range resp.Data {
		validator, err := p.fromPQ(container)
		if err != nil {
			return nil, err
		}

		validators = append(validators, validator)
	}

	return &structs.GetValidatorsResponse{
		ExecutionOptimistic: resp.ExecutionOptimistic,
		Finalized:           resp.Finalized,
		Data:                validators,
	}, nil
}

// fromPQ converts a PQ validator container to a validator container, with the BLS public key of the validator if
// the validator client knows it. It then hands the VRF root of the validator to the registry.
func (p pqStateValidatorsProvider) fromPQ(container *structs.ValidatorContainerPQ) (*structs.ValidatorContainer, error) {
	if container == nil || container.Validator == nil {
		return nil, fmt.Errorf("nil validator in response")
	}

	validator := container.Validator

	xmssKey, err := bytesutil.DecodeHexWithLength(validator.Pubkey, fieldparams.XMSSPubkeyLength)
	if err != nil {
		return nil, fmt.Errorf("decode XMSS public key %s: %w", validator.Pubkey, err)
	}

	pubkey := validator.Pubkey
	if blsKey, ok := p.keys.BLSPubkey([fieldparams.XMSSPubkeyLength]byte(xmssKey)); ok {
		vrfRoot, err := bytesutil.DecodeHexWithLength(validator.VrfRoot, fieldparams.RootLength)
		if err != nil {
			return nil, fmt.Errorf("decode VRF root %s: %w", validator.VrfRoot, err)
		}

		p.keys.SetVRFRoot(blsKey, [fieldparams.RootLength]byte(vrfRoot))
		pubkey = hexutil.Encode(blsKey[:])
	}

	return &structs.ValidatorContainer{
		Index:   container.Index,
		Balance: container.Balance,
		Status:  container.Status,
		Validator: &structs.Validator{
			Pubkey:                     pubkey,
			WithdrawalCredentials:      validator.WithdrawalCredentials,
			EffectiveBalance:           validator.EffectiveBalance,
			Slashed:                    validator.Slashed,
			ActivationEligibilityEpoch: validator.ActivationEligibilityEpoch,
			ActivationEpoch:            validator.ActivationEpoch,
			ExitEpoch:                  validator.ExitEpoch,
			WithdrawableEpoch:          validator.WithdrawableEpoch,
		},
	}, nil
}
