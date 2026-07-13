package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const executionPayloadBidEndpoint = "/eth/v1/beacon/execution_payload_bids"

func (c *beaconApiValidatorClient) submitSignedExecutionPayloadBid(ctx context.Context, bid *ethpb.SignedExecutionPayloadBid) error {
	if bid == nil || bid.Message == nil {
		return errors.New("signed execution payload bid is nil")
	}

	headers := map[string]string{api.VersionHeader: version.String(version.Gloas)}

	// Prefer SSZ; fall back to JSON if the beacon node does not accept octet-stream request bodies.
	sszBody, err := bid.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "failed to marshal signed execution payload bid ssz")
	}
	if _, _, err = c.handler.PostSSZ(ctx, executionPayloadBidEndpoint, headers, bytes.NewBuffer(sszBody)); err == nil {
		return nil
	}
	errJson := &httputil.DefaultJsonError{}
	if !errors.As(err, &errJson) || errJson.Code != http.StatusUnsupportedMediaType {
		return err
	}
	log.WithError(err).Warn("Beacon node does not accept SSZ execution payload bid, falling back to JSON")

	jsonBody, err := json.Marshal(structs.SignedExecutionPayloadBidFromConsensus(bid))
	if err != nil {
		return errors.Wrap(err, "failed to marshal signed execution payload bid")
	}
	return c.handler.Post(ctx, executionPayloadBidEndpoint, headers, bytes.NewBuffer(jsonBody), nil)
}
