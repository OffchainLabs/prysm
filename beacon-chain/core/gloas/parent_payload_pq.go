package gloas

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/pkg/errors"
)

// Only empty requests are accepted until the PQ request processing is ported.
func processParentExecutionPayloadPQ(ctx context.Context, st state.BeaconState, reqs *enginev1.ExecutionRequestsPQ, isParentFull bool, bidRequestsRoot [32]byte) error {
	if reqs == nil {
		reqs = &enginev1.ExecutionRequestsPQ{}
	}
	if len(reqs.Deposits) != 0 || len(reqs.Withdrawals) != 0 || len(reqs.Consolidations) != 0 ||
		len(reqs.BuilderDeposits) != 0 || len(reqs.BuilderExits) != 0 ||
		len(reqs.ValidatorPreregistrations) != 0 || len(reqs.BuilderPreregistrations) != 0 || len(reqs.VoluntaryExits) != 0 {
		if !isParentFull {
			return errors.New("parent was empty but parent_execution_requests is non-empty")
		}
		return errors.New("execution requests are not supported on PQ yet")
	}
	if !isParentFull {
		return nil
	}
	root, err := reqs.HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute parent execution requests root")
	}
	if root != bidRequestsRoot {
		return errors.Errorf("parent execution requests root mismatch: block=%#x, bid=%#x", root, bidRequestsRoot)
	}
	return ApplyParentExecutionPayload(ctx, st, &enginev1.ExecutionRequestsGloas{})
}
