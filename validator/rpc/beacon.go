package rpc

import (
	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client"
	beaconApi "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api"
	beaconChainClientFactory "github.com/OffchainLabs/prysm/v7/validator/client/beacon-chain-client-factory"
	nodeClientFactory "github.com/OffchainLabs/prysm/v7/validator/client/node-client-factory"
	validatorClientFactory "github.com/OffchainLabs/prysm/v7/validator/client/validator-client-factory"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	grpcopentracing "github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// Initialize a client connect to a beacon node gRPC or HTTP endpoint.
func (s *Server) registerBeaconClient() error {
	streamInterceptor := grpc.WithStreamInterceptor(middleware.ChainStreamClient(
		grpcopentracing.StreamClientInterceptor(),
		grpcprometheus.StreamClientInterceptor,
		grpcretry.StreamClientInterceptor(),
	))
	dialOpts := client.ConstructDialOptions(
		s.grpcMaxCallRecvMsgSize,
		s.beaconNodeCert,
		s.grpcRetries,
		s.grpcRetryDelay,
		streamInterceptor,
	)
	if dialOpts == nil {
		return errors.New("no dial options for beacon chain gRPC client")
	}

	s.ctx = grpcutil.AppendHeaders(s.ctx, s.grpcHeaders)

	grpcProvider, err := grpcutil.NewGrpcConnectionProvider(s.ctx, s.beaconNodeEndpoint, dialOpts)
	if err != nil {
		return errors.Wrapf(err, "could not dial endpoint: %s", s.beaconNodeEndpoint)
	}
	if s.beaconNodeCert != "" {
		log.Info("Established secure gRPC connection")
	}
	s.healthClient = ethpb.NewHealthClient(grpcProvider.CurrentConn())

	restProvider, err := rest.NewRestConnectionProvider(
		s.beaconApiEndpoint,
		rest.WithHttpHeaders(s.beaconApiHeaders),
		rest.WithHttpTimeout(s.beaconApiTimeout),
		rest.WithTracing(),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create REST connection provider")
	}

	conn := validatorHelpers.NewNodeConnection(grpcProvider, restProvider)

	restHandler := beaconApi.NewBeaconApiRestHandler(
		*restProvider.HttpClient(),
		restProvider.CurrentHost(),
	)

	s.chainClient = beaconChainClientFactory.NewChainClient(conn, restHandler)
	s.nodeClient = nodeClientFactory.NewNodeClient(conn, restHandler)
	s.beaconNodeValidatorClient = validatorClientFactory.NewValidatorClient(conn, restHandler)
	return nil
}
