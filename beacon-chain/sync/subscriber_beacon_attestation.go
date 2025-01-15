package sync

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/container/slice"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"google.golang.org/protobuf/proto"
)

func (s *Service) committeeIndexBeaconAttestationSubscriber(_ context.Context, msg proto.Message) error {
	a, ok := msg.(eth.Att)
	if !ok {
		return fmt.Errorf("message was not type eth.Att, type=%T", msg)
	}

	data := a.GetData()

	if data == nil {
		return errors.New("nil attestation")
	}
	committeeIndex, err := a.GetCommitteeIndex()
	if err != nil {
		return errors.Wrap(err, "committeeIndexBeaconAttestationSubscriber failed to get committee index")
	}
	s.setSeenCommitteeIndicesSlot(data.Slot, committeeIndex, a.GetAggregationBits())

	if features.Get().EnableExperimentalAttestationPool {
		return s.cfg.attestationCache.Add(a)
	} else {
		exists, err := s.cfg.attPool.HasAggregatedAttestation(a)
		if err != nil {
			return errors.Wrap(err, "could not determine if attestation pool has this attestation")
		}
		if exists {
			return nil
		}
		return s.cfg.attPool.SaveUnaggregatedAttestation(a)
	}
}

func (*Service) persistentSubnetIndices() []uint64 {
	return cache.SubnetIDs.GetAllSubnets()
}

func (*Service) aggregatorSubnetIndices(currentSlot primitives.Slot) []uint64 {
	endEpoch := slots.ToEpoch(currentSlot) + 1
	endSlot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(endEpoch))
	var commIds []uint64
	for i := currentSlot; i <= endSlot; i++ {
		commIds = append(commIds, cache.SubnetIDs.GetAggregatorSubnetIDs(i)...)
	}
	return slice.SetUint64(commIds)
}

func (*Service) attesterSubnetIndices(currentSlot primitives.Slot) []uint64 {
	endEpoch := slots.ToEpoch(currentSlot) + 1
	endSlot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(endEpoch))
	var commIds []uint64
	for i := currentSlot; i <= endSlot; i++ {
		commIds = append(commIds, cache.SubnetIDs.GetAttesterSubnetIDs(i)...)
	}
	return slice.SetUint64(commIds)
}
