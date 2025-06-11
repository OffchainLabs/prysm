package p2p

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

// A background routine which listens for new and upcoming forks and
// updates the node's discovery service to reflect any new fork version
// changes.
func (s *Service) forkWatcher() {
	slotTicker := slots.NewSlotTicker(s.genesisTime, params.BeaconConfig().SecondsPerSlot)
	var clock *startup.Clock
	for {
		select {
		case currSlot := <-slotTicker.C():
			currEpoch := slots.ToEpoch(currSlot)
			if currEpoch == params.BeaconConfig().AltairForkEpoch ||
				currEpoch == params.BeaconConfig().BellatrixForkEpoch ||
				currEpoch == params.BeaconConfig().CapellaForkEpoch ||
				currEpoch == params.BeaconConfig().DenebForkEpoch ||
				currEpoch == params.BeaconConfig().ElectraForkEpoch ||
				currEpoch == params.BeaconConfig().FuluForkEpoch {
				// If we are in the fork epoch, we update our enr with
				// the updated fork digest. These repeatedly does
				// this over the epoch, which might be slightly wasteful
				// but is fine nonetheless.
				if s.dv5Listener != nil { // make sure it's not a local network
					if clock == nil {
						if s.clock != nil {
							clock = s.clock
						} else {
							clock = startup.NewClock(s.genesisTime, [32]byte(s.genesisValidatorsRoot))
						}
					}
					_, err := addForkEntry(s.dv5Listener.LocalNode(), clock.CurrentEpoch())
					if err != nil {
						log.WithError(err).Error("Could not add fork entry")
					}
				}
			}
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting goroutine")
			slotTicker.Done()
			return
		}
	}
}
