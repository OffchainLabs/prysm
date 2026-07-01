package gloas

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// ProcessDepositRequests queues each deposit request as a pending deposit.
//
// [Modified in Gloas:EIP8282] Builder onboarding no longer happens through the
// validator deposit path; builders are created and topped up only via
// BuilderDepositRequest. Deposit requests are queued like in Electra.
func ProcessDepositRequests(ctx context.Context, beaconState state.BeaconState, requests []*enginev1.DepositRequest) error {
	if len(requests) == 0 {
		return nil
	}

	for _, request := range requests {
		if err := processDepositRequest(beaconState, request); err != nil {
			return errors.Wrap(err, "could not apply deposit request")
		}
	}
	return nil
}

func processDepositRequest(beaconState state.BeaconState, request *enginev1.DepositRequest) error {
	if request == nil {
		return errors.New("nil deposit request")
	}

	if err := beaconState.AppendPendingDeposit(&ethpb.PendingDeposit{
		PublicKey:             request.Pubkey,
		WithdrawalCredentials: request.WithdrawalCredentials,
		Amount:                request.Amount,
		Signature:             request.Signature,
		Slot:                  beaconState.Slot(),
	}); err != nil {
		return errors.Wrap(err, "could not append deposit request")
	}
	return nil
}
