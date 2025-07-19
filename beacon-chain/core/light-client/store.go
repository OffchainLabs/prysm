package light_client

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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

func (s *Store) SaveLightClientBootstrap(ctx context.Context, blockRoot [32]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	blk, err := s.beaconDB.Block(ctx, blockRoot)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch block for root %x", blockRoot)
	}
	if blk == nil {
		return errors.Errorf("failed to fetch block for root %x", blockRoot)
	}

	state, err := s.beaconDB.State(ctx, blockRoot)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch state for block root %x", blockRoot)
	}
	if state == nil {
		return errors.Errorf("failed to fetch state for block root %x", blockRoot)
	}

	bootstrap, err := NewLightClientBootstrapFromBeaconState(ctx, state.Slot(), state, blk)
	if err != nil {
		return errors.Wrapf(err, "failed to create light client bootstrap for block root %x", blockRoot)
	}

	// Save the light client bootstrap to the database
	if err := s.beaconDB.SaveLightClientBootstrap(ctx, blockRoot[:], bootstrap); err != nil {
		return err
	}
	return nil
}

func (s *Store) LightClientUpdates(ctx context.Context, startPeriod, endPeriod uint64) ([]interfaces.LightClientUpdate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Fetch the light client updatesMap from the database
	updatesMap, err := s.beaconDB.LightClientUpdates(ctx, startPeriod, endPeriod)
	if err != nil {
		return nil, err
	}

	var updates []interfaces.LightClientUpdate
	for i := startPeriod; i <= endPeriod; i++ {
		update, ok := updatesMap[i]
		if !ok {
			// Only return the first contiguous range of updates
			break
		}
		updates = append(updates, update)
	}

	return updates, nil
}

func (s *Store) LightClientUpdate(ctx context.Context, period uint64) (interfaces.LightClientUpdate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Fetch the light client update for the given period from the database
	update, err := s.beaconDB.LightClientUpdate(ctx, period)
	if err != nil {
		return nil, err
	}

	return update, nil
}

func (s *Store) SaveLightClientUpdate(ctx context.Context, period uint64, update interfaces.LightClientUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldUpdate, err := s.beaconDB.LightClientUpdate(ctx, period)
	if err != nil {
		return errors.Wrapf(err, "could not get current light client update")
	}

	if oldUpdate == nil {
		if err := s.beaconDB.SaveLightClientUpdate(ctx, period, update); err != nil {
			return errors.Wrapf(err, "could not save light client update")
		}
		log.WithField("period", period).Debug("Saved new light client update")
		return nil
	}

	isNewUpdateBetter, err := IsBetterUpdate(update, oldUpdate)
	if err != nil {
		return errors.Wrapf(err, "could not compare light client updates")
	}

	if isNewUpdateBetter {
		if err := s.beaconDB.SaveLightClientUpdate(ctx, period, update); err != nil {
			return errors.Wrapf(err, "could not save light client update")
		}
		log.WithField("period", period).Debug("Saved new light client update")
		return nil
	}
	log.WithField("period", period).Debug("New light client update is not better than the current one, skipping save")
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
