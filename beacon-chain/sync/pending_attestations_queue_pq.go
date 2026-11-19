package sync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func (s *Service) processPendingAttestationPQ(ctx context.Context, att *ethpb.SingleAttestationPQ, preState state.ReadOnlyBeaconState) {
	if err := pq.VerifySingleAttestation(preState, att); err != nil {
		log.WithError(err).Debug("Pending attestation has an invalid signature")
		return
	}
	s.cfg.pqAttPool.Save(att)
	if key, err := generateUnaggregatedAttCacheKey(att); err != nil {
		log.WithError(err).Error("Failed to generate cache key for attestation tracking")
	} else {
		_ = s.setSeenUnaggregatedAtt(key)
	}
	valCount, err := helpers.ActiveValidatorCount(ctx, preState, slots.ToEpoch(att.Data.Slot))
	if err != nil {
		log.WithError(err).Debug("Failed to retrieve active validator count")
		return
	}
	if err := s.cfg.p2p.BroadcastAttestation(ctx, helpers.ComputeSubnetForAttestation(valCount, att), att); err != nil {
		log.WithError(err).Debug("Failed to broadcast attestation")
	}
}
