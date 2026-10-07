package client

import (
	"github.com/OffchainLabs/prysm/v7/config/features"
	beaconApi "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api"
	grpcApi "github.com/OffchainLabs/prysm/v7/validator/client/grpc-api"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
)

func NewNodeClient(validatorConn *validatorHelpers.NodeConnection) iface.NodeClient {
	if features.Get().EnableBeaconRESTApi && validatorConn.GetRestConnectionProvider() != nil {
		return beaconApi.NewNodeClient(validatorConn.GetRestConnectionProvider())
	}
	return grpcApi.NewNodeClient(validatorConn)
}

func NewChainClient(validatorConn *validatorHelpers.NodeConnection) iface.ChainClient {
	grpcClient := grpcApi.NewGrpcChainClient(validatorConn)
	if features.Get().EnableBeaconRESTApi && validatorConn.GetRestConnectionProvider() != nil {
		return beaconApi.NewBeaconApiChainClientWithFallback(validatorConn.GetRestConnectionProvider(), grpcClient)
	}
	return grpcClient
}

func NewValidatorClient(validatorConn *validatorHelpers.NodeConnection, opts ...iface.Option) iface.ValidatorClient {
	if features.Get().EnableBeaconRESTApi && validatorConn.GetRestConnectionProvider() != nil {
		return beaconApi.NewBeaconApiValidatorClient(validatorConn.GetRestConnectionProvider(), opts...)
	}
	return grpcApi.NewGrpcValidatorClient(validatorConn, opts...)
}
