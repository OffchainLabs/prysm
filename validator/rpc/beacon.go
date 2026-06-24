package rpc

import (
	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client"
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

	conn, err := validatorHelpers.NewNodeConnection(
		validatorHelpers.WithGRPC(s.ctx, s.beaconNodeEndpoint, dialOpts),
		validatorHelpers.WithREST(s.beaconApiEndpoint,
			rest.WithHttpHeaders(s.beaconApiHeaders),
			rest.WithHttpTimeout(s.beaconApiTimeout),
			rest.WithTracing(),
		),
	)
	if err != nil {
		return err
	}
	if s.beaconNodeCert != "" && s.beaconNodeEndpoint != "" {
		log.Info("Established secure gRPC connection")
	}
	if grpcConn := conn.GetGrpcClientConn(); grpcConn != nil {
		s.healthClient = ethpb.NewHealthClient(grpcConn)
	}

	s.chainClient = beaconChainClientFactory.NewChainClient(conn)
	s.nodeClient = nodeClientFactory.NewNodeClient(conn)
	s.beaconNodeValidatorClient = validatorClientFactory.NewValidatorClient(conn)
	return nil
}
