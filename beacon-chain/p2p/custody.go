package p2p

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var _ CustodyManager = (*Service)(nil)

// EarliestAvailableSlot returns the earliest available slot.
func (s *Service) EarliestAvailableSlot() (primitives.Slot, error) {
	s.custodyInfoLock.RLock()
	defer s.custodyInfoLock.RUnlock()

	if s.custodyInfo == nil {
		return 0, errors.New("no custody info available")
	}

	return s.custodyInfo.earliestAvailableSlot, nil
}

// CustodyGroupCount returns the custody group count.
func (s *Service) CustodyGroupCount() (uint64, error) {
	s.custodyInfoLock.Lock()
	defer s.custodyInfoLock.Unlock()

	if s.custodyInfo == nil {
		return 0, errors.New("no custody info available")
	}

	return s.custodyInfo.groupCount, nil
}

// UpdateCustodyInfo updates the stored custody group count to the incoming one
// if the incoming one is greater than the stored one. In this case, the
// incoming earliest available slot should be greater than or equal to the
// stored one or an error is returned.
//
//   - If there is no stored custody info, or
//   - If the incoming earliest available slot is greater than or equal to the
//     fulu fork slot and the incoming custody group count is greater than the
//     number of samples per slot
//
// then the stored earliest available slot is updated to the incoming one.
//
// This function returns a boolean indicating whether the custody info was
// updated and the (possibly updated) custody info itself.
//
// Rationale:
//   - The custody group count can only be increased (specification)
//   - If the custody group count is increased before Fulu, we can still serve
//     all the data, since there is no sharding before Fulu. As a consequence
//     we do not need to update the earliest available slot in this case.
//   - If the custody group count is increased after Fulu, but to a value less
//     than or equal to the number of samples per slot, we can still serve all
//     the data, since we store all sampled data column sidecars in all cases.
//     As a consequence, we do not need to update the earliest available slot
//   - If the custody group count is increased after Fulu to a value higher than
//     the number of samples per slot, then, until the backfill is complete, we
//     are unable to serve the data column sidecars corresponding to the new
//     custody groups. As a consequence, we need to update the earliest
//     available slot to inform the peers that we are not able to serve data
//     column sidecars before this point.
func (s *Service) UpdateCustodyInfo(earliestAvailableSlot primitives.Slot, custodyGroupCount uint64) (primitives.Slot, uint64, error) {
	samplesPerSlot := params.BeaconConfig().SamplesPerSlot

	s.custodyInfoLock.Lock()
	defer s.custodyInfoLock.Unlock()

	if s.custodyInfo == nil {
		s.custodyInfo = &custodyInfo{
			earliestAvailableSlot: earliestAvailableSlot,
			groupCount:            custodyGroupCount,
		}
		return earliestAvailableSlot, custodyGroupCount, nil
	}

	inMemory := s.custodyInfo
	if custodyGroupCount <= inMemory.groupCount {
		return inMemory.earliestAvailableSlot, inMemory.groupCount, nil
	}

	if earliestAvailableSlot < inMemory.earliestAvailableSlot {
		return 0, 0, errors.Errorf(
			"earliest available slot %d is less than the current one %d. (custody group count: %d, current one: %d)",
			earliestAvailableSlot, inMemory.earliestAvailableSlot, custodyGroupCount, inMemory.groupCount,
		)
	}

	if custodyGroupCount <= samplesPerSlot {
		inMemory.groupCount = custodyGroupCount
		return inMemory.earliestAvailableSlot, custodyGroupCount, nil
	}

	fuluForkSlot, err := fuluForkSlot()
	if err != nil {
		return 0, 0, errors.Wrap(err, "fulu fork slot")
	}

	if earliestAvailableSlot < fuluForkSlot {
		inMemory.groupCount = custodyGroupCount
		return inMemory.earliestAvailableSlot, custodyGroupCount, nil
	}

	inMemory.earliestAvailableSlot = earliestAvailableSlot
	inMemory.groupCount = custodyGroupCount
	return earliestAvailableSlot, custodyGroupCount, nil
}

// CustodyGroupCountFromPeer retrieves custody group count from a peer.
// It first tries to get the custody group count from the peer's metadata,
// then falls back to the ENR value if the metadata is not available, then
// falls back to the minimum number of custody groups an honest node should custodiy
// and serve samples from if ENR is not available.
func (s *Service) CustodyGroupCountFromPeer(pid peer.ID) uint64 {
	log := log.WithField("peerID", pid)
	// Try to get the custody group count from the peer's metadata.
	metadata, err := s.peers.Metadata(pid)
	if err != nil {
		// On error, default to the ENR value.
		log.WithError(err).Debug("Failed to retrieve metadata for peer, defaulting to the ENR value")
		return s.custodyGroupCountFromPeerENR(pid)
	}

	// If the metadata is nil, default to the ENR value.
	if metadata == nil {
		log.Debug("Metadata is nil, defaulting to the ENR value")
		return s.custodyGroupCountFromPeerENR(pid)
	}

	// Get the custody subnets count from the metadata.
	custodyCount := metadata.CustodyGroupCount()

	// If the custody count is null, default to the ENR value.
	if custodyCount == 0 {
		log.Debug("The custody count extracted from the metadata equals to 0, defaulting to the ENR value")
		return s.custodyGroupCountFromPeerENR(pid)
	}

	return custodyCount
}

// custodyGroupCountFromPeerENR retrieves the custody count from the peer's ENR.
// If the ENR is not available, it defaults to the minimum number of custody groups
// an honest node custodies and serves samples from.
func (s *Service) custodyGroupCountFromPeerENR(pid peer.ID) uint64 {
	// By default, we assume the peer custodies the minimum number of groups.
	custodyRequirement := params.BeaconConfig().CustodyRequirement

	log := log.WithFields(logrus.Fields{
		"peerID":       pid,
		"defaultValue": custodyRequirement,
	})

	// Retrieve the ENR of the peer.
	record, err := s.peers.ENR(pid)
	if err != nil {
		log.WithError(err).Debug("Failed to retrieve ENR for peer, defaulting to the default value")

		return custodyRequirement
	}

	// Retrieve the custody group count from the ENR.
	custodyGroupCount, err := peerdas.CustodyGroupCountFromRecord(record)
	if err != nil {
		log.WithError(err).Debug("Failed to retrieve custody group count from ENR for peer, defaulting to the default value")

		return custodyRequirement
	}

	return custodyGroupCount
}

func fuluForkSlot() (primitives.Slot, error) {
	beaconConfig := params.BeaconConfig()

	fuluForkEpoch := beaconConfig.FuluForkEpoch
	if fuluForkEpoch == beaconConfig.FarFutureEpoch {
		return beaconConfig.FarFutureSlot, nil
	}

	forkFuluSlot, err := slots.EpochStart(fuluForkEpoch)
	if err != nil {
		return 0, errors.Wrap(err, "epoch start")
	}

	return forkFuluSlot, nil
}
