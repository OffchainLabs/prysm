package grpc_api

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/client"
	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// sendEvent forwards ev unless the stream has been canceled or replaced.
func sendEvent(ctx context.Context, eventsChannel chan<- *eventClient.Event, ev *eventClient.Event) bool {
	select {
	case eventsChannel <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *grpcValidatorClient) StartEventStream(ctx context.Context, topics []string, eventsChannel chan<- *eventClient.Event) {
	ctx, span := trace.StartSpan(ctx, "validator.gRPCClient.StartEventStream")
	defer span.End()
	if len(topics) == 0 {
		sendEvent(ctx, eventsChannel, &eventClient.Event{Type: eventClient.EventError, Data: []byte("no topics were added")})
		return
	}
	var head, payload bool
	for _, topic := range topics {
		switch topic {
		case eventClient.EventHead, eventClient.EventHeadV2:
			head = true
		case eventClient.EventExecutionPayloadAvailable:
			payload = true
		default:
			log.WithField("topic", topic).Warn("Unsupported gRPC event topic, ignoring")
		}
	}
	if !head && !payload {
		sendEvent(ctx, eventsChannel, &eventClient.Event{
			Type: eventClient.EventConnectionError,
			Data: []byte("no supported gRPC event topics were requested"),
		})
		return
	}

	subCtx, finish := c.eventStreamGuard.Replace(ctx)
	defer finish()
	rpcClient := c.getClient()

	// The group context stops the sibling stream as soon as one of them fails.
	g, gctx := errgroup.WithContext(subCtx)
	if head {
		g.Go(func() error { return receiveSlotEvents(gctx, rpcClient, eventsChannel) })
	}
	if payload {
		g.Go(func() error {
			err := receivePayloadAvailableEvents(gctx, rpcClient, eventsChannel)
			if status.Code(errors.Cause(err)) == codes.Unimplemented {
				log.Warn("Beacon node does not support payload availability streaming; gRPC PTC votes will wait for the deadline. Upgrade the beacon node or use --beacon-rest-api-provider for on-arrival voting")
				return nil
			}
			return err
		})
	}
	c.eventStreamGuard.MarkRunning(true)
	err := g.Wait()
	c.eventStreamGuard.MarkRunning(false)
	if err != nil && subCtx.Err() == nil {
		sendEvent(subCtx, eventsChannel, &eventClient.Event{
			Type: eventClient.EventConnectionError,
			Data: []byte(errors.Wrap(client.ErrConnectionIssue, err.Error()).Error()),
		})
	}
}

func receiveSlotEvents(ctx context.Context, rpcClient ethpb.BeaconNodeValidatorClient, eventsChannel chan<- *eventClient.Event) error {
	stream, err := rpcClient.StreamSlots(ctx, &ethpb.StreamSlotsRequest{VerifiedOnly: true})
	if err != nil {
		return err
	}
	return forwardStream(ctx, eventsChannel, stream, eventClient.EventHead, func(res *ethpb.StreamSlotsResponse) any {
		return &structs.HeadEvent{
			Slot:                      strconv.FormatUint(uint64(res.Slot), 10),
			PreviousDutyDependentRoot: hexutil.Encode(res.PreviousDutyDependentRoot),
			CurrentDutyDependentRoot:  hexutil.Encode(res.CurrentDutyDependentRoot),
		}
	})
}

func receivePayloadAvailableEvents(ctx context.Context, rpcClient ethpb.BeaconNodeValidatorClient, eventsChannel chan<- *eventClient.Event) error {
	stream, err := rpcClient.StreamExecutionPayloadAvailable(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	return forwardStream(ctx, eventsChannel, stream, eventClient.EventExecutionPayloadAvailable, func(res *ethpb.StreamExecutionPayloadAvailableResponse) any {
		return &structs.ExecutionPayloadAvailableEvent{
			Slot:      strconv.FormatUint(uint64(res.Slot), 10),
			BlockRoot: hexutil.Encode(res.BlockRoot),
		}
	})
}

// forwardStream relays each received message to eventsChannel as a JSON event until Recv or the send fails.
func forwardStream[T any](ctx context.Context, eventsChannel chan<- *eventClient.Event, stream interface{ Recv() (*T, error) }, eventType string, toEvent func(*T) any) error {
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		if res == nil {
			continue
		}
		data, err := json.Marshal(toEvent(res))
		if err != nil {
			return errors.Wrapf(err, "failed to marshal %s event", eventType)
		}
		if !sendEvent(ctx, eventsChannel, &eventClient.Event{Type: eventType, Data: data}) {
			return ctx.Err()
		}
	}
}

func (c *grpcValidatorClient) EventStreamIsRunning() bool {
	return c.eventStreamGuard.IsRunning()
}
