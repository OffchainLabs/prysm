package prysm_api

import (
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/chain"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/node"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
	"google.golang.org/grpc"
)

func NewClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler client.JsonRestHandler) Client {
	if features.Get().EnableBeaconRESTApi {
		return NewPrysmChainRestClient(jsonRestHandler, node.NewClient(validatorConn, jsonRestHandler))
	} else {
		return NewGrpcPrysmChainClient(validatorConn.GetGrpcClientConn())
	}
}

// NewPrysmChainClient returns implementation of Client.
func NewPrysmChainRestClient(jsonRestHandler client.JsonRestHandler, nodeClient node.Client) Client {
	return prysmChainClient{
		jsonRestHandler: jsonRestHandler,
		nodeClient:      nodeClient,
	}
}

func NewGrpcPrysmChainClient(cc grpc.ClientConnInterface) Client {
	return &grpcPrysmChainClient{chainClient: chain.NewGrpcChainClient(cc)}
}
