package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/apiutil"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) submitSyncMessage(ctx context.Context, syncMessage *ethpb.SyncCommitteeMessage) error {
	const endpoint = "/eth/v1/beacon/pool/sync_committees"

	jsonSyncCommitteeMessage := &structs.SyncCommitteeMessage{
		Slot:            strconv.FormatUint(uint64(syncMessage.Slot), 10),
		BeaconBlockRoot: hexutil.Encode(syncMessage.BlockRoot),
		ValidatorIndex:  strconv.FormatUint(uint64(syncMessage.ValidatorIndex), 10),
		Signature:       hexutil.Encode(syncMessage.Signature),
	}

	marshalledJsonSyncCommitteeMessage, err := json.Marshal([]*structs.SyncCommitteeMessage{jsonSyncCommitteeMessage})
	if err != nil {
		return errors.Wrap(err, "failed to marshal sync committee message")
	}

	// The validator already signed this root, so the contribution must use it even if the broadcast fails.
	c.syncMessageRoots.set(syncMessage.Slot, uint64(syncMessage.ValidatorIndex), syncMessage.BlockRoot)
	return c.handler.Post(ctx, endpoint, nil, bytes.NewBuffer(marshalledJsonSyncCommitteeMessage), nil)
}

func (c *beaconApiValidatorClient) syncMessageBlockRoot(ctx context.Context) (*ethpb.SyncMessageBlockRootResponse, error) {
	// Get head beacon block root.
	var resp structs.BlockRootResponse
	opts := syncCommitteeFreshnessOptions(ctx)
	if err := c.handler.Get(ctx, "/eth/v1/beacon/blocks/head/root", &resp, opts...); err != nil {
		return nil, err
	}

	// An optimistic validator MUST NOT participate in sync committees
	// (i.e., sign across the DOMAIN_SYNC_COMMITTEE, DOMAIN_SYNC_COMMITTEE_SELECTION_PROOF or DOMAIN_CONTRIBUTION_AND_PROOF domains).
	if resp.ExecutionOptimistic {
		return nil, errors.New("the node is currently optimistic and cannot serve validators")
	}

	if resp.Data == nil {
		return nil, errors.New("no data returned")
	}

	if resp.Data.Root == "" {
		return nil, errors.New("no root returned")
	}

	blockRoot, err := hexutil.Decode(resp.Data.Root)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode beacon block root")
	}

	return &ethpb.SyncMessageBlockRootResponse{
		Root: blockRoot,
	}, nil
}

func (c *beaconApiValidatorClient) syncCommitteeContribution(
	ctx context.Context,
	req *ethpb.SyncCommitteeContributionRequest,
) (*ethpb.SyncCommitteeContribution, error) {
	// The head may have moved since the aggregator signed its sync message, so aggregate over the signed root.
	root, err := c.signedSyncMessageRoot(ctx, req.Slot, req.PublicKey)
	if err != nil {
		return nil, err
	}
	if root == nil {
		blockRootResponse, err := c.syncMessageBlockRoot(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get sync message block root")
		}
		root = blockRootResponse.Root
	}
	blockRoot := hexutil.Encode(root)

	params := url.Values{}
	params.Add("slot", strconv.FormatUint(uint64(req.Slot), 10))
	params.Add("subcommittee_index", strconv.FormatUint(req.SubnetId, 10))
	params.Add("beacon_block_root", blockRoot)

	var resp structs.ProduceSyncCommitteeContributionResponse
	if err := c.handler.Get(ctx, apiutil.BuildURL("/eth/v1/validator/sync_committee_contribution", params), &resp); err != nil {
		return nil, err
	}

	return convertSyncContributionJsonToProto(resp.Data)
}

// signedSyncMessageRoot returns the block root of the sync message that the validator submitted at the slot, or nil.
func (c *beaconApiValidatorClient) signedSyncMessageRoot(ctx context.Context, slot primitives.Slot, pubkey []byte) ([]byte, error) {
	sharedRoot, found := c.syncMessageRoots.slotRoot(slot)
	if !found {
		return nil, nil
	}
	// The index POST is answered by any host, so its execution_optimistic flag cannot gate the contribution.
	isOptimistic, err := c.isOptimistic(ctx)
	if err != nil {
		return nil, err
	}
	// An optimistic validator MUST NOT sign across the DOMAIN_CONTRIBUTION_AND_PROOF domain.
	if isOptimistic {
		return nil, errors.New("the node is currently optimistic and cannot serve validators")
	}
	if sharedRoot != nil {
		return sharedRoot, nil
	}
	resp, err := c.stateValidatorsProvider.StateValidators(ctx, []string{hexutil.Encode(pubkey)}, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get state validator")
	}
	if len(resp.Data) == 0 || resp.Data[0] == nil {
		return nil, nil
	}
	index, err := strconv.ParseUint(resp.Data[0].Index, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse validator index %s", resp.Data[0].Index)
	}
	root, _ := c.syncMessageRoots.get(slot, index)
	return root, nil
}

func (c *beaconApiValidatorClient) syncSubcommitteeIndex(ctx context.Context, in *ethpb.SyncSubcommitteeIndexRequest) (*ethpb.SyncSubcommitteeIndexResponse, error) {
	validatorIndexResponse, err := c.validatorIndex(ctx, &ethpb.ValidatorIndexRequest{PublicKey: in.PublicKey})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator index")
	}

	// A duty for slot is sampled from the committee of slot+1, so the last slot of a period uses the next committee.
	syncDuties, err := c.dutiesProvider.SyncDuties(ctx, slots.ToEpoch(in.Slot+1), []primitives.ValidatorIndex{validatorIndexResponse.Index})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sync committee duties")
	}

	if len(syncDuties) == 0 {
		return nil, errors.Errorf("no sync committee duty for the given slot %d", in.Slot)
	}

	// First sync duty is required since we requested sync duties for one validator index.
	syncDuty := syncDuties[0]

	var indices []primitives.CommitteeIndex
	for _, idx := range syncDuty.ValidatorSyncCommitteeIndices {
		syncCommIdx, err := strconv.ParseUint(idx, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator sync committee index %s", idx)
		}

		indices = append(indices, primitives.CommitteeIndex(syncCommIdx))
	}

	return &ethpb.SyncSubcommitteeIndexResponse{Indices: indices}, nil
}

func convertSyncContributionJsonToProto(contribution *structs.SyncCommitteeContribution) (*ethpb.SyncCommitteeContribution, error) {
	if contribution == nil {
		return nil, errors.New("sync committee contribution is nil")
	}

	slot, err := strconv.ParseUint(contribution.Slot, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse slot `%s`", contribution.Slot)
	}

	blockRoot, err := hexutil.Decode(contribution.BeaconBlockRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode beacon block root `%s`", contribution.BeaconBlockRoot)
	}

	subcommitteeIdx, err := strconv.ParseUint(contribution.SubcommitteeIndex, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse subcommittee index `%s`", contribution.SubcommitteeIndex)
	}

	aggregationBits, err := hexutil.Decode(contribution.AggregationBits)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode aggregation bits `%s`", contribution.AggregationBits)
	}

	signature, err := hexutil.Decode(contribution.Signature)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode contribution signature `%s`", contribution.Signature)
	}

	return &ethpb.SyncCommitteeContribution{
		Slot:              primitives.Slot(slot),
		BlockRoot:         blockRoot,
		SubcommitteeIndex: subcommitteeIdx,
		AggregationBits:   aggregationBits,
		Signature:         signature,
	}, nil
}
