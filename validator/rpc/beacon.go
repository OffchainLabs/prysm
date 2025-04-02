package rpc

import (
	"net/http"

	middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	grpcopentracing "github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/pkg/errors"
	api_client "github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/chain"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/node"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/validator_api"
	grpcutil "github.com/prysmaticlabs/prysm/v5/api/grpc"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/client"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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

	grpcConn, err := grpc.DialContext(s.ctx, s.beaconNodeEndpoint, dialOpts...)
	if err != nil {
		return errors.Wrapf(err, "could not dial endpoint: %s", s.beaconNodeEndpoint)
	}
	if s.beaconNodeCert != "" {
		log.Info("Established secure gRPC connection")
	}
	s.healthClient = ethpb.NewHealthClient(grpcConn)

	conn := validatorHelpers.NewNodeConnection(
		grpcConn,
		s.beaconApiEndpoint,
		s.beaconApiTimeout,
	)

	restHandler := api_client.NewBeaconApiJsonRestHandler(
		http.Client{Timeout: s.beaconApiTimeout, Transport: otelhttp.NewTransport(http.DefaultTransport)},
		s.beaconApiEndpoint,
	)

	s.chainClient = chain.NewClient(conn, restHandler)
	s.nodeClient = node.NewClient(conn, restHandler)
	s.beaconNodeValidatorClient = validator_api.NewClient(conn, restHandler)
	return nil
}
