package light_client

import (
	"sync"

	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
)

type Store struct {
	sync.RWMutex

	lastFinalityUpdate   interfaces.LightClientFinalityUpdate
	lastOptimisticUpdate interfaces.LightClientOptimisticUpdate
}

func (s *Store) SetLastFinalityUpdate(update interfaces.LightClientFinalityUpdate) {
	s.Lock()
	defer s.Unlock()
	s.lastFinalityUpdate = update
}

func (s *Store) LastFinalityUpdate() interfaces.LightClientFinalityUpdate {
	s.RLock()
	defer s.RUnlock()
	return s.lastFinalityUpdate
}

func (s *Store) SetLastOptimisticUpdate(update interfaces.LightClientOptimisticUpdate) {
	s.Lock()
	defer s.Unlock()
	s.lastOptimisticUpdate = update
}

func (s *Store) LastOptimisticUpdate() interfaces.LightClientOptimisticUpdate {
	s.RLock()
	defer s.RUnlock()
	return s.lastOptimisticUpdate
}
