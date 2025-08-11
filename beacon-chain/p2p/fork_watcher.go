package p2p

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

// A background routine which listens for new and upcoming forks and
// updates the node's discovery service to reflect any new fork version
// changes.
func (s *Service) forkWatcher() {
	// Exit early if discovery is disabled - there's no ENR to update
	if s.dv5Listener == nil {
		log.Debug("Discovery disabled, exiting fork watcher")
		return
	}

	slotTicker := slots.NewSlotTicker(s.genesisTime, params.BeaconConfig().SecondsPerSlot)
	var scheduleEntry params.NetworkScheduleEntry
	for {
		select {
		case currSlot := <-slotTicker.C():
			currentEpoch := slots.ToEpoch(currSlot)
			newEntry := params.GetNetworkScheduleEntry(currentEpoch)
			if newEntry.ForkDigest != scheduleEntry.ForkDigest {
				nextEntry := params.NextNetworkScheduleEntry(currentEpoch)
				if err := updateENR(s.dv5Listener.LocalNode(), newEntry, nextEntry); err != nil {
					log.WithFields(newEntry.LogFields()).WithError(err).Error("Could not add fork entry")
					continue // don't replace scheduleEntry until this succeeds
				}
				scheduleEntry = newEntry
			}
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting goroutine")
			slotTicker.Done()
			return
		}
	}
}
