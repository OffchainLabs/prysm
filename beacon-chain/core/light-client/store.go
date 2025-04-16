package light_client

import (
	"sync"

	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
)

type Store struct {
	mu sync.RWMutex

	lastFinalityUpdate   interfaces.LightClientFinalityUpdate
	lastOptimisticUpdate interfaces.LightClientOptimisticUpdate
}

func (s *Store) SetLastFinalityUpdate(update interfaces.LightClientFinalityUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFinalityUpdate = update
}

func (s *Store) LastFinalityUpdate() interfaces.LightClientFinalityUpdate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastFinalityUpdate
}

func (s *Store) SetLastOptimisticUpdate(update interfaces.LightClientOptimisticUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastOptimisticUpdate = update
}

func (s *Store) LastOptimisticUpdate() interfaces.LightClientOptimisticUpdate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastOptimisticUpdate
}
