package requests

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// ProcessDepositRequests processes execution layer deposits requests.
func ProcessDepositRequests(ctx context.Context, beaconState state.BeaconState, reqs []*enginev1.DepositRequest) (state.BeaconState, error) {
	_, span := trace.StartSpan(ctx, "requests.ProcessDepositRequests")
	defer span.End()

	if len(reqs) == 0 {
		return beaconState, nil
	}

	var err error
	for _, req := range reqs {
		beaconState, err = processDepositRequest(beaconState, req)
		if err != nil {
			return nil, errors.Wrap(err, "could not apply deposit request")
		}
	}
	return beaconState, nil
}

// processDepositRequest processes the specific deposit request
//
// def process_deposit_request(state: BeaconState, deposit_request: DepositRequest) -> None:
//
//	# Set deposit request start index
//	if state.deposit_requests_start_index == UNSET_DEPOSIT_REQUESTS_START_INDEX:
//	    state.deposit_requests_start_index = deposit_request.index
//
//	# Create pending deposit
//	state.pending_deposits.append(PendingDeposit(
//	    pubkey=deposit_request.pubkey,
//	    withdrawal_credentials=deposit_request.withdrawal_credentials,
//	    amount=deposit_request.amount,
//	    signature=deposit_request.signature,
//	    slot=state.slot,
//	))
func processDepositRequest(beaconState state.BeaconState, req *enginev1.DepositRequest) (state.BeaconState, error) {
	requestsStartIndex, err := beaconState.DepositRequestsStartIndex()
	if err != nil {
		return nil, errors.Wrap(err, "could not get deposit requests start index")
	}
	if req == nil {
		return nil, errors.New("nil deposit request")
	}
	if requestsStartIndex == params.BeaconConfig().UnsetDepositRequestsStartIndex {
		if err := beaconState.SetDepositRequestsStartIndex(req.Index); err != nil {
			return nil, errors.Wrap(err, "could not set deposit requests start index")
		}
	}
	if err := beaconState.AppendPendingDeposit(&ethpb.PendingDeposit{
		PublicKey:             bytesutil.SafeCopyBytes(req.Pubkey),
		WithdrawalCredentials: bytesutil.SafeCopyBytes(req.WithdrawalCredentials),
		Amount:                req.Amount,
		Signature:             bytesutil.SafeCopyBytes(req.Signature),
		Slot:                  beaconState.Slot(),
	}); err != nil {
		return nil, errors.Wrap(err, "could not append deposit request")
	}
	return beaconState, nil
}
