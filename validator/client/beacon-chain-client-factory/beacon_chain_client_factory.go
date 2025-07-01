package beacon_chain_client_factory

import (
	"github.com/OffchainLabs/prysm/v6/config/features"
	beaconApi "github.com/OffchainLabs/prysm/v6/validator/client/beacon-api"
	grpcApi "github.com/OffchainLabs/prysm/v6/validator/client/grpc-api"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	nodeClientFactory "github.com/OffchainLabs/prysm/v6/validator/client/node-client-factory"
	validatorHelpers "github.com/OffchainLabs/prysm/v6/validator/helpers"
)

func NewChainClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler beaconApi.RestHandler) iface.ChainClient {
	grpcClient := grpcApi.NewGrpcChainClient(validatorConn.GetGrpcClientConn())
	if features.Get().EnableBeaconRESTApi {
		return beaconApi.NewBeaconApiChainClientWithFallback(jsonRestHandler, grpcClient)
	} else {
		return grpcClient
	}
}

func NewPrysmChainClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler beaconApi.RestHandler) iface.PrysmChainClient {
	if features.Get().EnableBeaconRESTApi {
		return beaconApi.NewPrysmChainClient(jsonRestHandler, nodeClientFactory.NewNodeClient(validatorConn, jsonRestHandler))
	} else {
		return grpcApi.NewGrpcPrysmChainClient(validatorConn.GetGrpcClientConn())
	}
}
