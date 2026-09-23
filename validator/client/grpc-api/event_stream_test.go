package grpc_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

type eventStreamServer struct {
	ethpb.UnimplementedBeaconNodeValidatorServer
	slots   func(*ethpb.StreamSlotsRequest, ethpb.BeaconNodeValidator_StreamSlotsServer) error
	payload func(ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error
}

func (s *eventStreamServer) StreamSlots(req *ethpb.StreamSlotsRequest, stream ethpb.BeaconNodeValidator_StreamSlotsServer) error {
	if s.slots == nil {
		return status.Error(codes.Unimplemented, "slot stream not supported")
	}
	return s.slots(req, stream)
}

func (s *eventStreamServer) StreamExecutionPayloadAvailable(_ *emptypb.Empty, stream ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error {
	if s.payload == nil {
		return status.Error(codes.Unimplemented, "payload stream not supported")
	}
	return s.payload(stream)
}

func eventStreamRPCClient(t *testing.T, server *eventStreamServer, opts ...grpc.DialOption) ethpb.BeaconNodeValidatorClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	ethpb.RegisterBeaconNodeValidatorServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	opts = append(opts,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	conn, err := grpc.NewClient("passthrough:///event-stream-test", opts...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	return ethpb.NewBeaconNodeValidatorClient(conn)
}

func eventStreamClient(rpcClient ethpb.BeaconNodeValidatorClient) *grpcValidatorClient {
	return &grpcValidatorClient{
		grpcClientManager: newGrpcClientManager(&validatorHelpers.NodeConnection{}, func(grpc.ClientConnInterface) ethpb.BeaconNodeValidatorClient {
			return rpcClient
		}),
	}
}

func runEventStream(t *testing.T, client *grpcValidatorClient, topics []string, events chan<- *eventClient.Event) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		client.StartEventStream(ctx, topics, events)
	}()
	t.Cleanup(func() {
		cancel()
		awaitEventStreamValue(t, done)
	})
	return cancel, done
}

func awaitEventStreamValue[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event stream")
		var zero T
		return zero
	}
}

type unimplementedPayloadClient struct {
	ethpb.BeaconNodeValidatorClient
	setup bool
	calls atomic.Int32
}

func (c *unimplementedPayloadClient) StreamExecutionPayloadAvailable(ctx context.Context, req *emptypb.Empty, opts ...grpc.CallOption) (ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableClient, error) {
	c.calls.Add(1)
	if c.setup {
		return nil, status.Error(codes.Unimplemented, "old beacon node")
	}
	return c.BeaconNodeValidatorClient.StreamExecutionPayloadAvailable(ctx, req, opts...)
}

type eventStreamSlotHookClient struct {
	ethpb.BeaconNodeValidatorClient
	beforeSlots func()
}

func (c *eventStreamSlotHookClient) StreamSlots(ctx context.Context, req *ethpb.StreamSlotsRequest, opts ...grpc.CallOption) (ethpb.BeaconNodeValidator_StreamSlotsClient, error) {
	c.beforeSlots()
	return c.BeaconNodeValidatorClient.StreamSlots(ctx, req, opts...)
}

func TestStartEventStream(t *testing.T) {
	t.Run("topic selection", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			wantErrType string
			topics      []string
			wantHead    bool
			wantPayload bool
			wantWarning bool
		}{
			{name: "no topics", wantErrType: eventClient.EventError},
			{name: "unsupported topics only", topics: []string{"unsupportedTopic"}, wantErrType: eventClient.EventConnectionError, wantWarning: true},
			{name: "head", topics: []string{eventClient.EventHead}, wantHead: true},
			{name: "head v2 retains slot stream", topics: []string{eventClient.EventHeadV2}, wantHead: true},
			{name: "head with unsupported topic", topics: []string{eventClient.EventHead, "unsupportedTopic"}, wantHead: true, wantWarning: true},
			{name: "payload only", topics: []string{eventClient.EventExecutionPayloadAvailable}, wantPayload: true},
			{name: "default topics", topics: eventClient.DefaultEventTopics, wantHead: true, wantPayload: true},
			{
				name:        "duplicate topics with head first",
				topics:      []string{eventClient.EventHead, eventClient.EventHeadV2, eventClient.EventHead, eventClient.EventExecutionPayloadAvailable, eventClient.EventExecutionPayloadAvailable},
				wantHead:    true,
				wantPayload: true,
			},
			{
				name:        "duplicate topics with payload and head_v2 first",
				topics:      []string{eventClient.EventExecutionPayloadAvailable, eventClient.EventHeadV2, eventClient.EventHead, eventClient.EventExecutionPayloadAvailable, eventClient.EventHeadV2},
				wantHead:    true,
				wantPayload: true,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				hook := logTest.NewGlobal()
				root := bytes.Repeat([]byte{0xab}, 32)
				slotRequests := make(chan *ethpb.StreamSlotsRequest, 1)
				server := &eventStreamServer{
					slots: func(req *ethpb.StreamSlotsRequest, stream ethpb.BeaconNodeValidator_StreamSlotsServer) error {
						slotRequests <- req
						if err := stream.Send(&ethpb.StreamSlotsResponse{Slot: 123, PreviousDutyDependentRoot: []byte{1}, CurrentDutyDependentRoot: []byte{2}}); err != nil {
							return err
						}
						<-stream.Context().Done()
						return stream.Context().Err()
					},
					payload: func(stream ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error {
						if err := stream.Send(&ethpb.StreamExecutionPayloadAvailableResponse{Slot: 123, BlockRoot: root}); err != nil {
							return err
						}
						<-stream.Context().Done()
						return stream.Context().Err()
					},
				}
				var slotCalls, payloadCalls atomic.Int32
				countStreams := grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, conn *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					switch method {
					case "/ethereum.eth.v1alpha1.BeaconNodeValidator/StreamSlots":
						slotCalls.Add(1)
					case "/ethereum.eth.v1alpha1.BeaconNodeValidator/StreamExecutionPayloadAvailable":
						payloadCalls.Add(1)
					}
					return streamer(ctx, desc, conn, method, opts...)
				})
				client := eventStreamClient(eventStreamRPCClient(t, server, countStreams))
				events := make(chan *eventClient.Event, 2)
				cancel, done := runEventStream(t, client, tc.topics, events)
				if tc.wantErrType != "" {
					require.Equal(t, tc.wantErrType, awaitEventStreamValue(t, events).Type)
				} else {
					count := 0
					for _, want := range []bool{tc.wantHead, tc.wantPayload} {
						if want {
							count++
						}
					}
					received := make(map[string]*eventClient.Event)
					for range count {
						ev := awaitEventStreamValue(t, events)
						received[ev.Type] = ev
					}
					if tc.wantHead {
						require.Equal(t, true, awaitEventStreamValue(t, slotRequests).VerifiedOnly)
						head := received[eventClient.EventHead]
						require.NotNil(t, head)
						var decoded structs.HeadEvent
						require.NoError(t, json.Unmarshal(head.Data, &decoded))
						require.Equal(t, "123", decoded.Slot)
						require.Equal(t, "0x01", decoded.PreviousDutyDependentRoot)
						require.Equal(t, "0x02", decoded.CurrentDutyDependentRoot)
					}
					if tc.wantPayload {
						payload := received[eventClient.EventExecutionPayloadAvailable]
						require.NotNil(t, payload)
						var availability structs.ExecutionPayloadAvailableEvent
						require.NoError(t, json.Unmarshal(payload.Data, &availability))
						require.Equal(t, "123", availability.Slot)
						require.Equal(t, "0x"+strings.Repeat("ab", 32), availability.BlockRoot)
					}
					require.Eventually(t, client.EventStreamIsRunning, 5*time.Second, time.Millisecond)
					cancel()
				}
				awaitEventStreamValue(t, done)
				require.Equal(t, false, client.EventStreamIsRunning())
				require.Equal(t, streamCallCount(tc.wantHead), slotCalls.Load())
				require.Equal(t, streamCallCount(tc.wantPayload), payloadCalls.Load())
				if tc.wantWarning {
					require.LogsContain(t, hook, "Unsupported gRPC event topic")
				}
			})
		}
	})

	t.Run("unimplemented payload keeps heads", func(t *testing.T) {
		for _, stage := range []string{"setup", "receive"} {
			t.Run(stage, func(t *testing.T) {
				hook := logTest.NewGlobal()
				heads := make(chan *ethpb.StreamSlotsResponse)
				headsStopped := make(chan struct{}, 1)
				var upgraded atomic.Bool
				server := &eventStreamServer{
					slots: func(_ *ethpb.StreamSlotsRequest, stream ethpb.BeaconNodeValidator_StreamSlotsServer) error {
						defer func() { headsStopped <- struct{}{} }()
						for {
							select {
							case head := <-heads:
								if err := stream.Send(head); err != nil {
									return err
								}
							case <-stream.Context().Done():
								return stream.Context().Err()
							}
						}
					},
					payload: func(stream ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error {
						if !upgraded.Load() {
							return status.Error(codes.Unimplemented, "old beacon node")
						}
						if err := stream.Send(&ethpb.StreamExecutionPayloadAvailableResponse{Slot: 43, BlockRoot: make([]byte, 32)}); err != nil {
							return err
						}
						<-stream.Context().Done()
						return stream.Context().Err()
					},
				}
				rpcClient := &unimplementedPayloadClient{
					BeaconNodeValidatorClient: eventStreamRPCClient(t, server),
					setup:                     stage == "setup",
				}
				client := eventStreamClient(rpcClient)
				for attempt := range 2 {
					if attempt == 1 {
						upgraded.Store(true)
						rpcClient.setup = false
					}
					events := make(chan *eventClient.Event, 2)
					cancel, done := runEventStream(t, client, eventClient.DefaultEventTopics, events)
					if attempt == 0 {
						require.Eventually(t, func() bool {
							for _, entry := range hook.AllEntries() {
								if strings.Contains(entry.Message, "does not support payload availability streaming") {
									return true
								}
							}
							return false
						}, 5*time.Second, time.Millisecond)
					}
					select {
					case heads <- &ethpb.StreamSlotsResponse{Slot: 42}:
					case <-time.After(5 * time.Second):
						t.Fatal("slot stream stopped after unsupported payload stream")
					}
					received := make(map[string]*eventClient.Event)
					for range attempt + 1 {
						ev := awaitEventStreamValue(t, events)
						received[ev.Type] = ev
					}
					require.NotNil(t, received[eventClient.EventHead])
					if attempt == 1 {
						payload := received[eventClient.EventExecutionPayloadAvailable]
						require.NotNil(t, payload)
						var decoded structs.ExecutionPayloadAvailableEvent
						require.NoError(t, json.Unmarshal(payload.Data, &decoded))
						require.Equal(t, "43", decoded.Slot)
					}
					require.Eventually(t, client.EventStreamIsRunning, 5*time.Second, time.Millisecond)
					cancel()
					awaitEventStreamValue(t, done)
					awaitEventStreamValue(t, headsStopped)
					require.Equal(t, int32(attempt+1), rpcClient.calls.Load())
				}
			})
		}
	})

	t.Run("unimplemented payload only stops", func(t *testing.T) {
		rpcClient := &unimplementedPayloadClient{BeaconNodeValidatorClient: eventStreamRPCClient(t, &eventStreamServer{})}
		client := eventStreamClient(rpcClient)
		events := make(chan *eventClient.Event, 1)
		_, done := runEventStream(t, client, []string{eventClient.EventExecutionPayloadAvailable}, events)
		awaitEventStreamValue(t, done)
		require.Equal(t, int32(1), rpcClient.calls.Load())
		require.Equal(t, false, client.EventStreamIsRunning())
		select {
		case ev := <-events:
			t.Fatalf("unsupported payload stream produced unexpected event: %+v", ev)
		default:
		}
	})

	t.Run("failure cancels sibling", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			stream string
			code   codes.Code
		}{
			{name: "slots unavailable", stream: "slots", code: codes.Unavailable},
			{name: "payload unavailable", stream: "payload", code: codes.Unavailable},
			{name: "slots unimplemented", stream: "slots", code: codes.Unimplemented},
		} {
			t.Run(tc.name, func(t *testing.T) {
				started := make(chan struct{}, 2)
				stopped := make(chan struct{}, 2)
				fail := make(chan struct{})
				handler := func(ctx context.Context, stream string) error {
					started <- struct{}{}
					defer func() { stopped <- struct{}{} }()
					if stream == tc.stream {
						select {
						case <-fail:
							return status.Error(tc.code, "stream interrupted")
						case <-ctx.Done():
							return ctx.Err()
						}
					}
					<-ctx.Done()
					return ctx.Err()
				}
				server := &eventStreamServer{
					slots: func(_ *ethpb.StreamSlotsRequest, stream ethpb.BeaconNodeValidator_StreamSlotsServer) error {
						return handler(stream.Context(), "slots")
					},
					payload: func(stream ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error {
						return handler(stream.Context(), "payload")
					},
				}
				client := eventStreamClient(eventStreamRPCClient(t, server))
				events := make(chan *eventClient.Event, 1)
				_, done := runEventStream(t, client, eventClient.DefaultEventTopics, events)
				awaitEventStreamValue(t, started)
				awaitEventStreamValue(t, started)
				close(fail)
				ev := awaitEventStreamValue(t, events)
				require.Equal(t, eventClient.EventConnectionError, ev.Type)
				require.StringContains(t, "stream interrupted", string(ev.Data))
				require.StringContains(t, tc.code.String(), string(ev.Data))
				awaitEventStreamValue(t, done)
				awaitEventStreamValue(t, stopped)
				awaitEventStreamValue(t, stopped)
				require.Equal(t, false, client.EventStreamIsRunning())
			})
		}
	})

	t.Run("replacement drains blocked streams", func(t *testing.T) {
		started := make(chan struct{}, 4)
		stopped := make(chan struct{}, 4)
		server := &eventStreamServer{
			slots: func(_ *ethpb.StreamSlotsRequest, stream ethpb.BeaconNodeValidator_StreamSlotsServer) error {
				defer func() { stopped <- struct{}{} }()
				if err := stream.Send(&ethpb.StreamSlotsResponse{Slot: 7}); err != nil {
					return err
				}
				started <- struct{}{}
				<-stream.Context().Done()
				return stream.Context().Err()
			},
			payload: func(stream ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error {
				defer func() { stopped <- struct{}{} }()
				if err := stream.Send(&ethpb.StreamExecutionPayloadAvailableResponse{Slot: 7, BlockRoot: make([]byte, 32)}); err != nil {
					return err
				}
				started <- struct{}{}
				<-stream.Context().Done()
				return stream.Context().Err()
			},
		}
		client := eventStreamClient(eventStreamRPCClient(t, server))
		blockedEvents := make(chan *eventClient.Event)
		_, oldDone := runEventStream(t, client, eventClient.DefaultEventTopics, blockedEvents)
		awaitEventStreamValue(t, started)
		awaitEventStreamValue(t, started)
		events := make(chan *eventClient.Event, 2)
		cancel, done := runEventStream(t, client, eventClient.DefaultEventTopics, events)
		awaitEventStreamValue(t, oldDone)
		awaitEventStreamValue(t, stopped)
		awaitEventStreamValue(t, stopped)
		received := make(map[string]bool)
		for range 2 {
			received[awaitEventStreamValue(t, events).Type] = true
		}
		require.Equal(t, true, received[eventClient.EventHead])
		require.Equal(t, true, received[eventClient.EventExecutionPayloadAvailable])
		require.Eventually(t, client.EventStreamIsRunning, 5*time.Second, time.Millisecond)
		cancel()
		awaitEventStreamValue(t, done)
		awaitEventStreamValue(t, stopped)
		awaitEventStreamValue(t, stopped)
		require.Equal(t, false, client.EventStreamIsRunning())
	})

	t.Run("captures client before host switch", func(t *testing.T) {
		provider := &mockProvider{hosts: []string{"first", "second"}}
		firstRPC := eventStreamRPCClient(t, &eventStreamServer{
			slots: func(_ *ethpb.StreamSlotsRequest, stream ethpb.BeaconNodeValidator_StreamSlotsServer) error {
				if err := stream.Send(&ethpb.StreamSlotsResponse{Slot: 10}); err != nil {
					return err
				}
				<-stream.Context().Done()
				return stream.Context().Err()
			},
			payload: func(stream ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error {
				if err := stream.Send(&ethpb.StreamExecutionPayloadAvailableResponse{Slot: 10, BlockRoot: make([]byte, 32)}); err != nil {
					return err
				}
				<-stream.Context().Done()
				return stream.Context().Err()
			},
		})
		secondRPC := eventStreamRPCClient(t, &eventStreamServer{
			payload: func(stream ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error {
				if err := stream.Send(&ethpb.StreamExecutionPayloadAvailableResponse{Slot: 20, BlockRoot: make([]byte, 32)}); err != nil {
					return err
				}
				<-stream.Context().Done()
				return stream.Context().Err()
			},
		})
		firstRPC = &eventStreamSlotHookClient{BeaconNodeValidatorClient: firstRPC, beforeSlots: provider.nextHost}
		conn, err := validatorHelpers.NewNodeConnection(validatorHelpers.WithGRPCProvider(provider))
		require.NoError(t, err)
		client := &grpcValidatorClient{
			grpcClientManager: newGrpcClientManager(conn, func(grpc.ClientConnInterface) ethpb.BeaconNodeValidatorClient {
				if provider.CurrentHost() == "first" {
					return firstRPC
				}
				return secondRPC
			}),
		}
		events := make(chan *eventClient.Event, 2)
		cancel, done := runEventStream(t, client, eventClient.DefaultEventTopics, events)
		for range 2 {
			ev := awaitEventStreamValue(t, events)
			var decoded struct {
				Slot string `json:"slot"`
			}
			require.NoError(t, json.Unmarshal(ev.Data, &decoded))
			require.Equal(t, "10", decoded.Slot)
		}
		require.Equal(t, "second", provider.CurrentHost())
		cancel()
		awaitEventStreamValue(t, done)
		_, _ = runEventStream(t, client, []string{eventClient.EventExecutionPayloadAvailable}, events)
		ev := awaitEventStreamValue(t, events)
		require.Equal(t, eventClient.EventExecutionPayloadAvailable, ev.Type)
		var payload structs.ExecutionPayloadAvailableEvent
		require.NoError(t, json.Unmarshal(ev.Data, &payload))
		require.Equal(t, "20", payload.Slot)
	})
}

func streamCallCount(want bool) int32 {
	if want {
		return 1
	}
	return 0
}
