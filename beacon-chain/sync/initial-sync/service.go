// Package initialsync includes all initial block download and processing
// logic for the beacon node, using a round robin strategy and a finite-state-machine
// to handle edge-cases in a beacon node's sync status.
package initialsync

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/paulbellamy/ratecounter"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/async/abool"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	blockfeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/block"
	statefeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/das"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2ptypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/sync"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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
}

// Service service.
type Service struct {
	cfg             *Config
	ctx             context.Context
	cancel          context.CancelFunc
	synced          *abool.AtomicBool
	chainStarted    *abool.AtomicBool
	counter         *ratecounter.RateCounter
	genesisChan     chan time.Time
	clock           *startup.Clock
	verifierWaiter  *verification.InitializerWaiter
	newBlobVerifier verification.NewBlobVerifier
	ctxMap          sync.ContextByteVersions
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
		log.WithError(err).Error("initial-sync failed to receive startup event")
		return
	}
	s.clock = clock
	log.Info("Received state initialized event")
	ctxMap, err := sync.ContextByteVersionsForValRoot(clock.GenesisValidatorsRoot())
	if err != nil {
		log.WithField("genesisValidatorRoot", clock.GenesisValidatorsRoot()).
			WithError(err).Error("unable to initialize context version map using genesis validator")
		return
	}
	s.ctxMap = ctxMap

	v, err := s.verifierWaiter.WaitForInitializer(s.ctx)
	if err != nil {
		log.WithError(err).Error("Could not get verification initializer")
		return
	}
	s.newBlobVerifier = newBlobVerifierFromInitializer(v)

	gt := clock.GenesisTime()
	if gt.IsZero() {
		log.Debug("Exiting Initial Sync Service")
		return
	}
	// Exit entering round-robin sync if we require 0 peers to sync.
	if flags.Get().MinimumSyncPeers == 0 {
		s.markSynced()
		log.WithField("genesisTime", gt).Info("Due to number of peers required for sync being set at 0, entering regular sync immediately.")
		return
	}
	if gt.After(prysmTime.Now()) {
		s.markSynced()
		log.WithField("genesisTime", gt).Info("Genesis time has not arrived - not syncing")
		return
	}
	currentSlot := clock.CurrentSlot()
	if slots.ToEpoch(currentSlot) == 0 {
		log.WithField("genesisTime", gt).Info("Chain started within the last epoch - not syncing")
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
	if err := s.fetchOriginBlobs(peers); err != nil {
		log.WithError(err).Error("Failed to fetch missing blobs for checkpoint origin")
		return
	}
	if err := s.roundRobinSync(gt); err != nil {
		if errors.Is(s.ctx.Err(), context.Canceled) {
			return
		}
		panic(err)
	}
	log.WithField("slot", s.cfg.Chain.HeadSlot()).Info("Synced up to")
	s.markSynced()
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
	defer func() { s.synced.Set() }()                       // Reset it at the end of the method.
	genesis := time.Unix(int64(headState.GenesisTime()), 0) // lint:ignore uintcast -- Genesis time will not exceed int64 in your lifetime.

	_, err = s.waitForMinimumPeers()
	if err != nil {
		return err
	}
	if err = s.roundRobinSync(genesis); err != nil {
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
	onDisk, err := store.Indices(r)
	if err != nil {
		return nil, errors.Wrapf(err, "error checking existing blobs for checkpoint sync block root %#x", r)
	}
	req := make(p2ptypes.BlobSidecarsByRootReq, 0, len(cmts))
	for i := range cmts {
		if onDisk[i] {
			continue
		}
		req = append(req, &eth.BlobIdentifier{BlockRoot: r[:], Index: uint64(i)})
	}
	return req, nil
}

func (s *Service) fetchOriginBlobs(pids []peer.ID) error {
	r, err := s.cfg.DB.OriginCheckpointBlockRoot(s.ctx)
	if errors.Is(err, db.ErrNotFoundOriginBlockRoot) {
		return nil
	}
	blk, err := s.cfg.DB.Block(s.ctx, r)
	if err != nil {
		log.WithField("root", fmt.Sprintf("%#x", r)).Error("Block for checkpoint sync origin root not found in db")
		return err
	}
	if !params.WithinDAPeriod(slots.ToEpoch(blk.Block().Slot()), slots.ToEpoch(s.clock.CurrentSlot())) {
		return nil
	}
	rob, err := blocks.NewROBlockWithRoot(blk, r)
	if err != nil {
		return err
	}
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
		sidecars, err := sync.SendBlobSidecarByRoot(s.ctx, s.clock, s.cfg.P2P, pids[i], s.ctxMap, &req)
		if err != nil {
			continue
		}
		if len(sidecars) != len(req) {
			continue
		}
		bv := verification.NewBlobBatchVerifier(s.newBlobVerifier, verification.InitsyncSidecarRequirements)
		avs := das.NewLazilyPersistentStore(s.cfg.BlobStorage, bv)
		current := s.clock.CurrentSlot()
		if err := avs.Persist(current, sidecars...); err != nil {
			return err
		}
		if err := avs.IsDataAvailable(s.ctx, current, rob); err != nil {
			log.WithField("root", fmt.Sprintf("%#x", r)).WithField("peerID", pids[i]).Warn("Blobs from peer for origin block were unusable")
			continue
		}
		log.WithField("nBlobs", len(sidecars)).WithField("root", fmt.Sprintf("%#x", r)).Info("Successfully downloaded blobs for checkpoint sync block")
		return nil
	}
	return fmt.Errorf("no connected peer able to provide blobs for checkpoint sync block %#x", r)
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
