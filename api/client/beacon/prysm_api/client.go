package prysm_api

import (
	"github.com/prysmaticlabs/prysm/v5/config/features"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	beaconApi "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api"
	nodeClientFactory "github.com/prysmaticlabs/prysm/v5/validator/client/node-client-factory"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
	"google.golang.org/grpc"
)

func NewPrysmChainClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler beaconApi.JsonRestHandler) PrysmChainClient {
	if features.Get().EnableBeaconRESTApi {
		return beaconApi.NewPrysmChainClient(jsonRestHandler, nodeClientFactory.NewNodeClient(validatorConn, jsonRestHandler))
	} else {
		return NewGrpcPrysmChainClient(validatorConn.GetGrpcClientConn())
	}
}

func NewGrpcPrysmChainClient(cc grpc.ClientConnInterface) PrysmChainClient {
	return &grpcPrysmChainClient{chainClient: &grpcChainClient{ethpb.NewBeaconChainClient(cc)}}
}
