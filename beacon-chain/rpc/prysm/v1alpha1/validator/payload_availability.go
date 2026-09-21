package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const payloadAvailabilityBufferSize = 16

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// StreamExecutionPayloadAvailable announces payload availability independently of execution validation.
func (vs *Server) StreamExecutionPayloadAvailable(_ *emptypb.Empty, stream ethpb.BeaconNodeValidator_StreamExecutionPayloadAvailableServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	events := make(chan *feed.Event, payloadAvailabilityBufferSize)
	sub := vs.StateNotifier.StateFeed().Subscribe(events)
	defer sub.Unsubscribe()

	outbox := make(chan *ethpb.StreamExecutionPayloadAvailableResponse, payloadAvailabilityBufferSize)
	sendErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case response := <-outbox:
				if err := stream.Send(response); err != nil {
					sendErr <- err
					return
				}
			}
		}
	}()

	for {
		select {
		case ev := <-events:
			if ev == nil || ev.Type != statefeed.ExecutionPayloadAvailable {
				continue
			}
			data, ok := ev.Data.(*statefeed.ExecutionPayloadAvailableData)
			if !ok || data == nil {
				continue
			}
			response := &ethpb.StreamExecutionPayloadAvailableResponse{
				Slot:      data.Slot,
				BlockRoot: append([]byte(nil), data.BlockRoot[:]...),
			}
			select {
			case outbox <- response:
			case <-ctx.Done():
				return status.Error(codes.Canceled, "Context canceled")
			default:
				// Returning closes the gRPC stream and releases any blocked Send.
				return status.Error(codes.ResourceExhausted, "Payload availability client is not reading fast enough")
			}
		case err := <-sendErr:
			return status.Errorf(codes.Unavailable, "Could not send payload availability event: %v", err)
		case <-sub.Err():
			return status.Error(codes.Aborted, "Subscriber closed, exiting goroutine")
		case <-vs.Ctx.Done():
			return status.Error(codes.Canceled, "Context canceled")
		case <-ctx.Done():
			return status.Error(codes.Canceled, "Context canceled")
		}
	}
}
