package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
)

type beaconApiChainClient struct {
	handler rest.Handler
}

func (c beaconApiChainClient) headBlockHeaders(ctx context.Context) (*structs.GetBlockHeaderResponse, error) {
	blockHeader := structs.GetBlockHeaderResponse{}
	err := c.handler.Get(ctx, "/eth/v1/beacon/headers/head", &blockHeader)
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
	if err := c.handler.Get(ctx, endpoint, &finalityCheckpoints); err != nil {
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

func (c beaconApiChainClient) ValidatorPerformance(ctx context.Context, in *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error) {
	// Note: ValidatorPerformance is only supported on Prysm nodes,
	// So we should check whether the node is Prysm by node version.
	var versionResponse structs.GetVersionResponse
	if err := c.handler.Get(ctx, "/eth/v1/node/version", &versionResponse); err != nil {
		return nil, errors.Wrap(err, "failed to get node version")
	}

	if versionResponse.Data == nil || versionResponse.Data.Version == "" {
		return nil, errors.New("empty version response")
	}

	if !strings.Contains(strings.ToLower(versionResponse.Data.Version), "prysm") {
		return nil, iface.ErrNotSupported
	}

	// Now confirmed that the node is Prysm, call Prysm-specific performace endpoint.
	request, err := json.Marshal(structs.GetValidatorPerformanceRequest{
		PublicKeys: in.PublicKeys,
		Indices:    in.Indices,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal request")
	}
	resp := &structs.GetValidatorPerformanceResponse{}
	if err = c.handler.Post(ctx, "/prysm/validators/performance", nil, bytes.NewBuffer(request), resp); err != nil {
		return nil, err
	}

	return &ethpb.ValidatorPerformanceResponse{
		CurrentEffectiveBalances:      resp.CurrentEffectiveBalances,
		CorrectlyVotedSource:          resp.CorrectlyVotedSource,
		CorrectlyVotedTarget:          resp.CorrectlyVotedTarget,
		CorrectlyVotedHead:            resp.CorrectlyVotedHead,
		BalancesBeforeEpochTransition: resp.BalancesBeforeEpochTransition,
		BalancesAfterEpochTransition:  resp.BalancesAfterEpochTransition,
		MissingValidators:             resp.MissingValidators,
		PublicKeys:                    resp.PublicKeys,
		InactivityScores:              resp.InactivityScores,
	}, nil
}

func NewChainClient(provider rest.RestConnectionProvider) iface.ChainClient {
	handler := provider.Handler()
	return &beaconApiChainClient{
		handler: handler,
	}
}
