package p2p

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

// A background routine which listens for new and upcoming forks and
// updates the node's discovery service to reflect any new fork version
// changes.
func (s *Service) forkWatcher() {
	slotTicker := slots.NewSlotTicker(s.genesisTime, params.BeaconConfig().SecondsPerSlot)
	var scheduleEntry params.NetworkScheduleEntry
	for {
		select {
		case currSlot := <-slotTicker.C():
			newEntry := params.GetNetworkScheduleEntry(slots.ToEpoch(currSlot))
			if newEntry.ForkDigest != scheduleEntry.ForkDigest {
				nextEntry := params.GetNetworkScheduleEntry(newEntry.Epoch)
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
