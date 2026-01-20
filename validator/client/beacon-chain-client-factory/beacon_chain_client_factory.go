package beacon_chain_client_factory

import (
	"github.com/OffchainLabs/prysm/v7/config/features"
	beaconApi "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api"
	grpcApi "github.com/OffchainLabs/prysm/v7/validator/client/grpc-api"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	nodeClientFactory "github.com/OffchainLabs/prysm/v7/validator/client/node-client-factory"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
)

func NewChainClient(validatorConn validatorHelpers.NodeConnection) iface.ChainClient {
	grpcClient := grpcApi.NewGrpcChainClient(validatorConn)
	if features.Get().EnableBeaconRESTApi {
		return beaconApi.NewBeaconApiChainClientWithFallback(validatorConn.GetRestHandler(), grpcClient)
	}
	return grpcClient
}

func NewPrysmChainClient(validatorConn validatorHelpers.NodeConnection) iface.PrysmChainClient {
	if features.Get().EnableBeaconRESTApi {
		return beaconApi.NewPrysmChainClient(validatorConn.GetRestHandler(), nodeClientFactory.NewNodeClient(validatorConn))
	}
	return grpcApi.NewGrpcPrysmChainClient(validatorConn)
}
