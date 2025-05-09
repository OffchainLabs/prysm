// Package stategen defines functions to regenerate beacon chain states
// by replaying blocks from a stored state checkpoint, useful for
// optimization and reducing a beacon node's resource consumption.
package stategen

import (
	"context"
	stderrors "errors"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync/backfill/coverage"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

var defaultHotStateDBInterval primitives.Slot = 128

var populatePubkeyCacheOnce sync.Once

// StateManager represents a management object that handles the internal
// logic of maintaining both hot and cold states in DB.
type StateManager interface {
	Resume(ctx context.Context, fState state.BeaconState) (state.BeaconState, error)
	DisableSaveHotStateToDB(ctx context.Context) error
	EnableSaveHotStateToDB(_ context.Context)
	HasState(ctx context.Context, blockRoot [32]byte) (bool, error)
	DeleteStateFromCaches(ctx context.Context, blockRoot [32]byte) error
	ForceCheckpoint(ctx context.Context, root []byte) error
	SaveState(ctx context.Context, blockRoot [32]byte, st state.BeaconState) error
	SaveFinalizedState(fSlot primitives.Slot, fRoot [32]byte, fState state.BeaconState)
	MigrateToCold(ctx context.Context, fRoot [32]byte) error
	StateByRoot(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error)
	ActiveNonSlashedBalancesByRoot(context.Context, [32]byte) ([]uint64, error)
	StateByRootIfCachedNoCopy(blockRoot [32]byte) state.BeaconState
	StateByRootInitialSync(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error)
}

// State is a concrete implementation of StateManager.
type State struct {
	beaconDB                db.NoHeadAccessDatabase
	slotsPerArchivedPoint   primitives.Slot
	hotStateCache           *hotStateCache
	finalizedInfo           *finalizedInfo
	epochBoundaryStateCache *epochBoundaryState
	saveHotStateDB          *saveHotStateDbConfig
	avb                     coverage.AvailableBlocker
	migrationLock           *sync.Mutex
	fc                      forkchoice.ForkChoicer
}

// This tracks the config in the event of long non-finality,
// how often does the node save hot states to db? what are
// the saved hot states in db?... etc
type saveHotStateDbConfig struct {
	enabled                 bool
	lock                    sync.Mutex
	duration                primitives.Slot
	blockRootsOfSavedStates [][32]byte
}

// This tracks the finalized point. It's also the point where slot and the block root of
// cold and hot sections of the DB splits.
type finalizedInfo struct {
	slot  primitives.Slot
	root  [32]byte
	state state.BeaconState
	lock  sync.RWMutex
}

// Option is a functional option for controlling the initialization of a *State value
type Option func(*State)

// WithAvailableBlocker gives stategen an AvailableBlocker, which is used to determine if a given
// block is available. This is necessary because backfill creates a hole in the block history.
func WithAvailableBlocker(avb coverage.AvailableBlocker) Option {
	return func(sg *State) {
		sg.avb = avb
	}
}

// New returns a new state management object.
func New(beaconDB db.NoHeadAccessDatabase, fc forkchoice.ForkChoicer, opts ...Option) *State {
	s := &State{
		beaconDB:                beaconDB,
		hotStateCache:           newHotStateCache(),
		finalizedInfo:           &finalizedInfo{slot: 0, root: params.BeaconConfig().ZeroHash},
		slotsPerArchivedPoint:   params.BeaconConfig().SlotsPerArchivedPoint,
		epochBoundaryStateCache: newBoundaryStateCache(),
		saveHotStateDB: &saveHotStateDbConfig{
			duration: defaultHotStateDBInterval,
		},
		migrationLock: new(sync.Mutex),
		fc:            fc,
	}
	for _, o := range opts {
		o(s)
	}
	fc.Lock()
	defer fc.Unlock()
	fc.SetBalancesByRooter(s.ActiveNonSlashedBalancesByRoot)
	return s
}

// Resume resumes a new state management object from previously saved finalized checkpoint in DB.
func (s *State) Resume(ctx context.Context, fState state.BeaconState) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "stateGen.Resume")
	defer span.End()

	c, err := s.beaconDB.FinalizedCheckpoint(ctx)
	if err != nil {
		return nil, err
	}
	fRoot := bytesutil.ToBytes32(c.Root)
	st := fState
	// Resume as genesis state if last finalized root is zero hashes.
	if fRoot == params.BeaconConfig().ZeroHash {
		st, err = s.beaconDB.GenesisState(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not get genesis state")
		}
		// Save genesis state in the hot state cache.
		gbr, err := s.beaconDB.GenesisBlockRoot(ctx)
		if err != nil {
			return nil, stderrors.Join(ErrNoGenesisBlock, err)
		}
		fRoot = gbr
		if err := s.SaveState(ctx, gbr, st); err != nil {
			return nil, errors.Wrap(err, "could not save genesis state")
		}
	}

	if st == nil || st.IsNil() {
		return nil, errors.New("finalized state is nil")
	}

	go func() {
		if err := s.beaconDB.CleanUpDirtyStates(ctx, s.slotsPerArchivedPoint); err != nil {
			log.WithError(err).Error("Could not clean up dirty states")
		}
	}()

	s.finalizedInfo = &finalizedInfo{slot: st.Slot(), root: fRoot, state: st.Copy()}
	populatePubkeyCache(ctx, st)
	return st, nil
}

func populatePubkeyCache(ctx context.Context, st state.BeaconState) {
	epoch := slots.ToEpoch(st.Slot())
	go populatePubkeyCacheOnce.Do(func() {
		log.Debug("Populating pubkey cache")
		start := time.Now()
		if err := st.ReadFromEveryValidator(func(_ int, val state.ReadOnlyValidator) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Do not cache for non-active validators.
			if !helpers.IsActiveValidatorUsingTrie(val, epoch) {
				return nil
			}
			pub := val.PublicKey()
			_, err := bls.PublicKeyFromBytes(pub[:])
			return err
		}); err != nil {
			log.WithError(err).Error("Failed to populate pubkey cache")
		}
		log.WithField("duration", time.Since(start)).Debug("Done populating pubkey cache")
	})
}

// SaveFinalizedState saves the finalized slot, root and state into memory to be used by state gen service.
// This used for migration at the correct start slot and used for hot state play back to ensure
// lower bound to start is always at the last finalized state.
func (s *State) SaveFinalizedState(fSlot primitives.Slot, fRoot [32]byte, fState state.BeaconState) {
	s.finalizedInfo.lock.Lock()
	defer s.finalizedInfo.lock.Unlock()
	s.finalizedInfo.root = fRoot
	s.finalizedInfo.state = fState.Copy()
	s.finalizedInfo.slot = fSlot
}

// Returns true if input root equals to cached finalized root.
func (s *State) isFinalizedRoot(r [32]byte) bool {
	s.finalizedInfo.lock.RLock()
	defer s.finalizedInfo.lock.RUnlock()
	return r == s.finalizedInfo.root
}

// Returns the cached and copied finalized state.
func (s *State) FinalizedState() state.BeaconState {
	s.finalizedInfo.lock.RLock()
	defer s.finalizedInfo.lock.RUnlock()
	return s.finalizedInfo.state.Copy()
}
