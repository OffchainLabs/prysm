package sync

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/container/slice"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func (s *Service) committeeIndexBeaconAttestationSubscriber(_ context.Context, msg proto.Message) error {
	a, ok := msg.(eth.Att)
	if !ok {
		return fmt.Errorf("message was not type eth.Att, type=%T", msg)
	}

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

func persistentSubnetIndices() []uint64 {
	return cache.SubnetIDs.GetAllSubnets()
}

func aggregatorSubnetIndices(currentSlot primitives.Slot) []uint64 {
	endEpoch := slots.ToEpoch(currentSlot) + 1
	endSlot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(endEpoch))
	var commIds []uint64
	for i := currentSlot; i <= endSlot; i++ {
		commIds = append(commIds, cache.SubnetIDs.GetAggregatorSubnetIDs(i)...)
	}
	return slice.SetUint64(commIds)
}

func attesterSubnetIndices(currentSlot primitives.Slot) map[uint64]bool {
	endEpoch := slots.ToEpoch(currentSlot) + 1
	endSlot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(endEpoch))

	subnets := make(map[uint64]bool, int(endSlot-currentSlot+1))
	for i := currentSlot; i <= endSlot; i++ {
		for _, subnetId := range cache.SubnetIDs.GetAttesterSubnetIDs(i) {
			subnets[subnetId] = true
		}
	}

	return subnets
}
