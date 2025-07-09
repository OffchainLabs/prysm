package beacon_api

import (
	"context"
	"reflect"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
)

type beaconApiChainClient struct {
	fallbackClient          iface.ChainClient
	jsonRestHandler         RestHandler
	stateValidatorsProvider StateValidatorsProvider
}

func (c beaconApiChainClient) headBlockHeaders(ctx context.Context) (*structs.GetBlockHeaderResponse, error) {
	blockHeader := structs.GetBlockHeaderResponse{}
	err := c.jsonRestHandler.Get(ctx, "/eth/v1/beacon/headers/head", &blockHeader)
	if err != nil {
		return nil, err
	}

	if blockHeader.Data == nil || blockHeader.Data.Header == nil {
		return nil, errors.New("block header data is nil")
	}

	if blockHeader.Data.Header.Message == nil {
		return nil, errors.New("block header message is nil")
	}

	return &blockHeader, nil
}

func (c beaconApiChainClient) ChainHead(ctx context.Context, _ *empty.Empty) (*ethpb.ChainHead, error) {
	const endpoint = "/eth/v1/beacon/states/head/finality_checkpoints"

	finalityCheckpoints := structs.GetFinalityCheckpointsResponse{}
	if err := c.jsonRestHandler.Get(ctx, endpoint, &finalityCheckpoints); err != nil {
		return nil, err
	}

	if finalityCheckpoints.Data == nil {
		return nil, errors.New("finality checkpoints data is nil")
	}

	if finalityCheckpoints.Data.Finalized == nil {
		return nil, errors.New("finalized checkpoint is nil")
	}

	finalizedEpoch, err := strconv.ParseUint(finalityCheckpoints.Data.Finalized.Epoch, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse finalized epoch `%s`", finalityCheckpoints.Data.Finalized.Epoch)
	}

	finalizedSlot, err := slots.EpochStart(primitives.Epoch(finalizedEpoch))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get first slot for epoch `%d`", finalizedEpoch)
	}

	finalizedRoot, err := hexutil.Decode(finalityCheckpoints.Data.Finalized.Root)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode finalized checkpoint root `%s`", finalityCheckpoints.Data.Finalized.Root)
	}

	if finalityCheckpoints.Data.CurrentJustified == nil {
		return nil, errors.New("current justified checkpoint is nil")
	}

	justifiedEpoch, err := strconv.ParseUint(finalityCheckpoints.Data.CurrentJustified.Epoch, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse current justified checkpoint epoch `%s`", finalityCheckpoints.Data.CurrentJustified.Epoch)
	}

	justifiedSlot, err := slots.EpochStart(primitives.Epoch(justifiedEpoch))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get first slot for epoch `%d`", justifiedEpoch)
	}

	justifiedRoot, err := hexutil.Decode(finalityCheckpoints.Data.CurrentJustified.Root)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode current justified checkpoint root `%s`", finalityCheckpoints.Data.CurrentJustified.Root)
	}

	if finalityCheckpoints.Data.PreviousJustified == nil {
		return nil, errors.New("previous justified checkpoint is nil")
	}

	previousJustifiedEpoch, err := strconv.ParseUint(finalityCheckpoints.Data.PreviousJustified.Epoch, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse previous justified checkpoint epoch `%s`", finalityCheckpoints.Data.PreviousJustified.Epoch)
	}

	previousJustifiedSlot, err := slots.EpochStart(primitives.Epoch(previousJustifiedEpoch))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get first slot for epoch `%d`", previousJustifiedEpoch)
	}

	previousJustifiedRoot, err := hexutil.Decode(finalityCheckpoints.Data.PreviousJustified.Root)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode previous justified checkpoint root `%s`", finalityCheckpoints.Data.PreviousJustified.Root)
	}

	blockHeader, err := c.headBlockHeaders(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get head block headers")
	}

	headSlot, err := strconv.ParseUint(blockHeader.Data.Header.Message.Slot, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse head block slot `%s`", blockHeader.Data.Header.Message.Slot)
	}

	headEpoch := slots.ToEpoch(primitives.Slot(headSlot))

	headBlockRoot, err := hexutil.Decode(blockHeader.Data.Root)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode head block root `%s`", blockHeader.Data.Root)
	}

	return &ethpb.ChainHead{
		HeadSlot:                   primitives.Slot(headSlot),
		HeadEpoch:                  headEpoch,
		HeadBlockRoot:              headBlockRoot,
		FinalizedSlot:              finalizedSlot,
		FinalizedEpoch:             primitives.Epoch(finalizedEpoch),
		FinalizedBlockRoot:         finalizedRoot,
		JustifiedSlot:              justifiedSlot,
		JustifiedEpoch:             primitives.Epoch(justifiedEpoch),
		JustifiedBlockRoot:         justifiedRoot,
		PreviousJustifiedSlot:      previousJustifiedSlot,
		PreviousJustifiedEpoch:     primitives.Epoch(previousJustifiedEpoch),
		PreviousJustifiedBlockRoot: previousJustifiedRoot,
		OptimisticStatus:           blockHeader.ExecutionOptimistic,
	}, nil
}

func (c beaconApiChainClient) ValidatorBalances(ctx context.Context, in *ethpb.ListValidatorBalancesRequest) (*ethpb.ValidatorBalances, error) {
	if c.fallbackClient != nil {
		return c.fallbackClient.ValidatorBalances(ctx, in)
	}

	// TODO: Implement me
	return nil, errors.New("beaconApiChainClient.ValidatorBalances is not implemented. To use a fallback client, pass a fallback client as the last argument of NewBeaconApiChainClientWithFallback.")
}

func (c beaconApiChainClient) Validators(ctx context.Context, in *ethpb.ListValidatorsRequest) (*ethpb.Validators, error) {
	pageSize := in.PageSize

	// We follow the gRPC behavior here, which returns a maximum of 250 results when pageSize == 0
	if pageSize == 0 {
		pageSize = 250
	}

	var pageToken uint64
	var err error

	if in.PageToken != "" {
		if pageToken, err = strconv.ParseUint(in.PageToken, 10, 64); err != nil {
			return nil, errors.Wrapf(err, "failed to parse page token `%s`", in.PageToken)
		}
	}

	var statuses []string
	if in.Active {
		statuses = []string{"active"}
	}

	pubkeys := make([]string, len(in.PublicKeys))
	for idx, pubkey := range in.PublicKeys {
		pubkeys[idx] = hexutil.Encode(pubkey)
	}

	var stateValidators *structs.GetValidatorsResponse
	var epoch primitives.Epoch

	switch queryFilter := in.QueryFilter.(type) {
	case *ethpb.ListValidatorsRequest_Epoch:
		slot, err := slots.EpochStart(queryFilter.Epoch)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get first slot for epoch `%d`", queryFilter.Epoch)
		}
		if stateValidators, err = c.stateValidatorsProvider.StateValidatorsForSlot(ctx, slot, pubkeys, in.Indices, statuses); err != nil {
			return nil, errors.Wrapf(err, "failed to get state validators for slot `%d`", slot)
		}
		epoch = slots.ToEpoch(slot)
	case *ethpb.ListValidatorsRequest_Genesis:
		if stateValidators, err = c.stateValidatorsProvider.StateValidatorsForSlot(ctx, 0, pubkeys, in.Indices, statuses); err != nil {
			return nil, errors.Wrapf(err, "failed to get genesis state validators")
		}
		epoch = 0
	case nil:
		if stateValidators, err = c.stateValidatorsProvider.StateValidatorsForHead(ctx, pubkeys, in.Indices, statuses); err != nil {
			return nil, errors.Wrap(err, "failed to get head state validators")
		}

		blockHeader, err := c.headBlockHeaders(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get head block headers")
		}

		slot, err := strconv.ParseUint(blockHeader.Data.Header.Message.Slot, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse header slot `%s`", blockHeader.Data.Header.Message.Slot)
		}

		epoch = slots.ToEpoch(primitives.Slot(slot))
	default:
		return nil, errors.Errorf("unsupported query filter type `%v`", reflect.TypeOf(queryFilter))
	}

	if stateValidators.Data == nil {
		return nil, errors.New("state validators data is nil")
	}

	start := pageToken * uint64(pageSize)
	if start > uint64(len(stateValidators.Data)) {
		start = uint64(len(stateValidators.Data))
	}

	end := start + uint64(pageSize)
	if end > uint64(len(stateValidators.Data)) {
		end = uint64(len(stateValidators.Data))
	}

	validators := make([]*ethpb.Validators_ValidatorContainer, end-start)
	for idx := start; idx < end; idx++ {
		stateValidator := stateValidators.Data[idx]

		if stateValidator.Validator == nil {
			return nil, errors.Errorf("state validator at index `%d` is nil", idx)
		}

		pubkey, err := hexutil.Decode(stateValidator.Validator.Pubkey)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode validator pubkey `%s`", stateValidator.Validator.Pubkey)
		}

		withdrawalCredentials, err := hexutil.Decode(stateValidator.Validator.WithdrawalCredentials)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode validator withdrawal credentials `%s`", stateValidator.Validator.WithdrawalCredentials)
		}

		effectiveBalance, err := strconv.ParseUint(stateValidator.Validator.EffectiveBalance, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator effective balance `%s`", stateValidator.Validator.EffectiveBalance)
		}

		validatorIndex, err := strconv.ParseUint(stateValidator.Index, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator index `%s`", stateValidator.Index)
		}

		activationEligibilityEpoch, err := strconv.ParseUint(stateValidator.Validator.ActivationEligibilityEpoch, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator activation eligibility epoch `%s`", stateValidator.Validator.ActivationEligibilityEpoch)
		}

		activationEpoch, err := strconv.ParseUint(stateValidator.Validator.ActivationEpoch, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator activation epoch `%s`", stateValidator.Validator.ActivationEpoch)
		}

		exitEpoch, err := strconv.ParseUint(stateValidator.Validator.ExitEpoch, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator exit epoch `%s`", stateValidator.Validator.ExitEpoch)
		}

		withdrawableEpoch, err := strconv.ParseUint(stateValidator.Validator.WithdrawableEpoch, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator withdrawable epoch `%s`", stateValidator.Validator.WithdrawableEpoch)
		}

		validators[idx-start] = &ethpb.Validators_ValidatorContainer{
			Index: primitives.ValidatorIndex(validatorIndex),
			Validator: &ethpb.Validator{
				PublicKey:                  pubkey,
				WithdrawalCredentials:      withdrawalCredentials,
				EffectiveBalance:           effectiveBalance,
				Slashed:                    stateValidator.Validator.Slashed,
				ActivationEligibilityEpoch: primitives.Epoch(activationEligibilityEpoch),
				ActivationEpoch:            primitives.Epoch(activationEpoch),
				ExitEpoch:                  primitives.Epoch(exitEpoch),
				WithdrawableEpoch:          primitives.Epoch(withdrawableEpoch),
			},
		}
	}

	var nextPageToken string
	if end < uint64(len(stateValidators.Data)) {
		nextPageToken = strconv.FormatUint(pageToken+1, 10)
	}

	return &ethpb.Validators{
		TotalSize:     int32(len(stateValidators.Data)),
		Epoch:         epoch,
		ValidatorList: validators,
		NextPageToken: nextPageToken,
	}, nil
}

func (c beaconApiChainClient) ValidatorQueue(ctx context.Context, in *empty.Empty) (*ethpb.ValidatorQueue, error) {
	if c.fallbackClient != nil {
		return c.fallbackClient.ValidatorQueue(ctx, in)
	}

	// TODO: Implement me
	return nil, errors.New("beaconApiChainClient.ValidatorQueue is not implemented. To use a fallback client, pass a fallback client as the last argument of NewBeaconApiChainClientWithFallback.")
}

func (c beaconApiChainClient) ValidatorPerformance(ctx context.Context, in *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error) {
	if c.fallbackClient != nil {
		return c.fallbackClient.ValidatorPerformance(ctx, in)
	}

	// TODO: Implement me
	return nil, errors.New("beaconApiChainClient.ValidatorPerformance is not implemented. To use a fallback client, pass a fallback client as the last argument of NewBeaconApiChainClientWithFallback.")
}

func (c beaconApiChainClient) ValidatorParticipation(ctx context.Context, in *ethpb.GetValidatorParticipationRequest) (*ethpb.ValidatorParticipationResponse, error) {
	if c.fallbackClient != nil {
		return c.fallbackClient.ValidatorParticipation(ctx, in)
	}

	// TODO: Implement me
	return nil, errors.New("beaconApiChainClient.ValidatorParticipation is not implemented. To use a fallback client, pass a fallback client as the last argument of NewBeaconApiChainClientWithFallback.")
}

func NewBeaconApiChainClientWithFallback(jsonRestHandler RestHandler, fallbackClient iface.ChainClient) iface.ChainClient {
	return &beaconApiChainClient{
		jsonRestHandler:         jsonRestHandler,
		fallbackClient:          fallbackClient,
		stateValidatorsProvider: beaconApiStateValidatorsProvider{jsonRestHandler: jsonRestHandler},
	}
}
