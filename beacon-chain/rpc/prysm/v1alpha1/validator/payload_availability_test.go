//go:build minimal

package validator

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/async/event"
	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

type payloadAvailabilityTestFeed struct {
	event.Feed
	subscribed chan struct{}
	sub        event.Subscription
}

func (f *payloadAvailabilityTestFeed) Subscribe(channel any) event.Subscription {
	f.sub = f.Feed.Subscribe(channel)
	close(f.subscribed)
	return f.sub
}

func newPayloadAvailabilityTestServer(ctx context.Context) (*Server, *payloadAvailabilityTestFeed) {
	f := &payloadAvailabilityTestFeed{subscribed: make(chan struct{})}
	return &Server{Ctx: ctx, StateNotifier: &chainMock.SimpleNotifier{Feed: f}}, f
}

func waitPayloadAvailabilitySignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for payload availability stream")
	}
}

func waitPayloadAvailabilityResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for payload availability stream result")
		return nil
	}
}

func TestServer_StreamExecutionPayloadAvailable(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server, f := newPayloadAvailabilityTestServer(ctx)
	stream := mock.NewMockBeaconNodeValidator_StreamExecutionPayloadAvailableServer(gomock.NewController(t))
	stream.EXPECT().Context().Return(ctx)
	root := [32]byte{0xAB}
	sent := make(chan struct{})
	stream.EXPECT().Send(&ethpb.StreamExecutionPayloadAvailableResponse{Slot: 42, BlockRoot: root[:]}).
		DoAndReturn(func(*ethpb.StreamExecutionPayloadAvailableResponse) error {
			close(sent)
			return nil
		})
	result := make(chan error, 1)
	go func() { result <- server.StreamExecutionPayloadAvailable(&emptypb.Empty{}, stream) }()
	waitPayloadAvailabilitySignal(t, f.subscribed)

	f.Send((*feed.Event)(nil))
	f.Send(&feed.Event{Type: statefeed.BlockProcessed, Data: &statefeed.BlockProcessedData{Slot: 42}})
	f.Send(&feed.Event{Type: statefeed.ExecutionPayloadAvailable, Data: &statefeed.BlockProcessedData{Slot: 42}})
	f.Send(&feed.Event{Type: statefeed.ExecutionPayloadAvailable, Data: (*statefeed.ExecutionPayloadAvailableData)(nil)})
	f.Send(&feed.Event{Type: statefeed.ExecutionPayloadAvailable, Data: &statefeed.ExecutionPayloadAvailableData{Slot: 42, BlockRoot: root}})
	waitPayloadAvailabilitySignal(t, sent)
	cancel()
	require.Equal(t, codes.Canceled, status.Code(waitPayloadAvailabilityResult(t, result)))
	require.Equal(t, 0, f.Send(&feed.Event{Type: statefeed.ExecutionPayloadAvailable}))
}

func TestServer_StreamExecutionPayloadAvailable_Shutdown(t *testing.T) {
	for _, reason := range []string{"server canceled", "client canceled", "subscription closed", "send failed"} {
		t.Run(reason, func(t *testing.T) {
			serverCtx, cancelServer := context.WithCancel(t.Context())
			defer cancelServer()
			clientCtx, cancelClient := context.WithCancel(t.Context())
			defer cancelClient()
			server, f := newPayloadAvailabilityTestServer(serverCtx)
			stream := mock.NewMockBeaconNodeValidator_StreamExecutionPayloadAvailableServer(gomock.NewController(t))
			stream.EXPECT().Context().Return(clientCtx)
			if reason == "send failed" {
				stream.EXPECT().Send(gomock.Any()).Return(errors.New("transport failed"))
			}
			result := make(chan error, 1)
			go func() { result <- server.StreamExecutionPayloadAvailable(&emptypb.Empty{}, stream) }()
			waitPayloadAvailabilitySignal(t, f.subscribed)
			wantCode := codes.Canceled
			switch reason {
			case "server canceled":
				cancelServer()
			case "client canceled":
				cancelClient()
			case "subscription closed":
				f.sub.Unsubscribe()
				wantCode = codes.Aborted
			case "send failed":
				f.Send(&feed.Event{Type: statefeed.ExecutionPayloadAvailable, Data: &statefeed.ExecutionPayloadAvailableData{Slot: 42}})
				wantCode = codes.Unavailable
			}
			require.Equal(t, wantCode, status.Code(waitPayloadAvailabilityResult(t, result)))
			require.Equal(t, 0, f.Send(&feed.Event{Type: statefeed.ExecutionPayloadAvailable}))
		})
	}
}

type payloadAvailabilityObservedServer struct {
	*Server
	sendStarted  chan struct{}
	sendFinished chan error
	returned     chan error
}

func (s *payloadAvailabilityObservedServer) StreamExecutionPayloadAvailable(req *emptypb.Empty, stream ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error {
	err := s.Server.StreamExecutionPayloadAvailable(req, &payloadAvailabilityObservedStream{
		BeaconNodeValidator_StreamExecutionPayloadAvailableServer: stream,
		started:  s.sendStarted,
		finished: s.sendFinished,
	})
	s.returned <- err
	return err
}

type payloadAvailabilityObservedStream struct {
	ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer
	started  chan<- struct{}
	finished chan<- error
}

func (s *payloadAvailabilityObservedStream) Send(response *ethpb.StreamExecutionPayloadAvailableResponse) error {
	s.started <- struct{}{}
	err := s.BeaconNodeValidator_StreamExecutionPayloadAvailableServer.Send(response)
	s.finished <- err
	return err
}

func TestServer_StreamExecutionPayloadAvailable_SlowClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	server, f := newPayloadAvailabilityTestServer(ctx)
	observed := &payloadAvailabilityObservedServer{
		Server:       server,
		sendStarted:  make(chan struct{}, 1),
		sendFinished: make(chan error, 1),
		returned:     make(chan error, 1),
	}
	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })
	grpcServer := grpc.NewServer()
	ethpb.RegisterBeaconNodeValidatorServer(grpcServer, observed)
	t.Cleanup(grpcServer.Stop)
	go func() { _ = grpcServer.Serve(listener) }()
	conn, err := grpc.NewClient("passthrough:///payload-availability", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.DialContext(ctx) }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = ethpb.NewBeaconNodeValidatorClient(conn).StreamExecutionPayloadAvailable(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	waitPayloadAvailabilitySignal(t, f.subscribed)

	ev := &feed.Event{Type: statefeed.ExecutionPayloadAvailable, Data: &statefeed.ExecutionPayloadAvailableData{Slot: 42}}
	blocked := false
	for i := 0; i < 10000; i++ {
		f.Send(ev)
		waitPayloadAvailabilitySignal(t, observed.sendStarted)
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case err := <-observed.sendFinished:
			timer.Stop()
			require.NoError(t, err)
		case <-timer.C:
			blocked = true
		}
		if blocked {
			break
		}
	}
	require.Equal(t, true, blocked, "Client must exhaust gRPC flow control without reading")

	published := make(chan struct{})
	go func() {
		for i := 0; i <= payloadAvailabilityBufferSize; i++ {
			f.Send(ev)
		}
		close(published)
	}()
	waitPayloadAvailabilitySignal(t, published)
	require.Equal(t, codes.ResourceExhausted, status.Code(waitPayloadAvailabilityResult(t, observed.returned)))
	require.NotNil(t, waitPayloadAvailabilityResult(t, observed.sendFinished), "Returning the handler must release the blocked transport send")
	require.Equal(t, 0, f.Send(ev))
}
