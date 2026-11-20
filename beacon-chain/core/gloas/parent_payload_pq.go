package gloas

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/pkg/errors"
)

// The devnet runs an unmodified execution client, so the three PQ request types must stay empty.
func processParentExecutionPayloadPQ(ctx context.Context, st state.BeaconState, reqs *enginev1.ExecutionRequestsPQ, isParentFull bool, bidRequestsRoot [32]byte) error {
	if reqs == nil {
		reqs = &enginev1.ExecutionRequestsPQ{}
	}
	if len(reqs.ValidatorPreregistrations) != 0 || len(reqs.BuilderPreregistrations) != 0 || len(reqs.VoluntaryExits) != 0 {
		return errors.New("validator preregistration, builder preregistration and voluntary exit requests are not supported")
	}
	if !isParentFull {
		if len(reqs.Deposits) != 0 || len(reqs.Withdrawals) != 0 || len(reqs.Consolidations) != 0 || len(reqs.BuilderDeposits) != 0 || len(reqs.BuilderExits) != 0 {
			return errors.New("parent was empty but parent_execution_requests is non-empty")
		}
		return nil
	}
	root, err := reqs.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute parent execution requests root")
	}
	if root != bidRequestsRoot {
		return errors.Errorf("parent execution requests root mismatch: block=%#x, bid=%#x", root, bidRequestsRoot)
	}
	return ApplyParentExecutionPayload(ctx, st, &enginev1.ExecutionRequestsGloas{
		Deposits:        reqs.Deposits,
		Withdrawals:     reqs.Withdrawals,
		Consolidations:  reqs.Consolidations,
		BuilderDeposits: reqs.BuilderDeposits,
		BuilderExits:    reqs.BuilderExits,
	})
}
