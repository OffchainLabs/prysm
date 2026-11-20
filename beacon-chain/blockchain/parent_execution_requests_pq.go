package blockchain

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// PQ envelopes keep their requests in the PQ record, the shared record has none.
func (s *Service) parentExecutionRequests(ctx context.Context, st state.ReadOnlyBeaconState, parentRoot [32]byte) (*enginev1.ExecutionRequestsGloas, error) {
	if st.Version() < version.PQ {
		envelope, err := s.cfg.BeaconDB.ExecutionPayloadEnvelope(ctx, parentRoot)
		if err != nil {
			return nil, err
		}
		return envelope.Message.ExecutionRequests, nil
	}
	envelope, err := s.cfg.BeaconDB.ExecutionPayloadEnvelopePQ(ctx, parentRoot)
	if err != nil {
		return nil, err
	}
	reqs := envelope.Message.ExecutionRequests
	return &enginev1.ExecutionRequestsGloas{
		Deposits:        reqs.GetDeposits(),
		Withdrawals:     reqs.GetWithdrawals(),
		Consolidations:  reqs.GetConsolidations(),
		BuilderDeposits: reqs.GetBuilderDeposits(),
		BuilderExits:    reqs.GetBuilderExits(),
	}, nil
}
