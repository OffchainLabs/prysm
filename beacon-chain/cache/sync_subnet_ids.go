package cache

import (
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/container/slice"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
)

type syncSubnetIDs struct {
	sCommittee    *cache.Cache
	sCommiteeLock sync.RWMutex
}

// SyncSubnetIDs for sync committee participant.
var SyncSubnetIDs = newSyncSubnetIDs()

func newSyncSubnetIDs() *syncSubnetIDs {
	epochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	// Set the default duration of a sync subnet index as the whole sync committee period.
	subLength := epochDuration * time.Duration(params.BeaconConfig().EpochsPerSyncCommitteePeriod)
	persistentCache := cache.New(subLength*time.Second, epochDuration*time.Second)
	return &syncSubnetIDs{sCommittee: persistentCache}
}

// GetSyncCommitteeSubnets retrieves the sync committee subnet and expiration time of that validator's subscription.
func (s *syncSubnetIDs) GetSyncCommitteeSubnets(pubkey []byte, epoch primitives.Epoch) ([]uint64, primitives.Epoch, bool, time.Time) {
	s.sCommiteeLock.RLock()
	defer s.sCommiteeLock.RUnlock()

	id, duration, ok := s.sCommittee.GetWithExpiration(keyBuilder(pubkey, epoch))
	if !ok {
		return []uint64{}, 0, ok, time.Time{}
	}
	// Retrieve indices from the cache.
	idxs, ok := id.([]uint64)
	if !ok {
		return []uint64{}, 0, ok, time.Time{}
	}
	// If no committees are saved, we return nothing.
	if len(idxs) <= 1 {
		return []uint64{}, 0, ok, time.Time{}
	}

	// Index 0 was used to store validator's join epoch. We do not
	// return it to the caller.
	// Index 1 and beyond were used to store subnets.
	return idxs[1:], primitives.Epoch(idxs[0]), ok, duration
}

// GetAllSubnets retrieves all the non-expired subscribed subnets of all the validators
// in the cache. This method also takes the epoch as an argument to only retrieve
// assignments for epochs that have happened.
func (s *syncSubnetIDs) GetAllSubnets(currEpoch primitives.Epoch) []uint64 {
	s.sCommiteeLock.RLock()
	defer s.sCommiteeLock.RUnlock()

	itemsMap := s.sCommittee.Items()
	var committees []uint64

	for _, v := range itemsMap {
		if v.Expired() {
			continue
		}
		idxs, ok := v.Object.([]uint64)
		if !ok {
			continue
		}
		// We skip if we do not have a join
		// epoch or any relevant committee indices.
		if len(idxs) <= 1 {
			continue
		}
		// Check if the subnet is valid in the current epoch. If our
		// join epoch is still in the future we skip retrieving the
		// relevant committee index.
		if primitives.Epoch(idxs[0]) > currEpoch {
			continue
		}
		// Ignore the first index as that represents the
		// epoch of the validator's assignments.
		committees = append(committees, idxs[1:]...)
	}
	return slice.SetUint64(committees)
}

// AddSyncCommitteeSubnets adds the relevant committee for that particular validator along with its
// expiration period. An Epoch argument here denotes the epoch from which the sync committee subnets
// will be active.
func (s *syncSubnetIDs) AddSyncCommitteeSubnets(pubkey []byte, epoch primitives.Epoch, comIndex []uint64, duration time.Duration) {
	s.sCommiteeLock.Lock()
	defer s.sCommiteeLock.Unlock()
	subComCount := params.BeaconConfig().SyncCommitteeSubnetCount
	// To join a sync committee subnet, select a random number of epochs before the end of the
	// current sync committee period between 1 and SYNC_COMMITTEE_SUBNET_COUNT, inclusive.
	// This is to smoothing out the join and exiting of the subnets so not everyone surging at the same time.
	diff := (rand.NewGenerator().Uint64() % subComCount) + 1
	joinEpoch, err := epoch.SafeSub(diff)
	if err != nil {
		// If we underflow here, we simply set the value to join
		// at 0.
		joinEpoch = 0
	}
	// Append the epoch of the subnet into the first index here. The join epoch is a special
	// value, it is the epoch at which a node is supposed to join the relevant subnets.
	s.sCommittee.Set(keyBuilder(pubkey, epoch), append([]uint64{uint64(joinEpoch)}, comIndex...), duration)
}

// EmptyAllCaches empties out all the related caches and flushes any stored
// entries on them. This should only ever be used for testing, in normal
// production, handling of the relevant subnets for each role is done
// separately.
func (s *syncSubnetIDs) EmptyAllCaches() {
	// Clear the cache.

	s.sCommiteeLock.Lock()
	s.sCommittee.Flush()
	s.sCommiteeLock.Unlock()
}

// build a key composed of both the pubkey and epoch here. The epoch
// here would be the 1st epoch of the sync committee period.
func keyBuilder(pubkey []byte, epoch primitives.Epoch) string {
	epochBytes := bytesutil.Bytes8(uint64(epoch))
	return string(append(pubkey, epochBytes...))
}
