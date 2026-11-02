package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

var beaconAPITogRPCValidatorStatus = map[string]ethpb.ValidatorStatus{
	"pending_initialized": ethpb.ValidatorStatus_DEPOSITED,
	"pending_queued":      ethpb.ValidatorStatus_PENDING,
	"active_ongoing":      ethpb.ValidatorStatus_ACTIVE,
	"active_exiting":      ethpb.ValidatorStatus_EXITING,
	"active_slashed":      ethpb.ValidatorStatus_SLASHING,
	"exited_unslashed":    ethpb.ValidatorStatus_EXITED,
	"exited_slashed":      ethpb.ValidatorStatus_EXITED,
	"withdrawal_possible": ethpb.ValidatorStatus_EXITED,
	"withdrawal_done":     ethpb.ValidatorStatus_EXITED,
}

func (c *beaconApiValidatorClient) fork(ctx context.Context) (*structs.GetStateForkResponse, error) {
	const endpoint = "/eth/v1/beacon/states/head/fork"

	stateForkResponseJson := &structs.GetStateForkResponse{}

	if err := c.handler.Get(ctx, endpoint, stateForkResponseJson); err != nil {
		return nil, err
	}

	return stateForkResponseJson, nil
}

func (c *beaconApiValidatorClient) headers(ctx context.Context) (*structs.GetBlockHeadersResponse, error) {
	const endpoint = "/eth/v1/beacon/headers"

	blockHeadersResponseJson := &structs.GetBlockHeadersResponse{}

	if err := c.handler.Get(ctx, endpoint, blockHeadersResponseJson); err != nil {
		return nil, err
	}

	return blockHeadersResponseJson, nil
}

func (c *beaconApiValidatorClient) liveness(ctx context.Context, epoch primitives.Epoch, validatorIndexes []string) (*structs.GetLivenessResponse, error) {
	const endpoint = "/eth/v1/validator/liveness/"
	url := endpoint + strconv.FormatUint(uint64(epoch), 10)

	livenessResponseJson := &structs.GetLivenessResponse{}

	marshalledJsonValidatorIndexes, err := json.Marshal(validatorIndexes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal validator indexes")
	}

	if err = c.handler.Post(ctx, url, nil, bytes.NewBuffer(marshalledJsonValidatorIndexes), livenessResponseJson); err != nil {
		return nil, err
	}

	return livenessResponseJson, nil
}

// isSyncedResponse reports whether a raw `/eth/v1/node/syncing` response says the node is synced.
func isSyncedResponse(raw json.RawMessage) bool {
	var resp structs.SyncStatusResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return false
	}

	return resp.Data != nil && !resp.Data.IsSyncing
}

// isSyncedAndNotOptimisticResponse reports whether a raw `/eth/v1/node/syncing` response says
// the node is synced and its head is not optimistic.
func isSyncedAndNotOptimisticResponse(raw json.RawMessage) bool {
	var resp structs.SyncStatusResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return false
	}

	return resp.Data != nil && !resp.Data.IsSyncing && !resp.Data.IsOptimistic
}

func (c *beaconApiValidatorClient) syncing(ctx context.Context, opts ...rest.QueryOption) (*structs.SyncStatusResponse, error) {
	const endpoint = "/eth/v1/node/syncing"

	syncingResponseJson := &structs.SyncStatusResponse{}

	if err := c.handler.Get(ctx, endpoint, syncingResponseJson, opts...); err != nil {
		return nil, err
	}

	return syncingResponseJson, nil
}

// syncedNodeStatus returns the sync status of a synced beacon node if any configured
// beacon node is synced, or else the sync status of one of the syncing beacon nodes.
// It queries every node concurrently and prefers a synced one.
func (c *beaconApiValidatorClient) syncedNodeStatus(ctx context.Context) (*structs.SyncStatusResponseData, error) {
	response, err := c.syncing(ctx, rest.WithRace(), rest.WithAccept(isSyncedResponse))
	if err != nil {
		return nil, fmt.Errorf("failed to get syncing status: %w", err)
	}

	if response == nil || response.Data == nil {
		return nil, errors.New("syncing data is nil")
	}

	return response.Data, nil
}

// isOptimistic reports false if any configured beacon node is synced with a non-optimistic head.
// It queries every node concurrently and prefers such a node.
func (c *beaconApiValidatorClient) isOptimistic(ctx context.Context) (bool, error) {
	response, err := c.syncing(ctx, rest.WithRace(), rest.WithAccept(isSyncedAndNotOptimisticResponse))
	if err != nil || response == nil || response.Data == nil {
		return true, errors.Wrapf(err, "failed to get syncing status")
	}

	// A syncing node cannot serve validators either: treat it as optimistic.
	return response.Data.IsSyncing || response.Data.IsOptimistic, nil
}
