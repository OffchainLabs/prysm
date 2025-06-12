package validator_client_factory

import (
	"github.com/OffchainLabs/prysm/v6/config/features"
	beaconApi "github.com/OffchainLabs/prysm/v6/validator/client/beacon-api"
	grpcApi "github.com/OffchainLabs/prysm/v6/validator/client/grpc-api"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v6/validator/helpers"
)

func NewValidatorClient(
	validatorConn validatorHelpers.NodeConnection,
	jsonRestHandler beaconApi.RestHandler,
	opt ...beaconApi.ValidatorClientOpt,
) iface.ValidatorClient {
	if features.Get().EnableBeaconRESTApi {
		return beaconApi.NewBeaconApiValidatorClient(jsonRestHandler, opt...)
	} else {
		return grpcApi.NewGrpcValidatorClient(validatorConn.GetGrpcClientConn())
	}
}
