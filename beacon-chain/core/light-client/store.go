package light_client

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/pkg/errors"
)

var ErrLightClientBootstrapNotFound = errors.New("light client bootstrap not found")

type Store struct {
	mu sync.RWMutex

	beaconDB             iface.HeadAccessDatabase
	lastFinalityUpdate   interfaces.LightClientFinalityUpdate
	lastOptimisticUpdate interfaces.LightClientOptimisticUpdate
}

func NewLightClientStore(db iface.HeadAccessDatabase) *Store {
	return &Store{
		beaconDB: db,
	}
}

func (s *Store) LightClientBootstrap(ctx context.Context, blockRoot [32]byte) (interfaces.LightClientBootstrap, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Fetch the light client bootstrap from the database
	bootstrap, err := s.beaconDB.LightClientBootstrap(ctx, blockRoot[:])
	if err != nil {
		return nil, err
	}
	if bootstrap == nil { // not found
		return nil, ErrLightClientBootstrapNotFound
	}

	return bootstrap, nil
}

func (s *Store) SaveLightClientBootstrap(ctx context.Context, blockRoot [32]byte, bootstrap interfaces.LightClientBootstrap) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Save the light client bootstrap to the database
	if err := s.beaconDB.SaveLightClientBootstrap(ctx, blockRoot[:], bootstrap); err != nil {
		return err
	}
	return nil
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
