package blockchain

import (
	"context"
	"slices"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func (s *Service) ReceivePayloadAttestationMessage(ctx context.Context, a *eth.PayloadAttestationMessage) error {
	if err := helpers.ValidateNilPayloadAttestationMessage(a); err != nil {
		return err
	}
	root := [32]byte(a.Data.BeaconBlockRoot)
	st, err := s.HeadStateReadOnly(ctx)
	if err != nil {
		return err
	}
	ptc, err := helpers.GetPayloadTimelinessCommittee(ctx, st, a.Data.Slot)
	if err != nil {
		return err
	}
	idx := slices.Index(ptc, a.ValidatorIndex)
	if idx == -1 {
		return errInvalidValidatorIndex
	}
	if s.cfg.PayloadAttestationCache.Seen(root, uint64(primitives.ValidatorIndex(idx))) {
		return nil
	}
	return s.cfg.PayloadAttestationCache.Add(a, uint64(idx))
}
