// Package blockchain defines the life-cycle of the blockchain at the core of
// Ethereum, including processing of new blocks and attestations using proof of stake.
package blockchain

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	coreTime "github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	f "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice"
	lightClient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Service represents a service that handles the internal
// logic of managing the full PoS beacon chain.
type Service struct {
	cfg                            *config
	ctx                            context.Context
	cancel                         context.CancelFunc
	genesisTime                    time.Time
	head                           *head
	headLock                       sync.RWMutex
	originBlockRoot                [32]byte // genesis root, or weak subjectivity checkpoint root, depending on how the node is initialized
	boundaryRoots                  [][32]byte
	checkpointStateCache           *cache.CheckpointStateCache
	initSyncBlocks                 map[[32]byte]interfaces.ReadOnlySignedBeaconBlock
	initSyncBlocksLock             sync.RWMutex
	wsVerifier                     *WeakSubjectivityVerifier
	clockSetter                    startup.ClockSetter
	clockWaiter                    startup.ClockWaiter
	syncComplete                   chan struct{}
	blobNotifiers                  *blobNotifierMap
	blockBeingSynced               *currentlySyncingBlock
	blobStorage                    *filesystem.BlobStorage
	dataColumnStorage              *filesystem.DataColumnStorage
	slasherEnabled                 bool
	lcStore                        *lightClient.Store
	startWaitingDataColumnSidecars chan bool // for testing purposes only
	syncCommitteeHeadState         *cache.SyncCommitteeHeadStateCache
}

// config options for the service.
type config struct {
	BeaconBlockBuf          int
	ChainStartFetcher       execution.ChainStartFetcher
	BeaconDB                db.HeadAccessDatabase
	DepositCache            cache.DepositCache
	PayloadIDCache          *cache.PayloadIDCache
	TrackedValidatorsCache  *cache.TrackedValidatorsCache
	AttestationCache        *cache.AttestationCache
	AttPool                 attestations.Pool
	ExitPool                voluntaryexits.PoolManager
	SlashingPool            slashings.PoolManager
	BLSToExecPool           blstoexec.PoolManager
	P2P                     p2p.Accessor
	MaxRoutines             int
	StateNotifier           statefeed.Notifier
	ForkChoiceStore         f.ForkChoicer
	AttService              *attestations.Service
	StateGen                *stategen.State
	SlasherAttestationsFeed *event.Feed
	WeakSubjectivityCheckpt *ethpb.Checkpoint
	BlockFetcher            execution.POWBlockFetcher
	FinalizedStateAtStartUp state.BeaconState
	ExecutionEngineCaller   execution.EngineCaller
	SyncChecker             Checker
}

// Checker is an interface used to determine if a node is in initial sync
// or regular sync.
type Checker interface {
	Synced() bool
}

var ErrMissingClockSetter = errors.New("blockchain Service initialized without a startup.ClockSetter")

type blobNotifierMap struct {
	sync.RWMutex
	notifiers map[[32]byte]chan uint64
	seenIndex map[[32]byte][]bool
}

// notifyIndex notifies a blob by its index for a given root.
// It uses internal maps to keep track of seen indices and notifier channels.
func (bn *blobNotifierMap) notifyIndex(root [32]byte, idx uint64, slot primitives.Slot) {
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(slot)
	if idx >= uint64(maxBlobsPerBlock) {
		return
	}

	bn.Lock()
	seen := bn.seenIndex[root]
	if seen == nil {
		seen = make([]bool, maxBlobsPerBlock)
	}
	if seen[idx] {
		bn.Unlock()
		return
	}
	seen[idx] = true
	bn.seenIndex[root] = seen

	// Retrieve or create the notifier channel for the given root.
	c, ok := bn.notifiers[root]
	if !ok {
		c = make(chan uint64, maxBlobsPerBlock)
		bn.notifiers[root] = c
	}

	bn.Unlock()

	c <- idx
}

func (bn *blobNotifierMap) forRoot(root [32]byte, slot primitives.Slot) chan uint64 {
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(slot)
	bn.Lock()
	defer bn.Unlock()
	c, ok := bn.notifiers[root]
	if !ok {
		c = make(chan uint64, maxBlobsPerBlock)
		bn.notifiers[root] = c
	}
	return c
}

func (bn *blobNotifierMap) delete(root [32]byte) {
	bn.Lock()
	defer bn.Unlock()
	delete(bn.seenIndex, root)
	delete(bn.notifiers, root)
}

// NewService instantiates a new block service instance that will
// be registered into a running beacon node.
func NewService(ctx context.Context, opts ...Option) (*Service, error) {
	var err error
	if params.DenebEnabled() {
		err = kzg.Start()
		if err != nil {
			return nil, errors.Wrap(err, "could not initialize go-kzg context")
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	bn := &blobNotifierMap{
		notifiers: make(map[[32]byte]chan uint64),
		seenIndex: make(map[[32]byte][]bool),
	}
	srv := &Service{
		ctx:                    ctx,
		cancel:                 cancel,
		boundaryRoots:          [][32]byte{},
		checkpointStateCache:   cache.NewCheckpointStateCache(),
		initSyncBlocks:         make(map[[32]byte]interfaces.ReadOnlySignedBeaconBlock),
		blobNotifiers:          bn,
		cfg:                    &config{},
		blockBeingSynced:       &currentlySyncingBlock{roots: make(map[[32]byte]struct{})},
		syncCommitteeHeadState: cache.NewSyncCommitteeHeadState(),
	}
	for _, opt := range opts {
		if err := opt(srv); err != nil {
			return nil, err
		}
	}
	if srv.clockSetter == nil {
		return nil, ErrMissingClockSetter
	}
	srv.wsVerifier, err = NewWeakSubjectivityVerifier(srv.cfg.WeakSubjectivityCheckpt, srv.cfg.BeaconDB)
	if err != nil {
		return nil, err
	}
	return srv, nil
}

// Start a blockchain service's main event loop.
func (s *Service) Start() {
	defer s.removeStartupState()
	if err := s.StartFromSavedState(s.cfg.FinalizedStateAtStartUp); err != nil {
		log.Fatal(err)
	}
	s.spawnProcessAttestationsRoutine()
	go s.runLateBlockTasks()
}

// Stop the blockchain service's main event loop and associated goroutines.
func (s *Service) Stop() error {
	defer s.cancel()

	// lock before accessing s.head, s.head.state, s.head.state.FinalizedCheckpoint().Root
	s.headLock.RLock()
	if s.cfg.StateGen != nil && s.head != nil && s.head.state != nil {
		r := s.head.state.FinalizedCheckpoint().Root
		s.headLock.RUnlock()
		// Save the last finalized state so that starting up in the following run will be much faster.
		if err := s.cfg.StateGen.ForceCheckpoint(s.ctx, r); err != nil {
			return err
		}
	} else {
		s.headLock.RUnlock()
	}
	// Save initial sync cached blocks to the DB before stop.
	return s.cfg.BeaconDB.SaveBlocks(s.ctx, s.getInitSyncBlocks())
}

// Status always returns nil unless there is an error condition that causes
// this service to be unhealthy.
func (s *Service) Status() error {
	optimistic, err := s.IsOptimistic(s.ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check if service is optimistic")
	}
	if optimistic {
		return errors.New("service is optimistic, and only limited service functionality is provided " +
			"please check if execution layer is fully synced")
	}

	if s.originBlockRoot == params.BeaconConfig().ZeroHash {
		return errors.New("genesis state has not been created")
	}
	if runtime.NumGoroutine() > s.cfg.MaxRoutines {
		return fmt.Errorf("too many goroutines (%d)", runtime.NumGoroutine())
	}
	return nil
}

// StartFromSavedState initializes the blockchain using a previously saved finalized checkpoint.
func (s *Service) StartFromSavedState(saved state.BeaconState) error {
	if state.IsNil(saved) {
		return errors.New("Last finalized state at startup is nil")
	}
	log.Info("Blockchain data already exists in DB, initializing...")
	s.genesisTime = saved.GenesisTime()
	s.cfg.AttService.SetGenesisTime(saved.GenesisTime())

	originRoot, err := s.originRootFromSavedState(s.ctx)
	if err != nil {
		return err
	}
	s.originBlockRoot = originRoot
	st, err := s.cfg.StateGen.Resume(s.ctx, s.cfg.FinalizedStateAtStartUp)
	if err != nil {
		return errors.Wrap(err, "could not get finalized state from db")
	}
	spawnCountdownIfPreGenesis(s.ctx, s.genesisTime, s.cfg.BeaconDB)
	if err := s.setupForkchoice(st); err != nil {
		return errors.Wrap(err, "could not set up forkchoice")
	}
	// not attempting to save initial sync blocks here, because there shouldn't be any until
	// after the statefeed.Initialized event is fired (below)
	cp := s.FinalizedCheckpt()
	if err := s.wsVerifier.VerifyWeakSubjectivity(s.ctx, cp.Epoch); err != nil {
		// Exit run time if the node failed to verify weak subjectivity checkpoint.
		return errors.Wrap(err, "could not verify initial checkpoint provided for chain sync")
	}

	vr := bytesutil.ToBytes32(saved.GenesisValidatorsRoot())
	if err := s.clockSetter.SetClock(startup.NewClock(s.genesisTime, vr)); err != nil {
		return errors.Wrap(err, "failed to initialize blockchain service")
	}

	if !params.FuluEnabled() {
		return nil
	}

	earliestAvailableSlot, custodySubnetCount, err := s.updateCustodyInfoInDB(saved.Slot())
	if err != nil {
		return errors.Wrap(err, "could not get and save custody group count")
	}

	if _, _, err := s.cfg.P2P.UpdateCustodyInfo(earliestAvailableSlot, custodySubnetCount); err != nil {
		return errors.Wrap(err, "update custody info")
	}

	return nil
}

func (s *Service) originRootFromSavedState(ctx context.Context) ([32]byte, error) {
	// first check if we have started from checkpoint sync and have a root
	originRoot, err := s.cfg.BeaconDB.OriginCheckpointBlockRoot(ctx)
	if err == nil {
		return originRoot, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return originRoot, errors.Wrap(err, "could not retrieve checkpoint sync chain origin data from db")
	}

	// we got here because OriginCheckpointBlockRoot gave us an ErrNotFound. this means the node was started from a genesis state,
	// so we should have a value for GenesisBlock
	genesisBlock, err := s.cfg.BeaconDB.GenesisBlock(ctx)
	if err != nil {
		return originRoot, errors.Wrap(err, "could not get genesis block from db")
	}
	if err := blocks.BeaconBlockIsNil(genesisBlock); err != nil {
		return originRoot, err
	}
	genesisBlkRoot, err := genesisBlock.Block().HashTreeRoot()
	if err != nil {
		return genesisBlkRoot, errors.Wrap(err, "could not get signing root of genesis block")
	}
	return genesisBlkRoot, nil
}

// initializeHeadFromDB uses the finalized checkpoint and head block root from forkchoice to set the current head.
// Note that this may block until stategen replays blocks between the finalized and head blocks
// if the head sync flag was specified and the gap between the finalized and head blocks is at least 128 epochs long.
func (s *Service) initializeHead(ctx context.Context, st state.BeaconState) error {
	cp := s.FinalizedCheckpt()
	fRoot := s.ensureRootNotZeros([32]byte(cp.Root))
	if st == nil || st.IsNil() {
		return errors.New("finalized state can't be nil")
	}

	s.cfg.ForkChoiceStore.RLock()
	root := s.cfg.ForkChoiceStore.HighestReceivedBlockRoot()
	s.cfg.ForkChoiceStore.RUnlock()
	blk, err := s.cfg.BeaconDB.Block(ctx, root)
	if err != nil {
		return errors.Wrap(err, "could not get head block")
	}
	if root != fRoot {
		st, err = s.cfg.StateGen.StateByRoot(ctx, root)
		if err != nil {
			return errors.Wrap(err, "could not get head state")
		}
	}
	if err := s.setHead(&head{root, blk, st, blk.Block().Slot(), false}); err != nil {
		return errors.Wrap(err, "could not set head")
	}
	log.WithFields(logrus.Fields{
		"root": fmt.Sprintf("%#x", root),
		"slot": blk.Block().Slot(),
	}).Info("Initialized head block from DB")
	return nil
}

// initializes the state and genesis block of the beacon chain to persistent storage
// based on a genesis timestamp value obtained from the ChainStart event emitted
// by the ETH1.0 Deposit Contract and the POWChain service of the node.
func (s *Service) initializeBeaconChain(
	ctx context.Context,
	genesisTime time.Time,
	preGenesisState state.BeaconState,
	eth1data *ethpb.Eth1Data) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.Service.initializeBeaconChain")
	defer span.End()
	s.genesisTime = genesisTime.Truncate(time.Second) // Genesis time has a precision of 1 second.
	unixTime := uint64(genesisTime.Unix())

	genesisState, err := transition.OptimizedGenesisBeaconState(unixTime, preGenesisState, eth1data)
	if err != nil {
		return nil, errors.Wrap(err, "could not initialize genesis state")
	}

	if err := s.saveGenesisData(ctx, genesisState); err != nil {
		return nil, errors.Wrap(err, "could not save genesis data")
	}

	log.Info("Initialized beacon chain genesis state")

	// Clear out all pre-genesis data now that the state is initialized.
	s.cfg.ChainStartFetcher.ClearPreGenesisData()

	// Update committee shuffled indices for genesis epoch.
	if err := helpers.UpdateCommitteeCache(ctx, genesisState, 0); err != nil {
		return nil, err
	}
	if err := helpers.UpdateProposerIndicesInCache(ctx, genesisState, coreTime.CurrentEpoch(genesisState)); err != nil {
		return nil, err
	}

	s.cfg.AttService.SetGenesisTime(genesisState.GenesisTime())

	return genesisState, nil
}

// This gets called when beacon chain is first initialized to save genesis data (state, block, and more) in db.
func (s *Service) saveGenesisData(ctx context.Context, genesisState state.BeaconState) error {
	if err := s.cfg.BeaconDB.SaveGenesisData(ctx, genesisState); err != nil {
		return errors.Wrap(err, "could not save genesis data")
	}
	genesisBlk, err := s.cfg.BeaconDB.GenesisBlock(ctx)
	if err != nil || genesisBlk == nil || genesisBlk.IsNil() {
		return fmt.Errorf("could not load genesis block: %w", err)
	}
	genesisBlkRoot, err := genesisBlk.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not get genesis block root")
	}

	s.originBlockRoot = genesisBlkRoot
	s.cfg.StateGen.SaveFinalizedState(0 /*slot*/, genesisBlkRoot, genesisState)

	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	gb, err := blocks.NewROBlockWithRoot(genesisBlk, genesisBlkRoot)
	if err != nil {
		return err
	}
	if err := s.cfg.ForkChoiceStore.InsertNode(ctx, genesisState, gb); err != nil {
		log.WithError(err).Fatal("Could not process genesis block for fork choice")
	}
	s.cfg.ForkChoiceStore.SetOriginRoot(genesisBlkRoot)
	// Set genesis as fully validated
	if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(ctx, genesisBlkRoot); err != nil {
		return errors.Wrap(err, "Could not set optimistic status of genesis block to false")
	}
	s.cfg.ForkChoiceStore.SetGenesisTime(s.genesisTime)

	if err := s.setHead(&head{
		genesisBlkRoot,
		genesisBlk,
		genesisState,
		genesisBlk.Block().Slot(),
		false,
	}); err != nil {
		log.WithError(err).Fatal("Could not set head")
	}
	return nil
}

// This returns true if block has been processed before. Two ways to verify the block has been processed:
// 1.) Check fork choice store.
// 2.) Check DB.
// Checking 1.) is ten times faster than checking 2.)
// this function requires a lock in forkchoice
func (s *Service) hasBlock(ctx context.Context, root [32]byte) bool {
	if s.cfg.ForkChoiceStore.HasNode(root) {
		return true
	}

	return s.cfg.BeaconDB.HasBlock(ctx, root)
}

func (s *Service) removeStartupState() {
	s.cfg.FinalizedStateAtStartUp = nil
}

// UpdateCustodyInfoInDB updates the custody information in the database.
// It returns the (potentially updated) custody group count and the earliest available slot.
func (s *Service) updateCustodyInfoInDB(slot primitives.Slot) (primitives.Slot, uint64, error) {
	isSubscribedToAllDataSubnets := flags.Get().SubscribeAllDataSubnets

	beaconConfig := params.BeaconConfig()
	custodyRequirement := beaconConfig.CustodyRequirement

	// Check if the node was previously subscribed to all data subnets, and if so,
	// store the new status accordingly.
	wasSubscribedToAllDataSubnets, err := s.cfg.BeaconDB.UpdateSubscribedToAllDataSubnets(s.ctx, isSubscribedToAllDataSubnets)
	if err != nil {
		log.WithError(err).Error("Could not update subscription status to all data subnets")
	}

	// Warn the user if the node was previously subscribed to all data subnets and is not any more.
	if wasSubscribedToAllDataSubnets && !isSubscribedToAllDataSubnets {
		log.Warnf(
			"Because the flag `--%s` was previously used, the node will still subscribe to all data subnets.",
			flags.SubscribeAllDataSubnets.Name,
		)
	}

	// Compute the custody group count.
	custodyGroupCount := custodyRequirement
	if isSubscribedToAllDataSubnets {
		custodyGroupCount = beaconConfig.NumberOfCustodyGroups
	}

	// Safely compute the fulu fork slot.
	fuluForkSlot, err := fuluForkSlot()
	if err != nil {
		return 0, 0, errors.Wrap(err, "fulu fork slot")
	}

	// If slot is before the fulu fork slot, then use the earliest stored slot as the reference slot.
	if slot < fuluForkSlot {
		slot, err = s.cfg.BeaconDB.EarliestSlot(s.ctx)
		if err != nil {
			return 0, 0, errors.Wrap(err, "earliest slot")
		}
	}

	earliestAvailableSlot, custodyGroupCount, err := s.cfg.BeaconDB.UpdateCustodyInfo(s.ctx, slot, custodyGroupCount)
	if err != nil {
		return 0, 0, errors.Wrap(err, "update custody info")
	}

	return earliestAvailableSlot, custodyGroupCount, nil
}

func spawnCountdownIfPreGenesis(ctx context.Context, genesisTime time.Time, db db.HeadAccessDatabase) {
	currentTime := prysmTime.Now()
	if currentTime.After(genesisTime) {
		return
	}

	gState, err := db.GenesisState(ctx)
	if err != nil {
		log.WithError(err).Fatal("Could not retrieve genesis state")
	}
	gRoot, err := gState.HashTreeRoot(ctx)
	if err != nil {
		log.WithError(err).Fatal("Could not hash tree root genesis state")
	}
	go slots.CountdownToGenesis(ctx, genesisTime, uint64(gState.NumValidators()), gRoot)
}

func fuluForkSlot() (primitives.Slot, error) {
	beaconConfig := params.BeaconConfig()

	fuluForkEpoch := beaconConfig.FuluForkEpoch
	if fuluForkEpoch == beaconConfig.FarFutureEpoch {
		return beaconConfig.FarFutureSlot, nil
	}

	forkFuluSlot, err := slots.EpochStart(fuluForkEpoch)
	if err != nil {
		return 0, errors.Wrap(err, "epoch start")
	}

	return forkFuluSlot, nil
}
