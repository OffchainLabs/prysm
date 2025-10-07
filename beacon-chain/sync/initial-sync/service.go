// Package initialsync includes all initial block download and processing
// logic for the beacon node, using a round robin strategy and a finite-state-machine
// to handle edge-cases in a beacon node's sync status.
package initialsync

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	blockfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/block"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/paulbellamy/ratecounter"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var _ runtime.Service = (*Service)(nil)

// blockchainService defines the interface for interaction with block chain service.
type blockchainService interface {
	blockchain.BlockReceiver
	blockchain.ChainInfoFetcher
}

// Config to set up the initial sync service.
type Config struct {
	P2P                 p2p.P2P
	DB                  db.NoHeadAccessDatabase
	Chain               blockchainService
	StateNotifier       statefeed.Notifier
	BlockNotifier       blockfeed.Notifier
	ClockWaiter         startup.ClockWaiter
	InitialSyncComplete chan struct{}
	BlobStorage         *filesystem.BlobStorage
	DataColumnStorage   *filesystem.DataColumnStorage
}

// Service service.
type Service struct {
	cfg                    *Config
	ctx                    context.Context
	cancel                 context.CancelFunc
	synced                 *abool.AtomicBool
	chainStarted           *abool.AtomicBool
	counter                *ratecounter.RateCounter
	genesisChan            chan time.Time
	clock                  *startup.Clock
	verifierWaiter         *verification.InitializerWaiter
	newBlobVerifier        verification.NewBlobVerifier
	newDataColumnsVerifier verification.NewDataColumnsVerifier
	ctxMap                 sync.ContextByteVersions
	genesisTime            time.Time
}

// Option is a functional option for the initial-sync Service.
type Option func(*Service)

// WithVerifierWaiter sets the verification.InitializerWaiter
// for the initial-sync Service.
func WithVerifierWaiter(viw *verification.InitializerWaiter) Option {
	return func(s *Service) {
		s.verifierWaiter = viw
	}
}

// WithSyncChecker registers the initial sync service
// in the checker.
func WithSyncChecker(checker *SyncChecker) Option {
	return func(service *Service) {
		checker.Svc = service
	}
}

// SyncChecker allows other services to check the current status of
// initial-sync and use that internally in their service.
type SyncChecker struct {
	Svc *Service
}

// Synced returns the status of the service.
func (s *SyncChecker) Synced() bool {
	if s.Svc == nil {
		log.Warn("Calling sync checker with a nil service initialized")
		return false
	}
	return s.Svc.Synced()
}

// NewService configures the initial sync service responsible for bringing the node up to the
// latest head of the blockchain.
func NewService(ctx context.Context, cfg *Config, opts ...Option) *Service {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{
		cfg:          cfg,
		ctx:          ctx,
		cancel:       cancel,
		synced:       abool.New(),
		chainStarted: abool.New(),
		counter:      ratecounter.NewRateCounter(counterSeconds * time.Second),
		genesisChan:  make(chan time.Time),
		clock:        startup.NewClock(time.Unix(0, 0), [32]byte{}), // default clock to prevent panic
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Start the initial sync service.
func (s *Service) Start() {
	log.Info("Waiting for state to be initialized")
	clock, err := s.cfg.ClockWaiter.WaitForClock(s.ctx)
	if err != nil {
		log.WithError(err).Error("Initial-sync failed to receive startup event")
		return
	}
	s.clock = clock
	log.Info("Received state initialized event")
	ctxMap, err := sync.ContextByteVersionsForValRoot(clock.GenesisValidatorsRoot())
	if err != nil {
		log.WithField("genesisValidatorRoot", clock.GenesisValidatorsRoot()).
			WithError(err).Error("Unable to initialize context version map using genesis validator")
		return
	}
	s.ctxMap = ctxMap

	v, err := s.verifierWaiter.WaitForInitializer(s.ctx)
	if err != nil {
		log.WithError(err).Error("Could not get verification initializer")
		return
	}
	s.newBlobVerifier = newBlobVerifierFromInitializer(v)
	s.newDataColumnsVerifier = newDataColumnsVerifierFromInitializer(v)

	gt := clock.GenesisTime()
	if gt.IsZero() {
		log.Debug("Exiting Initial Sync Service")
		return
	}
	s.genesisTime = gt
	// Exit entering round-robin sync if we require 0 peers to sync.
	if flags.Get().MinimumSyncPeers == 0 {
		s.markSynced()
		log.WithField("genesisTime", s.genesisTime).Info("Due to number of peers required for sync being set at 0, entering regular sync immediately.")
		return
	}
	if s.genesisTime.After(prysmTime.Now()) {
		s.markSynced()
		log.WithField("genesisTime", s.genesisTime).Info("Genesis time has not arrived - not syncing")
		return
	}
	currentSlot := clock.CurrentSlot()
	if slots.ToEpoch(currentSlot) == 0 {
		log.WithField("genesisTime", s.genesisTime).Info("Chain started within the last epoch - not syncing")
		s.markSynced()
		return
	}
	s.chainStarted.Set()
	log.Info("Starting initial chain sync...")

	// Are we already in sync, or close to it?
	if slots.ToEpoch(s.cfg.Chain.HeadSlot()) == slots.ToEpoch(currentSlot) {
		log.Info("Already synced to the current chain head")
		s.markSynced()
		return
	}

	peers, err := s.waitForMinimumPeers()
	if err != nil {
		log.WithError(err).Error("Error waiting for minimum number of peers")
		return
	}

	if err := s.fetchOriginSidecars(peers); err != nil {
		log.WithError(err).Error("Error fetching origin sidecars")
		return
	}
	if err := s.roundRobinSync(); err != nil {
		if errors.Is(s.ctx.Err(), context.Canceled) {
			return
		}
		panic(err) // lint:nopanic -- Unexpected error. This should probably be surfaced with a returned error.
	}
	log.WithField("slot", s.cfg.Chain.HeadSlot()).Info("Synced up to")
	s.markSynced()
}

// fetchOriginSidecars fetches origin sidecars
func (s *Service) fetchOriginSidecars(peers []peer.ID) error {
	const delay = 10 * time.Second // The delay between each attempt to fetch origin data column sidecars

	blockRoot, err := s.cfg.DB.OriginCheckpointBlockRoot(s.ctx)
	if errors.Is(err, db.ErrNotFoundOriginBlockRoot) {
		return nil
	}

	block, err := s.cfg.DB.Block(s.ctx, blockRoot)
	if err != nil {
		return errors.Wrap(err, "block")
	}

	currentSlot, blockSlot := s.clock.CurrentSlot(), block.Block().Slot()
	currentEpoch, blockEpoch := slots.ToEpoch(currentSlot), slots.ToEpoch(blockSlot)

	if !params.WithinDAPeriod(blockEpoch, currentEpoch) {
		return nil
	}

	roBlock, err := blocks.NewROBlockWithRoot(block, blockRoot)
	if err != nil {
		return errors.Wrap(err, "new ro block with root")
	}

	blockVersion := roBlock.Version()

	if blockVersion >= version.Fulu {
		if err := s.fetchOriginDataColumnSidecars(roBlock, delay); err != nil {
			return errors.Wrap(err, "fetch origin columns")
		}
		return nil
	}

	if blockVersion >= version.Deneb {
		if err := s.fetchOriginBlobSidecars(peers, roBlock); err != nil {
			return errors.Wrap(err, "fetch origin blobs")
		}
	}

	return nil
}

// Stop initial sync.
func (s *Service) Stop() error {
	s.cancel()
	return nil
}

// Status of initial sync.
func (s *Service) Status() error {
	if s.synced.IsNotSet() && s.chainStarted.IsSet() {
		return errors.New("syncing")
	}
	return nil
}

// Syncing returns true if initial sync is still running.
func (s *Service) Syncing() bool {
	return s.synced.IsNotSet()
}

// Initialized returns true if initial sync has been started.
func (s *Service) Initialized() bool {
	return s.chainStarted.IsSet()
}

// Synced returns true if initial sync has been completed.
func (s *Service) Synced() bool {
	return s.synced.IsSet()
}

// Resync allows a node to start syncing again if it has fallen
// behind the current network head.
func (s *Service) Resync() error {
	headState, err := s.cfg.Chain.HeadState(s.ctx)
	if err != nil || headState == nil || headState.IsNil() {
		return errors.Errorf("could not retrieve head state: %v", err)
	}

	// Set it to false since we are syncing again.
	s.synced.UnSet()
	defer func() { s.synced.Set() }() // Reset it at the end of the method.

	_, err = s.waitForMinimumPeers()
	if err != nil {
		return err
	}
	if err = s.roundRobinSync(); err != nil {
		log = log.WithError(err)
	}
	log.WithField("slot", s.cfg.Chain.HeadSlot()).Info("Resync attempt complete")
	return nil
}

func (s *Service) waitForMinimumPeers() ([]peer.ID, error) {
	required := params.BeaconConfig().MaxPeersToSync
	if flags.Get().MinimumSyncPeers < required {
		required = flags.Get().MinimumSyncPeers
	}
	for {
		if s.ctx.Err() != nil {
			return nil, s.ctx.Err()
		}
		cp := s.cfg.Chain.FinalizedCheckpt()
		_, peers := s.cfg.P2P.Peers().BestNonFinalized(flags.Get().MinimumSyncPeers, cp.Epoch)
		if len(peers) >= required {
			return peers, nil
		}
		log.WithFields(logrus.Fields{
			"suitable": len(peers),
			"required": required,
		}).Info("Waiting for enough suitable peers before syncing")
		time.Sleep(handshakePollingInterval)
	}
}

// markSynced marks node as synced and notifies feed listeners.
func (s *Service) markSynced() {
	s.synced.Set()
	close(s.cfg.InitialSyncComplete)
}

func missingBlobRequest(blk blocks.ROBlock, store *filesystem.BlobStorage) (p2ptypes.BlobSidecarsByRootReq, error) {
	r := blk.Root()
	if blk.Version() < version.Deneb {
		return nil, nil
	}
	cmts, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		log.WithField("root", r).Error("Error reading commitments from checkpoint sync origin block")
		return nil, err
	}
	if len(cmts) == 0 {
		return nil, nil
	}
	onDisk := store.Summary(r)
	req := make(p2ptypes.BlobSidecarsByRootReq, 0, len(cmts))
	for i := range cmts {
		if onDisk.HasIndex(uint64(i)) {
			continue
		}
		req = append(req, &eth.BlobIdentifier{BlockRoot: r[:], Index: uint64(i)})
	}
	return req, nil
}

func (s *Service) fetchOriginBlobSidecars(pids []peer.ID, rob blocks.ROBlock) error {
	r := rob.Root()

	req, err := missingBlobRequest(rob, s.cfg.BlobStorage)
	if err != nil {
		return err
	}
	if len(req) == 0 {
		log.WithField("root", fmt.Sprintf("%#x", r)).Debug("All blobs for checkpoint block are present")
		return nil
	}
	shufflePeers(pids)
	for i := range pids {
		blobSidecars, err := sync.SendBlobSidecarByRoot(s.ctx, s.clock, s.cfg.P2P, pids[i], s.ctxMap, &req, rob.Block().Slot())
		if err != nil {
			continue
		}

		if len(blobSidecars) != len(req) {
			continue
		}
		bv := verification.NewBlobBatchVerifier(s.newBlobVerifier, verification.InitsyncBlobSidecarRequirements)
		avs := das.NewLazilyPersistentStore(s.cfg.BlobStorage, bv)
		current := s.clock.CurrentSlot()
		if err := avs.Persist(current, blobSidecars...); err != nil {
			return err
		}

		if err := avs.IsDataAvailable(s.ctx, current, rob); err != nil {
			log.WithField("root", fmt.Sprintf("%#x", r)).WithField("peerID", pids[i]).Warn("Blobs from peer for origin block were unusable")
			continue
		}
		log.WithField("nBlobs", len(blobSidecars)).WithField("root", fmt.Sprintf("%#x", r)).Info("Successfully downloaded blobs for checkpoint sync block")
		return nil
	}
	return fmt.Errorf("no connected peer able to provide blobs for checkpoint sync block %#x", r)
}

func (s *Service) fetchOriginDataColumnSidecars(roBlock blocks.ROBlock, delay time.Duration) error {
	const (
		errorMessage     = "Failed to fetch origin data column sidecars"
		warningIteration = 10
	)

	samplesPerSlot := params.BeaconConfig().SamplesPerSlot

	// Return early if the origin block has no blob commitments.
	commitments, err := roBlock.Block().Body().BlobKzgCommitments()
	if err != nil {
		return errors.Wrap(err, "fetch blob commitments")
	}

	if len(commitments) == 0 {
		return nil
	}

	// Compute the indices we need to custody.
	custodyGroupCount, err := s.cfg.P2P.CustodyGroupCount(s.ctx)
	if err != nil {
		return errors.Wrap(err, "custody group count")
	}

	samplingSize := max(custodyGroupCount, samplesPerSlot)
	info, _, err := peerdas.Info(s.cfg.P2P.NodeID(), samplingSize)
	if err != nil {
		return errors.Wrap(err, "fetch peer info")
	}

	root := roBlock.Root()

	log := log.WithFields(logrus.Fields{
		"blockRoot":       fmt.Sprintf("%#x", roBlock.Root()),
		"blobCount":       len(commitments),
		"dataColumnCount": len(info.CustodyColumns),
	})

	// Check if some needed data column sidecars are missing.
	stored := s.cfg.DataColumnStorage.Summary(root).Stored()
	missing := make(map[uint64]bool, len(info.CustodyColumns))
	for column := range info.CustodyColumns {
		if !stored[column] {
			missing[column] = true
		}
	}

	if len(missing) == 0 {
		// All needed data column sidecars are present, exit early.
		log.Info("All needed origin data column sidecars are already present")

		return nil
	}

	params := sync.DataColumnSidecarsParams{
		Ctx:                     s.ctx,
		Tor:                     s.clock,
		P2P:                     s.cfg.P2P,
		CtxMap:                  s.ctxMap,
		Storage:                 s.cfg.DataColumnStorage,
		NewVerifier:             s.newDataColumnsVerifier,
		DownscorePeerOnRPCFault: true,
	}

	for attempt := uint64(0); ; attempt++ {
		// Retrieve missing data column sidecars.
		verifiedRoSidecarsByRoot, missingIndicesByRoot, err := sync.FetchDataColumnSidecars(params, []blocks.ROBlock{roBlock}, missing)
		if err != nil {
			return errors.Wrap(err, "fetch data column sidecars")
		}

		// Save retrieved data column sidecars.
		if err := s.cfg.DataColumnStorage.Save(verifiedRoSidecarsByRoot[root]); err != nil {
			return errors.Wrap(err, "save data column sidecars")
		}

		// Check if some needed data column sidecars are missing.
		if len(missingIndicesByRoot) == 0 {
			log.Info("Retrieved all needed origin data column sidecars")

			return nil
		}

		// Some sidecars are still missing.
		log := log.WithFields(logrus.Fields{
			"attempt":        attempt,
			"missingIndices": helpers.SortedPrettySliceFromMap(missingIndicesByRoot[root]),
			"delay":          delay,
		})

		logFunc := log.Debug
		if attempt > 0 && attempt%warningIteration == 0 {
			logFunc = log.Warning
		}

		logFunc("Failed to fetch some origin data column sidecars, retrying later")
	}
}

func shufflePeers(pids []peer.ID) {
	rg := rand.NewGenerator()
	rg.Shuffle(len(pids), func(i, j int) {
		pids[i], pids[j] = pids[j], pids[i]
	})
}

func newBlobVerifierFromInitializer(ini *verification.Initializer) verification.NewBlobVerifier {
	return func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
		return ini.NewBlobVerifier(b, reqs)
	}
}

func newDataColumnsVerifierFromInitializer(ini *verification.Initializer) verification.NewDataColumnsVerifier {
	return func(roDataColumns []blocks.RODataColumn, reqs []verification.Requirement) verification.DataColumnsVerifier {
		return ini.NewDataColumnsVerifier(roDataColumns, reqs)
	}
}
