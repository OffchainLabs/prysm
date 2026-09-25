package client

import (
	"context"
	"net"
	"testing"
	"time"

	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	grpcApi "github.com/OffchainLabs/prysm/v7/validator/client/grpc-api"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

type payloadAvailabilityStubServer struct {
	ethpb.UnimplementedBeaconNodeValidatorServer
	responses chan *ethpb.StreamExecutionPayloadAvailableResponse
}

func (s *payloadAvailabilityStubServer) StreamSlots(_ *ethpb.StreamSlotsRequest, stream ethpb.BeaconNodeValidator_StreamSlotsServer) error {
	<-stream.Context().Done()
	return stream.Context().Err()
}

func (s *payloadAvailabilityStubServer) StreamExecutionPayloadAvailable(_ *emptypb.Empty, stream ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error {
	for {
		select {
		case res := <-s.responses:
			if err := stream.Send(res); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

func TestPayloadAvailability_GRPCReleasesPTCWaiter(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stub := &payloadAvailabilityStubServer{responses: make(chan *ethpb.StreamExecutionPayloadAvailableResponse, 1)}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	ethpb.RegisterBeaconNodeValidatorServer(server, stub)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := validatorHelpers.NewNodeConnection(validatorHelpers.WithGRPC(ctx, "passthrough:///payload-availability", []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	}))
	require.NoError(t, err)
	t.Cleanup(conn.GetGrpcConnectionProvider().Close)
	grpcClient := grpcApi.NewGrpcValidatorClient(conn)
	events := make(chan *eventClient.Event, 1)
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		grpcClient.StartEventStream(ctx, eventClient.DefaultEventTopics, events)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-streamDone:
		case <-time.After(5 * time.Second):
			t.Error("gRPC event streams did not stop after cancellation")
		}
	})

	const slot = primitives.Slot(42)
	root := [32]byte{0xab, 0xcd}
	v := &validator{
		genesisTime:         time.Now().Add(time.Hour),
		payloadAvailability: newPayloadAvailability(),
	}
	deadline, err := v.slotComponentDeadline(slot, params.BeaconConfig().PayloadAttestationDueBPS)
	require.NoError(t, err)
	waiterDone := make(chan struct{})
	go func() {
		defer close(waiterDone)
		v.waitForPayloadAvailableOrDeadline(ctx, slot)
	}()

	stub.responses <- &ethpb.StreamExecutionPayloadAvailableResponse{Slot: slot, BlockRoot: root[:]}
	select {
	case ev := <-events:
		require.Equal(t, eventClient.EventExecutionPayloadAvailable, ev.Type)
		v.ProcessEvent(ctx, ev)
	case <-ctx.Done():
		t.Fatal("payload availability did not reach the validator: ", ctx.Err())
	}

	select {
	case <-waiterDone:
	case <-ctx.Done():
		t.Fatal("PTC waiter did not return after payload availability: ", ctx.Err())
	}
	require.NoError(t, ctx.Err())
	require.Equal(t, true, time.Now().Before(deadline), "PTC waiter must return before its deadline")
	gotRoot, available := v.payloadAvailability.payloadRoot(slot)
	require.Equal(t, true, available)
	require.Equal(t, root, gotRoot)
}
