package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/container/slice"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

var (
	attestationTracker   = make(map[primitives.Slot]*slotAttestationTracker)
	attestationTrackerMu sync.RWMutex
)

type slotAttestationTracker struct {
	count            uint64
	startTime        time.Time
	loggedThresholds map[int]bool
	mu               sync.RWMutex
}

func (s *Service) committeeIndexBeaconAttestationSubscriber(_ context.Context, msg proto.Message) error {
	a, ok := msg.(eth.Att)
	if !ok {
		return fmt.Errorf("message was not type eth.Att, type=%T", msg)
	}

	currentSlot := s.cfg.clock.CurrentSlot()
	attSlot := a.GetData().GetSlot()

	if attSlot == currentSlot {
		s.trackAttestationArrival(attSlot)
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

func (s *Service) trackAttestationArrival(slot primitives.Slot) {
	attestationTrackerMu.Lock()
	tracker, exists := attestationTracker[slot]
	if !exists {
		slotStartTime, err := slots.ToTime(uint64(s.cfg.clock.GenesisTime().Unix()), slot)
		if err != nil {
			attestationTrackerMu.Unlock()
			return
		}
		tracker = &slotAttestationTracker{
			startTime:        slotStartTime,
			loggedThresholds: make(map[int]bool),
		}
		attestationTracker[slot] = tracker
	}
	attestationTrackerMu.Unlock()

	tracker.mu.Lock()
	tracker.count++
	currentCount := tracker.count
	sinceStart := time.Since(tracker.startTime)
	tracker.mu.Unlock()

	expectedAttestations := 1083289 / 32
	percentage := float64(currentCount) / float64(expectedAttestations) * 100
	thresholds := []int{40, 50, 66, 80, 90, 98}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	for _, threshold := range thresholds {
		if percentage >= float64(threshold) && !tracker.loggedThresholds[threshold] {
			log.WithFields(logrus.Fields{
				"slotStartTime":      tracker.startTime.Unix(),
				"slot":               slot,
				"count":              currentCount,
				"expected":           expectedAttestations,
				"percentage":         fmt.Sprintf("%.1f%%", percentage),
				"sinceSlotStartTime": sinceStart,
				"sinceAttCutoffTime": sinceStart - 4*time.Second,
			}).Info("Attestation arrival threshold reached")
			tracker.loggedThresholds[threshold] = true
		}
	}

	s.cleanupOldTrackers(slot)
}

func (s *Service) cleanupOldTrackers(currentSlot primitives.Slot) {
	attestationTrackerMu.Lock()
	defer attestationTrackerMu.Unlock()

	for slot := range attestationTracker {
		if slot < currentSlot-2 {
			delete(attestationTracker, slot)
		}
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
