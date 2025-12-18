package beacon_api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	_ = iface.NodeClient(&beaconApiNodeClient{})
)

type beaconApiNodeClient struct {
	fallbackClient  iface.NodeClient
	jsonRestHandler RestHandler
	genesisProvider GenesisProvider
}

func (c *beaconApiNodeClient) SyncStatus(ctx context.Context, _ *empty.Empty) (*ethpb.SyncStatus, error) {
	syncingResponse := structs.SyncStatusResponse{}
	if err := c.jsonRestHandler.Get(ctx, "/eth/v1/node/syncing", &syncingResponse); err != nil {
		return nil, err
	}

	if syncingResponse.Data == nil {
		return nil, errors.New("syncing data is nil")
	}

	return &ethpb.SyncStatus{
		Syncing: syncingResponse.Data.IsSyncing,
	}, nil
}

func (c *beaconApiNodeClient) Genesis(ctx context.Context, _ *empty.Empty) (*ethpb.Genesis, error) {
	genesisJson, err := c.genesisProvider.Genesis(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get genesis")
	}

	genesisValidatorRoot, err := hexutil.Decode(genesisJson.GenesisValidatorsRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode genesis validator root `%s`", genesisJson.GenesisValidatorsRoot)
	}

	genesisTime, err := strconv.ParseInt(genesisJson.GenesisTime, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse genesis time `%s`", genesisJson.GenesisTime)
	}

	depositContractJson := structs.GetDepositContractResponse{}
	if err = c.jsonRestHandler.Get(ctx, "/eth/v1/config/deposit_contract", &depositContractJson); err != nil {
		return nil, err
	}

	if depositContractJson.Data == nil {
		return nil, errors.New("deposit contract data is nil")
	}

	depositContactAddress, err := hexutil.Decode(depositContractJson.Data.Address)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode deposit contract address `%s`", depositContractJson.Data.Address)
	}

	return &ethpb.Genesis{
		GenesisTime: &timestamppb.Timestamp{
			Seconds: genesisTime,
		},
		DepositContractAddress: depositContactAddress,
		GenesisValidatorsRoot:  genesisValidatorRoot,
	}, nil
}

func (c *beaconApiNodeClient) Version(ctx context.Context, _ *empty.Empty) (*ethpb.Version, error) {
	var versionResponse structs.GetVersionResponse
	if err := c.jsonRestHandler.Get(ctx, "/eth/v1/node/version", &versionResponse); err != nil {
		return nil, err
	}

	if versionResponse.Data == nil || versionResponse.Data.Version == "" {
		return nil, errors.New("empty version response")
	}

	return &ethpb.Version{
		Version: versionResponse.Data.Version,
	}, nil
}

func (c *beaconApiNodeClient) Peers(ctx context.Context, in *empty.Empty) (*ethpb.Peers, error) {
	if c.fallbackClient != nil {
		return c.fallbackClient.Peers(ctx, in)
	}

	// TODO: Implement me
	return nil, errors.New("beaconApiNodeClient.Peers is not implemented. To use a fallback client, pass a fallback client as the last argument of NewBeaconApiNodeClientWithFallback.")
}

// IsReady returns true only if the node is fully synced (200 OK).
// A 206 Partial Content response indicates the node is syncing and not ready.
func (c *beaconApiNodeClient) IsReady(ctx context.Context) bool {
	url := c.jsonRestHandler.Host() + "/eth/v1/node/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.WithError(err).Error("failed to create health request")
		return false
	}
	resp, err := c.jsonRestHandler.HttpClient().Do(req)
	if err != nil {
		log.WithError(err).Error("failed to get health of node")
		return false
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Error("failed to close response body")
		}
	}()
	// Only 200 OK means the node is fully synced and ready.
	// 206 Partial Content means syncing, 503 means unavailable.
	return resp.StatusCode == http.StatusOK
}

func NewNodeClientWithFallback(jsonRestHandler RestHandler, fallbackClient iface.NodeClient) iface.NodeClient {
	b := &beaconApiNodeClient{
		jsonRestHandler: jsonRestHandler,
		fallbackClient:  fallbackClient,
		genesisProvider: &beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler},
	}
	return b
}
