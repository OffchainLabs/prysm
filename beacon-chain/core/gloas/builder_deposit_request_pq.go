package gloas

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/pkg/errors"
)

// Spec: process_builder_deposit_request. Deposits carry no signature on PQ, so nothing is verified.
func processBuilderDepositRequestsPQ(ctx context.Context, st state.BeaconState, requests []*enginev1.BuilderDepositRequest) error {
	for _, request := range requests {
		if request == nil || !helpers.IsBuilderWithdrawalCredential(request.WithdrawalCredentials) {
			continue
		}
		pubkey := helpers.DepositPubkeyPQ(request.Pubkey)
		_, known := st.BuilderIndexByPubkey(pubkey)
		padded := &enginev1.BuilderDepositRequest{
			Pubkey:                pubkey[:],
			WithdrawalCredentials: request.WithdrawalCredentials,
			Amount:                request.Amount,
			Signature:             request.Signature,
		}
		if err := processBuilderDepositRequest(ctx, st, padded, sigKnownValid); err != nil {
			return errors.Wrap(err, "could not process builder deposit request")
		}
		// A legacy key can never sign, so its builder exits at once and the sweep refunds it.
		if !known && helpers.IsLegacyPubkey(request.Pubkey) {
			index, ok := st.BuilderIndexByPubkey(pubkey)
			if !ok {
				return errors.New("new builder is missing from the registry")
			}
			if err := InitiateBuilderExit(st, index); err != nil {
				return errors.Wrap(err, "could not exit legacy builder")
			}
		}
	}
	return nil
}
