package validator

import (
	"context"
	"fmt"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/encoding/ssz"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetInclusionList retrieves the inclusion list for the specified slot.
// The slot must be the current or next slot. The inclusion list is built using
// committee indices, the execution payload header from beacon state, and the transactions from the execution engine.
func (vs *Server) GetInclusionList(ctx context.Context, request *ethpb.GetInclusionListRequest) (*ethpb.InclusionList, error) {
	currentSlot := vs.TimeFetcher.CurrentSlot()
	if request.Slot != currentSlot && request.Slot+1 != currentSlot {
		return nil, status.Errorf(codes.InvalidArgument, "requested slot %d is not current or next slot", request.Slot)
	}

	st, err := vs.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get head state: %v", err)
	}
	st, err = transition.ProcessSlotsIfPossible(ctx, st, request.Slot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to process slots: %v", err)
	}
	indices, err := helpers.GetInclusionListCommittee(ctx, st, request.Slot)
	if err != nil {
		return nil, err
	}
	root, err := ssz.InclusionListRoot(indices)
	if err != nil {
		return nil, err
	}

	header, err := st.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, err
	}

	// Fetch the transactions associated with the inclusion list.
	txs, err := vs.ExecutionEngineCaller.GetInclusionList(ctx, [32]byte(header.BlockHash()))
	if err != nil {
		return nil, err
	}

	return &ethpb.InclusionList{
		Slot:                       request.Slot,
		InclusionListCommitteeRoot: root[:],
		Transactions:               txs,
	}, nil
}

// SubmitInclusionList broadcasts a signed inclusion list to the P2P network and caches it locally.
func (vs *Server) SubmitInclusionList(ctx context.Context, il *ethpb.SignedInclusionList) (*emptypb.Empty, error) {
	if err := vs.P2P.Broadcast(ctx, il); err != nil {
		return nil, err
	}

	vs.InclusionLists.Add(il.Message.Slot, il.Message.ValidatorIndex, il.Message.Transactions)

	log.WithFields(logrus.Fields{
		"slot":          il.Message.Slot,
		"committeeRoot": fmt.Sprintf("%#x", il.Message.InclusionListCommitteeRoot),
		"txCount":       len(il.Message.Transactions),
		"signature":     fmt.Sprintf("%#x", il.Signature),
	}).Info("Inclusion list added to cache")

	return &emptypb.Empty{}, nil
}
