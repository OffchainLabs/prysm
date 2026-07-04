package validator

import (
	"github.com/OffchainLabs/prysm/v7/async/event"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	blockfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/block"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// StreamSlots sends a the block's slot and dependent roots to clients every single time a block is received by the beacon node.
func (vs *Server) StreamSlots(req *ethpb.StreamSlotsRequest, stream ethpb.BeaconNodeValidator_StreamSlotsServer) error {
	ch := make(chan *feed.Event, 1)
	var sub event.Subscription
	if req.VerifiedOnly {
		sub = vs.StateNotifier.StateFeed().Subscribe(ch)
	} else {
		sub = vs.BlockNotifier.BlockFeed().Subscribe(ch)
	}
	defer sub.Unsubscribe()

	for {
		select {
		case ev := <-ch:
			var s primitives.Slot
			var currDependentRoot, prevDependentRoot [32]byte
			if req.VerifiedOnly {
				if ev.Type != statefeed.BlockProcessed {
					continue
				}
				data, ok := ev.Data.(*statefeed.BlockProcessedData)
				if !ok || data == nil {
					continue
				}
				s = data.Slot
				currDependentRoot = data.CurrDependentRoot
				prevDependentRoot = data.PrevDependentRoot
			} else {
				if ev.Type != blockfeed.ReceivedBlock {
					continue
				}
				data, ok := ev.Data.(*blockfeed.ReceivedBlockData)
				if !ok || data == nil {
					continue
				}
				s = data.SignedBlock.Block().Slot()
				currDependentRoot = data.CurrDependentRoot
				prevDependentRoot = data.PrevDependentRoot
			}
			if err := stream.Send(
				&ethpb.StreamSlotsResponse{
					Slot:                      s,
					PreviousDutyDependentRoot: prevDependentRoot[:],
					CurrentDutyDependentRoot:  currDependentRoot[:],
				}); err != nil {
				return status.Errorf(codes.Unavailable, "Could not send over stream: %v", err)
			}
		case <-sub.Err():
			return status.Error(codes.Aborted, "Subscriber closed, exiting goroutine")
		case <-vs.Ctx.Done():
			return status.Error(codes.Canceled, "Context canceled")
		case <-stream.Context().Done():
			return status.Error(codes.Canceled, "Context canceled")
		}
	}
}
