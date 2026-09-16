package grpc_api

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"

	"github.com/OffchainLabs/prysm/v7/api/client"
	eventClient "github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
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
	streamCtx, cancel := context.WithCancel(subCtx)
	var readers sync.WaitGroup
	defer func() {
		cancel()
		readers.Wait()
		c.eventStreamGuard.MarkRunning(false)
	}()

	type result struct {
		payload bool
		err     error
	}
	results := make(chan result, 2)
	start := func(isPayload bool, read func(context.Context, ethpb.BeaconNodeValidatorClient, chan<- *eventClient.Event) error) {
		readers.Add(1)
		go func() {
			defer readers.Done()
			results <- result{payload: isPayload, err: read(streamCtx, rpcClient, eventsChannel)}
		}()
	}
	if head {
		start(false, receiveSlotEvents)
	}
	if payload {
		start(true, receivePayloadAvailableEvents)
	}
	c.eventStreamGuard.MarkRunning(true)
	for {
		select {
		case <-subCtx.Done():
			return
		case res := <-results:
			if res.payload && status.Code(errors.Cause(res.err)) == codes.Unimplemented {
				log.Warn("Beacon node does not support payload availability streaming; gRPC PTC votes will wait for the deadline. Upgrade the beacon node or use --beacon-rest-api-provider for on-arrival voting")
				if !head {
					return
				}
				continue
			}
			cancel()
			readers.Wait()
			c.eventStreamGuard.MarkRunning(false)
			if res.err != nil && subCtx.Err() == nil {
				sendEvent(subCtx, eventsChannel, &eventClient.Event{
					Type: eventClient.EventConnectionError,
					Data: []byte(errors.Wrap(client.ErrConnectionIssue, res.err.Error()).Error()),
				})
			}
			return
		}
	}
}

func receiveSlotEvents(ctx context.Context, rpcClient ethpb.BeaconNodeValidatorClient, eventsChannel chan<- *eventClient.Event) error {
	stream, err := rpcClient.StreamSlots(ctx, &ethpb.StreamSlotsRequest{VerifiedOnly: true})
	if err != nil {
		return err
	}
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		if res == nil {
			continue
		}
		data, err := json.Marshal(&structs.HeadEvent{
			Slot:                      strconv.FormatUint(uint64(res.Slot), 10),
			PreviousDutyDependentRoot: hexutil.Encode(res.PreviousDutyDependentRoot),
			CurrentDutyDependentRoot:  hexutil.Encode(res.CurrentDutyDependentRoot),
		})
		if err != nil {
			return errors.Wrap(err, "failed to marshal head event")
		}
		if !sendEvent(ctx, eventsChannel, &eventClient.Event{Type: eventClient.EventHead, Data: data}) {
			return ctx.Err()
		}
	}
}

func receivePayloadAvailableEvents(ctx context.Context, rpcClient ethpb.BeaconNodeValidatorClient, eventsChannel chan<- *eventClient.Event) error {
	stream, err := rpcClient.StreamExecutionPayloadAvailable(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		if res == nil {
			continue
		}
		data, err := json.Marshal(&structs.ExecutionPayloadAvailableEvent{
			Slot:      strconv.FormatUint(uint64(res.Slot), 10),
			BlockRoot: hexutil.Encode(res.BlockRoot),
		})
		if err != nil {
			return errors.Wrap(err, "failed to marshal payload availability event")
		}
		if !sendEvent(ctx, eventsChannel, &eventClient.Event{Type: eventClient.EventExecutionPayloadAvailable, Data: data}) {
			return ctx.Err()
		}
	}
}

func (c *grpcValidatorClient) EventStreamIsRunning() bool {
	return c.eventStreamGuard.IsRunning()
}
