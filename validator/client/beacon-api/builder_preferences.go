package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) submitBuilderPreferences(ctx context.Context, in *ethpb.SubmitBuilderPreferencesRequest) error {
	if in == nil || in.Request == nil {
		return errors.New("builder preferences request is nil")
	}
	endpoint := fmt.Sprintf("/eth/v1/validator/builder_preferences/%s", hexutil.Encode(in.ValidatorPubkey))
	headers := map[string]string{api.VersionHeader: version.String(version.Gloas)}

	// Prefer SSZ; fall back to JSON if the beacon node does not accept octet-stream request bodies.
	sszBody, err := in.Request.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "failed to marshal builder preferences request ssz")
	}
	if _, _, err = c.handler.PostSSZ(ctx, endpoint, headers, bytes.NewBuffer(sszBody)); err == nil {
		return nil
	}
	errJson := &httputil.DefaultJsonError{}
	if !errors.As(err, &errJson) || errJson.Code != http.StatusUnsupportedMediaType {
		return err
	}
	log.WithError(err).Warn("Beacon node does not accept SSZ builder preferences, falling back to JSON")

	jsonBody, err := json.Marshal(structs.BuilderPreferencesRequestV1FromConsensus(in.Request))
	if err != nil {
		return errors.Wrap(err, "failed to marshal builder preferences request")
	}
	return c.handler.Post(ctx, endpoint, headers, bytes.NewBuffer(jsonBody), nil)
}
