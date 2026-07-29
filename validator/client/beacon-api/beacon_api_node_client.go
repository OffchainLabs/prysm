package beacon_api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/rest"
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
	handler         rest.Handler
	genesisProvider GenesisProvider
}

func (c *beaconApiNodeClient) SyncStatus(ctx context.Context, _ *empty.Empty) (*ethpb.SyncStatus, error) {
	syncingResponse := structs.SyncStatusResponse{}
	if err := c.handler.Get(ctx, "/eth/v1/node/syncing", &syncingResponse); err != nil {
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
	if err = c.handler.Get(ctx, "/eth/v1/config/deposit_contract", &depositContractJson); err != nil {
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
	if err := c.handler.Get(ctx, "/eth/v1/node/version", &versionResponse); err != nil {
		return nil, err
	}

	if versionResponse.Data == nil || versionResponse.Data.Version == "" {
		return nil, errors.New("empty version response")
	}

	return &ethpb.Version{
		Version: versionResponse.Data.Version,
	}, nil
}

func (c *beaconApiNodeClient) Peers(ctx context.Context, _ *empty.Empty) (*ethpb.Peers, error) {
	var peersResponse structs.GetPeersResponse
	if err := c.handler.Get(ctx, "/eth/v1/node/peers", &peersResponse); err != nil {
		return nil, err
	}

	peers := make([]*ethpb.Peer, len(peersResponse.Data))
	for i, p := range peersResponse.Data {
		if p == nil {
			return nil, fmt.Errorf("peer at index %d is nil", i)
		}

		// The beacon API spells the enums in lower case, the proto enums in upper case.
		direction, ok := ethpb.PeerDirection_value[strings.ToUpper(p.Direction)]
		if !ok {
			return nil, fmt.Errorf("unknown peer direction `%s`", p.Direction)
		}
		state, ok := ethpb.ConnectionState_value[strings.ToUpper(p.State)]
		if !ok {
			return nil, fmt.Errorf("unknown connection state `%s`", p.State)
		}

		peers[i] = &ethpb.Peer{
			Address:         p.LastSeenP2PAddress,
			Direction:       ethpb.PeerDirection(direction),
			ConnectionState: ethpb.ConnectionState(state),
			PeerId:          p.PeerId,
			Enr:             p.Enr,
		}
	}

	return &ethpb.Peers{Peers: peers}, nil
}

// IsReady returns true only if the node is fully synced (200 OK).
// A 206 Partial Content response indicates the node is syncing and not ready.
func (c *beaconApiNodeClient) IsReady(ctx context.Context) bool {
	statusCode, err := c.handler.GetStatusCode(ctx, "/eth/v1/node/health")
	if err != nil {
		log.WithError(err).WithField("url", c.handler.Host()).Error("failed to get health of node")
		return false
	}
	// Only 200 OK means the node is fully synced and ready.
	// 206 Partial Content means syncing, 503 means unavailable.
	return statusCode == http.StatusOK
}

func NewNodeClient(provider rest.RestConnectionProvider) iface.NodeClient {
	handler := provider.Handler()
	return &beaconApiNodeClient{
		handler:         handler,
		genesisProvider: &beaconApiGenesisProvider{handler: handler},
	}
}
