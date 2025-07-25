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

var _ DataColumnsHandler = (*Service)(nil)

// EarliestAvailableSlot returns the earliest available slot.
func (s *Service) EarliestAvailableSlot() primitives.Slot {
	s.custodyInfoMut.RLock()
	defer s.custodyInfoMut.RUnlock()

	return s.earliestAvailableSlot
}

// CustodyGroupCount returns the custody group count.
func (s *Service) CustodyGroupCount() uint64 {
	s.custodyInfoMut.Lock()
	defer s.custodyInfoMut.Unlock()

	return s.custodyGroupCount
}

// UdpateCustodyInfo updates the custody group count and earliest available slot
// if the new custody group count is greater than the stored one.
// It returns the (potentially updated) earliest available slot and custody group count.
func (s *Service) UpdateCustodyInfo(earliestAvailableSlot primitives.Slot, custodyGroupCount uint64) (primitives.Slot, uint64, error) {
	s.custodyInfoMut.Lock()
	defer s.custodyInfoMut.Unlock()

	if custodyGroupCount <= s.custodyGroupCount {
		return s.earliestAvailableSlot, s.custodyGroupCount, nil
	}

	if earliestAvailableSlot < s.earliestAvailableSlot {
		return 0, 0, errors.Errorf(
			"earliest available slot %d is less than the current one %d. (custody group count: %d, current one: %d)",
			earliestAvailableSlot, s.earliestAvailableSlot, custodyGroupCount, s.custodyGroupCount,
		)
	}

	s.custodyGroupCount = custodyGroupCount

	fuluForkSlot, err := fuluForkSlot()
	if err != nil {
		return 0, 0, errors.Wrap(err, "fulu fork slot")
	}

	if earliestAvailableSlot >= fuluForkSlot {
		s.earliestAvailableSlot = earliestAvailableSlot
	}

	log.WithFields(logrus.Fields{
		"earliestAvailableSlot": s.earliestAvailableSlot,
		"custodyGroupCount":     s.custodyGroupCount,
	}).Debug("Custody info updated")

	return s.earliestAvailableSlot, s.custodyGroupCount, nil
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
