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
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
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

func NewLightClientStore(p p2p.Accessor, e event.SubscriberSender, db iface.HeadAccessDatabase) *Store {
	return &Store{
		beaconDB:  db,
		p2p:       p,
		stateFeed: e,
		cache:     newLightClientCache(),
	}
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
		slot:   attestedBlock.Block().Slot(),
	}

	// check if parent exists in cache
	parentItem, ok := s.cache.items[parentRoot]
	if ok {
		newCacheItem.parent = parentItem
	} else {
		// if not, create an item for the parent, but don't need to save it since it's the accumulated best update and is just used for comparison
		bestUpdateSoFar, err := s.beaconDB.LightClientUpdate(ctx, period)
		if err != nil {
			return errors.Wrapf(err, "could not get best light client update for period %d", period)
		}
		parentItem = &cacheItem{
			period:             period,
			bestUpdate:         bestUpdateSoFar,
			bestFinalityUpdate: s.lastFinalityUpdate,
		}
	}

	// if at a period boundary, no need to compare data, just save new ones
	if parentItem.period != period {
		newCacheItem.bestUpdate = update
		newCacheItem.bestFinalityUpdate = finalityUpdate
		s.cache.items[blockRoot] = newCacheItem

		s.setLastOptimisticUpdate(optimisticUpdate, true)

		// if the new block is not head, we don't want to change our lastFinalityUpdate
		if newBlockIsHead {
			s.setLastFinalityUpdate(finalityUpdate, true)
		}

		return nil
	}

	// if in the same period, compare updates
	isUpdateBetter, err := IsBetterUpdate(update, parentItem.bestUpdate)
	if err != nil {
		return errors.Wrapf(err, "could not compare light client updates")
	}
	if isUpdateBetter {
		newCacheItem.bestUpdate = update
	} else {
		newCacheItem.bestUpdate = parentItem.bestUpdate
	}

	isBetterFinalityUpdate := IsBetterFinalityUpdate(finalityUpdate, parentItem.bestFinalityUpdate)
	if isBetterFinalityUpdate {
		newCacheItem.bestFinalityUpdate = finalityUpdate
	} else {
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
		s.setLastFinalityUpdate(newCacheItem.bestFinalityUpdate, isBetterFinalityUpdate)
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

func (s *Store) SaveLightClientBootstrap(ctx context.Context, blockRoot [32]byte, state state.BeaconState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	blk, err := s.beaconDB.Block(ctx, blockRoot)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch block for root %x", blockRoot)
	}
	if blk == nil {
		return errors.Errorf("nil block for root %x", blockRoot)
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
		return nil, errors.Wrapf(err, "failed to get updates from the database")
	}

	cacheUpdatesByPeriod, err := s.getCacheUpdatesByPeriod(headBlock)
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

func (s *Store) getCacheUpdatesByPeriod(headBlock interfaces.ReadOnlySignedBeaconBlock) (map[uint64]interfaces.LightClientUpdate, error) {
	updatesByPeriod := make(map[uint64]interfaces.LightClientUpdate)

	cacheHeadRoot := headBlock.Block().ParentRoot()

	cacheHeadItem, ok := s.cache.items[cacheHeadRoot]
	if !ok {
		log.Debugf("Head root %x not found in light client cache. Returning empty updates map for non finality cache.", cacheHeadRoot)
		return updatesByPeriod, nil
	}

	for cacheHeadItem != nil {
		if _, exists := updatesByPeriod[cacheHeadItem.period]; !exists {
			updatesByPeriod[cacheHeadItem.period] = cacheHeadItem.bestUpdate
		}
		cacheHeadItem = cacheHeadItem.parent
	}

	return updatesByPeriod, nil
}

// SetLastFinalityUpdate should be used only for testing.
func (s *Store) SetLastFinalityUpdate(update interfaces.LightClientFinalityUpdate, broadcast bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setLastFinalityUpdate(update, broadcast)
}

func (s *Store) setLastFinalityUpdate(update interfaces.LightClientFinalityUpdate, broadcast bool) {
	if broadcast && IsFinalityUpdateValidForBroadcast(update, s.lastFinalityUpdate) {
		go func() {
			if err := s.p2p.BroadcastLightClientFinalityUpdate(context.Background(), update); err != nil {
				log.WithError(err).Error("Could not broadcast light client finality update")
			}
		}()
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

// SetLastOptimisticUpdate should be used only for testing.
func (s *Store) SetLastOptimisticUpdate(update interfaces.LightClientOptimisticUpdate, broadcast bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setLastOptimisticUpdate(update, broadcast)
}

func (s *Store) setLastOptimisticUpdate(update interfaces.LightClientOptimisticUpdate, broadcast bool) {
	if broadcast {
		go func() {
			if err := s.p2p.BroadcastLightClientOptimisticUpdate(context.Background(), update); err != nil {
				log.WithError(err).Error("Could not broadcast light client optimistic update")
			}
		}()
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

func (s *Store) MigrateToCold(ctx context.Context, finalizedRoot [32]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If there cache is empty (some problem in processing data), we can skip migration.
	// This is a safety check and should not happen in normal operation.
	if len(s.cache.items) == 0 {
		log.Debug("Non-finality cache is empty. Skipping migration.")
		return nil
	}

	blk, err := s.beaconDB.Block(ctx, finalizedRoot)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch block for finalized root %x", finalizedRoot)
	}
	if blk == nil {
		return errors.Errorf("nil block for finalized root %x", finalizedRoot)
	}
	finalizedSlot := blk.Block().Slot()
	finalizedCacheHeadRoot := blk.Block().ParentRoot()

	var finalizedCacheHead *cacheItem
	var ok bool

	finalizedCacheHead, ok = s.cache.items[finalizedCacheHeadRoot]
	if !ok {
		log.Debugf("Finalized block's parent root %x not found in light client cache. Cleaning the broken part of the cache.", finalizedCacheHeadRoot)

		// delete non-finality cache items older than finalized slot
		s.cleanCache(finalizedSlot)

		return nil
	}

	updateByPeriod := make(map[uint64]interfaces.LightClientUpdate)
	// Traverse the cache from the head item to the tail, collecting updates
	for item := finalizedCacheHead; item != nil; item = item.parent {
		if _, seen := updateByPeriod[item.period]; seen {
			// We already have an update for this period, skip this item
			continue
		}
		updateByPeriod[item.period] = item.bestUpdate
	}

	// save updates to db
	for period, update := range updateByPeriod {
		err = s.beaconDB.SaveLightClientUpdate(ctx, period, update)
		if err != nil {
			log.WithError(err).Errorf("failed to save light client update for period %d. Skipping this period.", period)
		}
	}

	// delete non-finality cache items older than finalized slot
	s.cleanCache(finalizedSlot)

	return nil
}

func (s *Store) cleanCache(finalizedSlot primitives.Slot) {
	// delete non-finality cache items older than finalized slot
	for k, v := range s.cache.items {
		if v.slot < finalizedSlot {
			delete(s.cache.items, k)
		}
		if v.parent != nil && v.parent.slot < finalizedSlot {
			v.parent = nil // remove parent reference
		}
	}
}
