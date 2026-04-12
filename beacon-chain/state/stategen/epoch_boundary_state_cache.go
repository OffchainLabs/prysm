package stategen

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"k8s.io/client-go/tools/cache"
)

var (
	// maxCacheSize is 8. That means 8 epochs and roughly an hour
	// of no finality can be endured.
	maxCacheSize        = uint64(8)
	errNotSlotRootInfo  = errors.New("not slot root info type")
	errNotRootStateInfo = errors.New("not root state info type")
)

// slotRootInfo specifies the slot root info in the epoch boundary state cache.
type slotRootInfo struct {
	slot      primitives.Slot
	blockRoot [32]byte
}

// slotKeyFn takes the string representation of the slot to be used as key
// to retrieve root.
func slotKeyFn(obj any) (string, error) {
	s, ok := obj.(*slotRootInfo)
	if !ok {
		return "", errNotSlotRootInfo
	}
	return slotToString(s.slot), nil
}

// rootStateInfo specifies the root state info in the epoch boundary state cache.
type rootStateInfo struct {
	root  [32]byte
	state state.BeaconState
}

// rootKeyFn takes the string representation of the block root to be used as key
// to retrieve epoch boundary state.
func rootKeyFn(obj any) (string, error) {
	s, ok := obj.(*rootStateInfo)
	if !ok {
		return "", errNotRootStateInfo
	}
	return string(s.root[:]), nil
}

// epochBoundaryState struct with two queues by looking up beacon state by slot or root.
type epochBoundaryState struct {
	rootStateCache *cache.FIFO
	slotRootCache  *cache.FIFO
	lock           sync.RWMutex
}

// newBoundaryStateCache creates a new block newBoundaryStateCache for storing and accessing epoch boundary states from
// memory.
func newBoundaryStateCache() *epochBoundaryState {
	return &epochBoundaryState{
		rootStateCache: cache.NewFIFO(rootKeyFn),
		slotRootCache:  cache.NewFIFO(slotKeyFn),
	}
}

// ByBlockRoot satisfies the CachedGetter interface
func (e *epochBoundaryState) ByBlockRoot(r [32]byte) (state.BeaconState, error) {
	rsi, ok, err := e.getByBlockRoot(r)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotInCache
	}
	return rsi.state, nil
}

// get epoch boundary state by its block root. Returns copied state in state info object if exists. Otherwise returns nil.
func (e *epochBoundaryState) getByBlockRoot(r [32]byte) (*rootStateInfo, bool, error) {
	e.lock.RLock()
	defer e.lock.RUnlock()

	return e.getByBlockRootLockFree(r)
}

func (e *epochBoundaryState) getByBlockRootLockFree(r [32]byte) (*rootStateInfo, bool, error) {
	obj, exists, err := e.rootStateCache.GetByKey(string(r[:]))
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	s, ok := obj.(*rootStateInfo)
	if !ok {
		return nil, false, errNotRootStateInfo
	}

	return &rootStateInfo{
		root:  r,
		state: s.state.Copy(),
	}, true, nil
}

// get epoch boundary state by its slot. Returns copied state in state info object if exists. Otherwise returns nil.
func (e *epochBoundaryState) getBySlot(s primitives.Slot) (*rootStateInfo, bool, error) {
	e.lock.RLock()
	defer e.lock.RUnlock()

	obj, exists, err := e.slotRootCache.GetByKey(slotToString(s))
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	info, ok := obj.(*slotRootInfo)
	if !ok {
		return nil, false, errNotSlotRootInfo
	}

	return e.getByBlockRootLockFree(info.blockRoot)
}

// put adds a state to the epoch boundary state cache. This method also trims the
// least recently added state info if the cache size has reached the max cache
// size limit.
func (e *epochBoundaryState) put(blockRoot [32]byte, s state.ReadOnlyBeaconState) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	if err := e.slotRootCache.AddIfNotPresent(&slotRootInfo{
		slot:      s.Slot(),
		blockRoot: blockRoot,
	}); err != nil {
		return err
	}
	if err := e.rootStateCache.AddIfNotPresent(&rootStateInfo{
		root:  blockRoot,
		state: s.Copy(),
	}); err != nil {
		return err
	}

	trim(e.rootStateCache, maxCacheSize)
	trim(e.slotRootCache, maxCacheSize)

	epochBoundaryCacheSize.Set(float64(len(e.rootStateCache.ListKeys())))

	return nil
}

// getByBlockRootNoCopy returns the state for the given block root without copying.
func (e *epochBoundaryState) getByBlockRootNoCopy(r [32]byte) state.ReadOnlyBeaconState {
	e.lock.RLock()
	defer e.lock.RUnlock()

	obj, exists, err := e.rootStateCache.GetByKey(string(r[:]))
	if err != nil || !exists {
		return nil
	}

	s, ok := obj.(*rootStateInfo)
	if !ok {
		return nil
	}

	return s.state
}

// evictOlderThan removes all entries whose slot is strictly less than the given slot.
func (e *epochBoundaryState) evictOlderThan(slot primitives.Slot) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	for _, key := range e.slotRootCache.ListKeys() {
		obj, exists, err := e.slotRootCache.GetByKey(key)
		if err != nil {
			return fmt.Errorf("slot root cache get by key: %w", err)
		}
		if !exists {
			continue
		}

		info, ok := obj.(*slotRootInfo)
		if !ok {
			continue
		}

		if info.slot < slot {
			if err := e.slotRootCache.Delete(info); err != nil {
				return fmt.Errorf("slot root cache delete: %w", err)
			}

			if err := e.rootStateCache.Delete(&rootStateInfo{root: info.blockRoot}); err != nil {
				return fmt.Errorf("root state cache delete: %w", err)
			}
		}
	}

	epochBoundaryCacheSize.Set(float64(len(e.rootStateCache.ListKeys())))

	return nil
}

// delete the state from the epoch boundary state cache.
func (e *epochBoundaryState) delete(blockRoot [32]byte) error {
	e.lock.Lock()
	defer e.lock.Unlock()
	rInfo, ok, err := e.getByBlockRootLockFree(blockRoot)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	slotInfo := &slotRootInfo{
		slot:      rInfo.state.Slot(),
		blockRoot: blockRoot,
	}
	if err = e.slotRootCache.Delete(slotInfo); err != nil {
		return err
	}
	if err = e.rootStateCache.Delete(&rootStateInfo{root: blockRoot}); err != nil {
		return err
	}

	epochBoundaryCacheSize.Set(float64(len(e.rootStateCache.ListKeys())))

	return nil
}

// trim the FIFO queue to the maxSize.
func trim(queue *cache.FIFO, maxSize uint64) {
	for s := uint64(len(queue.ListKeys())); s > maxSize; s-- {
		if _, err := queue.Pop(popProcessNoopFunc); err != nil { // This never returns an error, but we'll handle anyway for sanity.
			panic(err) // lint:nopanic -- Never returns an error.
		}
	}
}

// popProcessNoopFunc is a no-op function that never returns an error.
func popProcessNoopFunc(_ any, _ bool) error {
	return nil
}

// Converts input uint64 to string. To be used as key for slot to get root.
func slotToString(s primitives.Slot) string {
	return strconv.FormatUint(uint64(s), 10)
}
