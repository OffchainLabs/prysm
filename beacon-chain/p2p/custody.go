package p2p

import (
	"context"

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
// It blocks until the custody info is set or the context is done.
func (s *Service) EarliestAvailableSlot(ctx context.Context) (primitives.Slot, error) {
	custodyInfo, err := s.waitForCustodyInfo(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "wait for custody info")
	}

	return custodyInfo.earliestAvailableSlot, nil
}

// CustodyGroupCount returns the custody group count.
// It blocks until the custody info is set or the context is done.
func (s *Service) CustodyGroupCount(ctx context.Context) (uint64, error) {
	custodyInfo, err := s.waitForCustodyInfo(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "wait for custody info")
	}

	return custodyInfo.groupCount, nil
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

		close(s.custodyInfoSet)

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

// UpdateEarliestAvailableSlot updates the earliest available slot.
//
// IMPORTANT: This function should only be called when Fulu is enabled. The caller is responsible
// for checking params.FuluEnabled() before calling this function.
func (s *Service) UpdateEarliestAvailableSlot(earliestAvailableSlot primitives.Slot) error {
	s.custodyInfoLock.Lock()
	defer s.custodyInfoLock.Unlock()

	if s.custodyInfo == nil {
		return errors.New("no custody info available")
	}

	currentSlot := slots.CurrentSlot(s.genesisTime)
	currentEpoch := slots.ToEpoch(currentSlot)

	// Allow decrease (for backfill scenarios)
	if earliestAvailableSlot < s.custodyInfo.earliestAvailableSlot {
		s.custodyInfo.earliestAvailableSlot = earliestAvailableSlot
		return nil
	}

	// Prevent increase within the MIN_EPOCHS_FOR_BLOCK_REQUESTS period
	// This ensures we don't voluntarily refuse to serve mandatory block data
	// This check applies regardless of whether we're early or late in the chain
	minEpochsForBlocks := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)

	// Calculate the minimum required epoch (or 0 if we're early in the chain)
	minRequiredEpoch := primitives.Epoch(0)
	if currentEpoch > minEpochsForBlocks {
		minRequiredEpoch = currentEpoch - minEpochsForBlocks
	}

	// Convert to slot to ensure we compare at slot-level granularity, not epoch-level
	// This prevents allowing increases to slots within minRequiredEpoch that are after its first slot
	minRequiredSlot, err := slots.EpochStart(minRequiredEpoch)
	if err != nil {
		return errors.Wrap(err, "epoch start")
	}

	// Prevent any increase that would put earliest slot beyond the minimum required slot
	if earliestAvailableSlot > s.custodyInfo.earliestAvailableSlot && earliestAvailableSlot > minRequiredSlot {
		return errors.Errorf(
			"cannot increase earliest available slot to %d (epoch %d) as it exceeds minimum required slot %d (epoch %d)",
			earliestAvailableSlot, slots.ToEpoch(earliestAvailableSlot), minRequiredSlot, minRequiredEpoch,
		)
	}

	s.custodyInfo.earliestAvailableSlot = earliestAvailableSlot
	return nil
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

func (s *Service) waitForCustodyInfo(ctx context.Context) (custodyInfo, error) {
	select {
	case <-s.custodyInfoSet:
		info, ok := s.copyCustodyInfo()
		if !ok {
			return custodyInfo{}, errors.New("custody info was set but is nil")
		}

		return info, nil
	case <-ctx.Done():
		return custodyInfo{}, ctx.Err()
	}
}

// copyCustodyInfo returns a copy of the current custody info in a thread-safe manner.
// If no custody info is set, it returns false as the second return value.
func (s *Service) copyCustodyInfo() (custodyInfo, bool) {
	s.custodyInfoLock.RLock()
	defer s.custodyInfoLock.RUnlock()

	if s.custodyInfo == nil {
		return custodyInfo{}, false
	}

	return *s.custodyInfo, true
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
		"agent":        agentString(pid, s.Host()),
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
