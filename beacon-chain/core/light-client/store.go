package light_client

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var ErrLightClientBootstrapNotFound = errors.New("light client bootstrap not found")

type Store struct {
	mu sync.RWMutex

	beaconDB             iface.HeadAccessDatabase
	lastFinalityUpdate   interfaces.LightClientFinalityUpdate   // tracks the best finality update seen so far
	lastOptimisticUpdate interfaces.LightClientOptimisticUpdate // tracks the best optimistic update seen so far
	p2p                  p2p.Accessor
	stateFeed            event.SubscriberSender
	cache                *cache // non finality cache
}

func NewLightClientStore(ctx context.Context, p p2p.Accessor, e event.SubscriberSender, db iface.HeadAccessDatabase) (*Store, error) {
	cp, err := db.FinalizedCheckpoint(ctx)
	if err != nil {
		log.WithError(err).Fatal("Failed to get finalized checkpoint from database")
		return nil, nil
	}
	return &Store{
		beaconDB:  db,
		p2p:       p,
		stateFeed: e,
		cache:     newLightClientCache([32]byte(cp.Root)),
	}, nil
}

func (s *Store) SaveLCData(ctx context.Context,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState,
	attestedBlock interfaces.ReadOnlySignedBeaconBlock,
	finalizedBlock interfaces.ReadOnlySignedBeaconBlock,
	headBlockRoot [32]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// compute required data
	update, err := NewLightClientUpdateFromBeaconState(ctx, state, block, attestedState, attestedBlock, finalizedBlock)
	if err != nil {
		return errors.Wrapf(err, "failed to create light client update")
	}
	finalityUpdate, err := NewLightClientFinalityUpdateFromBeaconState(ctx, state, block, attestedState, attestedBlock, finalizedBlock)
	if err != nil {
		return errors.Wrapf(err, "failed to create light client finality update")
	}
	optimisticUpdate, err := NewLightClientOptimisticUpdateFromBeaconState(ctx, state, block, attestedState, attestedBlock)
	if err != nil {
		return errors.Wrapf(err, "failed to create light client optimistic update")
	}
	period := slots.SyncCommitteePeriod(slots.ToEpoch(update.AttestedHeader().Beacon().Slot))
	blockRoot, err := attestedBlock.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrapf(err, "failed to compute attested block root")
	}
	parentRoot := [32]byte(update.AttestedHeader().Beacon().ParentRoot)
	signatureBlockRoot, err := block.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrapf(err, "failed to compute signature block root")
	}

	newBlockIsHead := signatureBlockRoot == headBlockRoot

	// create the new cache item
	newCacheItem := &cacheItem{
		period: period,
		slot:   block.Block().Slot(),
	}

	// check if parent exists in cache
	parentItem, ok := s.cache.items[parentRoot]
	if !ok {
		// if not, create an item for the parent, but don't need to save it.
		bestUpdateSoFar, err := s.beaconDB.LightClientUpdate(ctx, period)
		if err != nil {
			return errors.Wrapf(err, "could not get best light client update for period %d", period)
		}
		parentItem = &cacheItem{
			period:             period,
			bestUpdate:         &bestUpdateSoFar,
			bestFinalityUpdate: &s.lastFinalityUpdate,
		}
	} else {
		newCacheItem.parent = parentItem
	}

	// if at a period boundary, no need to compare data, just save new ones
	if parentItem.period != period {
		newCacheItem.bestUpdate = &update
		newCacheItem.bestFinalityUpdate = &finalityUpdate
		s.cache.items[blockRoot] = newCacheItem

		s.setLastOptimisticUpdate(optimisticUpdate, true)

		// if the new block is not head, we don't want to change our lastFinalityUpdate
		if newBlockIsHead {
			s.setLastFinalityUpdate(finalityUpdate, true)
		}

		return nil
	}

	// if in the same period, compare updates
	isUpdateBetter, err := IsBetterUpdate(update, *parentItem.bestUpdate)
	if err != nil {
		return errors.Wrapf(err, "could not compare light client updates")
	}
	if isUpdateBetter {
		newCacheItem.bestUpdate = &update
	} else {
		newCacheItem.bestUpdate = parentItem.bestUpdate
	}

	var finalityUpdateChanged bool
	isBetterFinalityUpdate := IsBetterFinalityUpdate(finalityUpdate, *parentItem.bestFinalityUpdate)
	if isBetterFinalityUpdate {
		finalityUpdateChanged = true
		newCacheItem.bestFinalityUpdate = &finalityUpdate
	} else {
		finalityUpdateChanged = false
		newCacheItem.bestFinalityUpdate = parentItem.bestFinalityUpdate
	}

	// save new item in cache
	s.cache.items[blockRoot] = newCacheItem

	// save lastOptimisticUpdate if better
	if isBetterOptimisticUpdate := IsBetterOptimisticUpdate(optimisticUpdate, s.lastOptimisticUpdate); isBetterOptimisticUpdate {
		s.setLastOptimisticUpdate(optimisticUpdate, true)
	}

	// if the new block is considered the head, set the last finality update
	if newBlockIsHead {
		s.setLastFinalityUpdate(*newCacheItem.bestFinalityUpdate, finalityUpdateChanged)
	}

	return nil
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

func (s *Store) LightClientUpdates(ctx context.Context, startPeriod, endPeriod uint64, headBlock interfaces.ReadOnlySignedBeaconBlock) ([]interfaces.LightClientUpdate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Fetch the light client updatesMap from the database
	updatesMap, err := s.beaconDB.LightClientUpdates(ctx, startPeriod, endPeriod)
	if err != nil {
		return nil, err
	}

	cacheUpdatesByPeriod, err := s.getCacheUpdatesByPeriod(ctx, headBlock)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get updates from cache")
	}

	for period, update := range cacheUpdatesByPeriod {
		updatesMap[period] = update
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

func (s *Store) LightClientUpdate(ctx context.Context, period uint64, headBlock interfaces.ReadOnlySignedBeaconBlock) (interfaces.LightClientUpdate, error) {
	// we don't need to lock here because the LightClientUpdates method locks the store
	updates, err := s.LightClientUpdates(ctx, period, period, headBlock)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get light client update for period %d", period)
	}
	if len(updates) == 0 {
		return nil, nil
	}
	return updates[0], nil
}

func (s *Store) getCacheUpdatesByPeriod(ctx context.Context, headBlock interfaces.ReadOnlySignedBeaconBlock) (map[uint64]interfaces.LightClientUpdate, error) {
	updatesByPeriod := make(map[uint64]interfaces.LightClientUpdate)

	headRoot := headBlock.Block().ParentRoot()

	var headItem *cacheItem
	var ok bool

	for {
		if headRoot == s.cache.tail || headRoot == [32]byte{} {
			return updatesByPeriod, nil
		}

		headItem, ok = s.cache.items[headRoot]
		if ok {
			break
		} else {
			blk, err := s.beaconDB.Block(ctx, headRoot)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to fetch block for root %x", headRoot)
			}
			if blk == nil {
				return nil, errors.Errorf("failed to fetch block for root %x", headRoot)
			}
			headRoot = blk.Block().ParentRoot()
		}
	}

	for headItem != nil {
		if _, exists := updatesByPeriod[headItem.period]; !exists {
			updatesByPeriod[headItem.period] = *headItem.bestUpdate
		}
		headItem = headItem.parent
	}

	return updatesByPeriod, nil
}

func (s *Store) SetLastFinalityUpdate(update interfaces.LightClientFinalityUpdate, broadcast bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setLastFinalityUpdate(update, broadcast)
}

func (s *Store) setLastFinalityUpdate(update interfaces.LightClientFinalityUpdate, broadcast bool) {
	if broadcast && IsFinalityUpdateValidForBroadcast(update, s.lastFinalityUpdate) {
		if err := s.p2p.BroadcastLightClientFinalityUpdate(context.Background(), update); err != nil {
			log.WithError(err).Error("Could not broadcast light client finality update")
		}
	}

	s.lastFinalityUpdate = update
	log.Debug("Saved new light client finality update")

	s.stateFeed.Send(&feed.Event{
		Type: statefeed.LightClientFinalityUpdate,
		Data: update,
	})
}

func (s *Store) LastFinalityUpdate() interfaces.LightClientFinalityUpdate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastFinalityUpdate
}

func (s *Store) SetLastOptimisticUpdate(update interfaces.LightClientOptimisticUpdate, broadcast bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setLastOptimisticUpdate(update, broadcast)
}

func (s *Store) setLastOptimisticUpdate(update interfaces.LightClientOptimisticUpdate, broadcast bool) {
	if broadcast {
		if err := s.p2p.BroadcastLightClientOptimisticUpdate(context.Background(), update); err != nil {
			log.WithError(err).Error("Could not broadcast light client optimistic update")
		}
	}

	s.lastOptimisticUpdate = update
	log.Debug("Saved new light client optimistic update")

	s.stateFeed.Send(&feed.Event{
		Type: statefeed.LightClientOptimisticUpdate,
		Data: update,
	})
}

func (s *Store) LastOptimisticUpdate() interfaces.LightClientOptimisticUpdate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastOptimisticUpdate
}

func (s *Store) MigrateToCold(ctx context.Context, root [32]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.cache.items) == 0 {
		log.Debug("Non-finality cache is empty. Skipping migration.")
		s.cache.tail = root
		return nil
	}

	blk, err := s.beaconDB.Block(ctx, root)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch block for finalized root %x", root)
	}
	if blk == nil {
		return errors.Errorf("failed to fetch block for finalized root %x", root)
	}
	cpSlot := blk.Block().Slot()
	headRoot := blk.Block().ParentRoot()

	var headItem *cacheItem
	var ok bool

	for {
		if headRoot == s.cache.tail || headRoot == [32]byte{} {
			log.Debug("Did not find any canonical item in the non-finality cache. Skipping migration.")
			s.cache.tail = root

			// delete non-finality cache items older than checkpoint slot
			for k, v := range s.cache.items {
				if v.slot < cpSlot {
					delete(s.cache.items, k)
				}
			}

			return nil
		}

		headItem, ok = s.cache.items[headRoot]
		if ok {
			break
		} else {
			blk, err = s.beaconDB.Block(ctx, headRoot)
			if err != nil {
				return errors.Wrapf(err, "failed to fetch block for root %x", headRoot)
			}
			if blk == nil {
				return errors.Errorf("failed to fetch block for root %x", headRoot)
			}
			headRoot = blk.Block().ParentRoot()
		}
	}

	updateByPeriod := make(map[uint64]interfaces.LightClientUpdate)
	for headItem != nil {
		if _, ok := updateByPeriod[headItem.period]; ok {
			// We already have an update for this period, skip this item
			headItem = headItem.parent
			continue
		}
		updateByPeriod[headItem.period] = *headItem.bestUpdate
		headItem = headItem.parent
	}

	// save updates to db
	for period, update := range updateByPeriod {
		err = s.beaconDB.SaveLightClientUpdate(ctx, period, update)
		if err != nil {
			return errors.Wrapf(err, "failed to save light client update for period %d", period)
		}
	}

	// delete non-finality cache items
	for k, v := range s.cache.items {
		if v.slot < cpSlot {
			delete(s.cache.items, k)
		}
	}

	s.cache.tail = root

	return nil
}
