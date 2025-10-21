// Package node is the main service which launches a beacon node and manages
// the lifecycle of all its associated services at runtime, such as p2p, RPC, sync,
// gracefully closing them if the process ends.
package node

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/OffchainLabs/prysm/v6/api/server/httprest"
	"github.com/OffchainLabs/prysm/v6/api/server/middleware"
	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache/depositsnapshot"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/kv"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/pruner"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/slasherkv"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	lightclient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/monitor"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/node/registration"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/synccommittee"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/slasher"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	regularsync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync/backfill"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync/backfill/coverage"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync/checkpoint"
	initialsync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/container/slice"
	"github.com/OffchainLabs/prysm/v6/genesis"
	"github.com/OffchainLabs/prysm/v6/monitoring/prometheus"
	"github.com/OffchainLabs/prysm/v6/runtime"
	"github.com/OffchainLabs/prysm/v6/runtime/prereqs"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

const testSkipPowFlag = "test-skip-pow"

// Used as a struct to keep cli flag options for configuring services
// for the beacon node. We keep this as a separate struct to not pollute the actual BeaconNode
// struct, as it is merely used to pass down configuration options into the appropriate services.
type serviceFlagOpts struct {
	blockchainFlagOpts     []blockchain.Option
	executionChainFlagOpts []execution.Option
	builderOpts            []builder.Option
}

// BeaconNode defines a struct that handles the services running a random beacon chain
// full PoS node. It handles the lifecycle of the entire system and registers
// services to a service registry.
type BeaconNode struct {
	cliCtx                   *cli.Context
	ctx                      context.Context
	cancel                   context.CancelFunc
	services                 *runtime.ServiceRegistry
	lock                     sync.RWMutex
	stop                     chan struct{} // Channel to wait for termination notifications.
	db                       db.Database
	slasherDB                db.SlasherDatabase
	attestationCache         *cache.AttestationCache
	attestationPool          attestations.Pool
	exitPool                 voluntaryexits.PoolManager
	slashingsPool            slashings.PoolManager
	syncCommitteePool        synccommittee.Pool
	blsToExecPool            blstoexec.PoolManager
	depositCache             cache.DepositCache
	trackedValidatorsCache   *cache.TrackedValidatorsCache
	payloadIDCache           *cache.PayloadIDCache
	stateFeed                *event.Feed
	blockFeed                *event.Feed
	opFeed                   *event.Feed
	stateGen                 *stategen.State
	collector                *bcnodeCollector
	slasherBlockHeadersFeed  *event.Feed
	slasherAttestationsFeed  *event.Feed
	finalizedStateAtStartUp  state.BeaconState
	serviceFlagOpts          *serviceFlagOpts
	GenesisProviders         []genesis.Provider
	CheckpointInitializer    checkpoint.Initializer
	forkChoicer              forkchoice.ForkChoicer
	clockWaiter              startup.ClockWaiter
	BackfillOpts             []backfill.ServiceOption
	initialSyncComplete      chan struct{}
	BlobStorage              *filesystem.BlobStorage
	BlobStorageOptions       []filesystem.BlobStorageOption
	DataColumnStorage        *filesystem.DataColumnStorage
	DataColumnStorageOptions []filesystem.DataColumnStorageOption
	verifyInitWaiter         *verification.InitializerWaiter
	syncChecker              *initialsync.SyncChecker
	slasherEnabled           bool
	lcStore                  *lightclient.Store
	ConfigOptions            []params.Option
}

// New creates a new node instance, sets up configuration options, and registers
// every required service to the node.
func New(cliCtx *cli.Context, cancel context.CancelFunc, opts ...Option) (*BeaconNode, error) {
	if err := configureBeacon(cliCtx); err != nil {
		return nil, errors.Wrap(err, "could not set beacon configuration options")
	}
	ctx := cliCtx.Context

	beacon := &BeaconNode{
		cliCtx:                  cliCtx,
		ctx:                     ctx,
		cancel:                  cancel,
		services:                runtime.NewServiceRegistry(),
		stop:                    make(chan struct{}),
		stateFeed:               new(event.Feed),
		blockFeed:               new(event.Feed),
		opFeed:                  new(event.Feed),
		attestationCache:        cache.NewAttestationCache(),
		attestationPool:         attestations.NewPool(),
		exitPool:                voluntaryexits.NewPool(),
		slashingsPool:           slashings.NewPool(),
		syncCommitteePool:       synccommittee.NewPool(),
		blsToExecPool:           blstoexec.NewPool(),
		trackedValidatorsCache:  cache.NewTrackedValidatorsCache(),
		payloadIDCache:          cache.NewPayloadIDCache(),
		slasherBlockHeadersFeed: new(event.Feed),
		slasherAttestationsFeed: new(event.Feed),
		serviceFlagOpts:         &serviceFlagOpts{},
		initialSyncComplete:     make(chan struct{}),
		syncChecker:             &initialsync.SyncChecker{},
		slasherEnabled:          cliCtx.Bool(flags.SlasherFlag.Name),
	}

	for _, opt := range opts {
		if err := opt(beacon); err != nil {
			return nil, err
		}
	}

	dbClearer := newDbClearer(cliCtx)
	dataDir := cliCtx.String(cmd.DataDirFlag.Name)
	boltFname := filepath.Join(dataDir, kv.BeaconNodeDbDirName)
	kvdb, err := openDB(ctx, boltFname, dbClearer)
	if err != nil {
		return nil, errors.Wrap(err, "could not open database")
	}
	beacon.db = kvdb

	if err := dbClearer.clearGenesis(dataDir); err != nil {
		return nil, errors.Wrap(err, "could not clear genesis state")
	}
	providers := append(beacon.GenesisProviders, kv.NewLegacyGenesisProvider(kvdb))
	if err := genesis.Initialize(ctx, dataDir, providers...); err != nil {
		return nil, errors.Wrap(err, "could not initialize genesis state")
	}

	beacon.ConfigOptions = append([]params.Option{params.WithGenesisValidatorsRoot(genesis.ValidatorsRoot())}, beacon.ConfigOptions...)
	params.BeaconConfig().ApplyOptions(beacon.ConfigOptions...)
	params.BeaconConfig().InitializeForkSchedule()
	params.LogDigests(params.BeaconConfig())

	synchronizer := startup.NewClockSynchronizer()
	beacon.clockWaiter = synchronizer
	beacon.forkChoicer = doublylinkedtree.New()

	depositAddress, err := execution.DepositContractAddress()
	if err != nil {
		return nil, err
	}

	// Allow tests to set it as an opt.
	if beacon.BlobStorage == nil {
		beacon.BlobStorageOptions = append(beacon.BlobStorageOptions, filesystem.WithSaveFsync(features.Get().BlobSaveFsync))
		blobs, err := filesystem.NewBlobStorage(beacon.BlobStorageOptions...)
		if err != nil {
			return nil, err
		}
		beacon.BlobStorage = blobs
	}
	if err := dbClearer.clearBlobs(beacon.BlobStorage); err != nil {
		return nil, errors.Wrap(err, "could not clear blob storage")
	}

	if beacon.DataColumnStorage == nil {
		dataColumnStorage, err := filesystem.NewDataColumnStorage(cliCtx.Context, beacon.DataColumnStorageOptions...)
		if err != nil {
			return nil, errors.Wrap(err, "new data column storage")
		}

		beacon.DataColumnStorage = dataColumnStorage
	}
	if err := dbClearer.clearColumns(beacon.DataColumnStorage); err != nil {
		return nil, errors.Wrap(err, "could not clear data column storage")
	}

	bfs, err := startBaseServices(cliCtx, beacon, depositAddress, dbClearer)
	if err != nil {
		return nil, errors.Wrap(err, "could not start modules")
	}

	beacon.verifyInitWaiter = verification.NewInitializerWaiter(
		beacon.clockWaiter, forkchoice.NewROForkChoice(beacon.forkChoicer), beacon.stateGen)

	beacon.BackfillOpts = append(
		beacon.BackfillOpts,
		backfill.WithVerifierWaiter(beacon.verifyInitWaiter),
		backfill.WithInitSyncWaiter(initSyncWaiter(ctx, beacon.initialSyncComplete)),
	)

	if err := registerServices(cliCtx, beacon, synchronizer, bfs); err != nil {
		return nil, errors.Wrap(err, "could not register services")
	}

	// db.DatabasePath is the path to the containing directory
	// db.NewFileName expands that to the canonical full path using
	// the same construction as NewDB()
	c, err := newBeaconNodePromCollector(db.NewFileName(beacon.db.DatabasePath()))
	if err != nil {
		return nil, err
	}
	beacon.collector = c

	// Do not store the finalized state as it has been provided to the respective services during
	// their initialization.
	beacon.finalizedStateAtStartUp = nil

	return beacon, nil
}

func configureBeacon(cliCtx *cli.Context) error {
	if err := configureTracing(cliCtx); err != nil {
		return errors.Wrap(err, "could not configure tracing")
	}

	prereqs.WarnIfPlatformNotSupported(cliCtx.Context)

	if hasNetworkFlag(cliCtx) && cliCtx.IsSet(cmd.ChainConfigFileFlag.Name) {
		return fmt.Errorf("%s cannot be passed concurrently with network flag", cmd.ChainConfigFileFlag.Name)
	}

	if err := features.ConfigureBeaconChain(cliCtx); err != nil {
		return errors.Wrap(err, "could not configure beacon chain")
	}

	if err := cmd.ConfigureBeaconChain(cliCtx); err != nil {
		return errors.Wrap(err, "could not configure beacon chain")
	}

	flags.ConfigureGlobalFlags(cliCtx)

	if err := configureChainConfig(cliCtx); err != nil {
		return errors.Wrap(err, "could not configure chain config")
	}

	if err := configureHistoricalSlasher(cliCtx); err != nil {
		return errors.Wrap(err, "could not configure historical slasher")
	}

	if err := configureBuilderCircuitBreaker(cliCtx); err != nil {
		return errors.Wrap(err, "could not configure builder circuit breaker")
	}

	if err := configureSlotsPerArchivedPoint(cliCtx); err != nil {
		return errors.Wrap(err, "could not configure slots per archived point")
	}

	if err := configureEth1Config(cliCtx); err != nil {
		return errors.Wrap(err, "could not configure eth1 config")
	}

	configureNetwork(cliCtx)

	if err := configureExecutionSetting(cliCtx); err != nil {
		return errors.Wrap(err, "could not configure execution setting")
	}

	return nil
}

func startBaseServices(cliCtx *cli.Context, beacon *BeaconNode, depositAddress string, clearer *dbClearer) (*backfill.Store, error) {
	ctx := cliCtx.Context
	log.Debugln("Starting DB")
	if err := beacon.startDB(cliCtx, depositAddress); err != nil {
		return nil, errors.Wrap(err, "could not start DB")
	}

	beacon.BlobStorage.WarmCache()
	beacon.DataColumnStorage.WarmCache()

	log.Debugln("Starting Slashing DB")
	if err := beacon.startSlasherDB(cliCtx, clearer); err != nil {
		return nil, errors.Wrap(err, "could not start slashing DB")
	}

	bfs, err := backfill.NewUpdater(ctx, beacon.db)
	if err != nil {
		return nil, errors.Wrap(err, "could not create backfill updater")
	}

	log.Debugln("Starting State Gen")
	if err := beacon.startStateGen(ctx, bfs, beacon.forkChoicer); err != nil {
		if errors.Is(err, stategen.ErrNoGenesisBlock) {
			log.Errorf("No genesis block/state is found. Prysm only provides a mainnet genesis "+
				"state bundled in the application. You must provide the --%s or --%s flag to load "+
				"a genesis block/state for this network.", "genesis-state", "genesis-beacon-api-url")
		}
		return nil, errors.Wrap(err, "could not start state generation")
	}

	return bfs, nil
}

func registerServices(cliCtx *cli.Context, beacon *BeaconNode, synchronizer *startup.ClockSynchronizer, bfs *backfill.Store) error {
	log.Debugln("Registering P2P Service")
	if err := beacon.registerP2P(cliCtx); err != nil {
		return errors.Wrap(err, "could not register P2P service")
	}

	if features.Get().EnableLightClient {
		log.Debugln("Registering Light Client Store")
		beacon.registerLightClientStore()
	}

	log.Debugln("Registering Backfill Service")
	if err := beacon.RegisterBackfillService(cliCtx, bfs); err != nil {
		return errors.Wrap(err, "could not register Back Fill service")
	}

	log.Debugln("Registering POW Chain Service")
	if err := beacon.registerPOWChainService(); err != nil {
		return errors.Wrap(err, "could not register POW chain service")
	}

	log.Debugln("Registering Attestation Pool Service")
	if err := beacon.registerAttestationPool(); err != nil {
		return errors.Wrap(err, "could not register attestation pool service")
	}

	log.Debugln("Registering Blockchain Service")
	if err := beacon.registerBlockchainService(beacon.forkChoicer, synchronizer, beacon.initialSyncComplete); err != nil {
		return errors.Wrap(err, "could not register blockchain service")
	}

	log.Debugln("Registering Initial Sync Service")
	if err := beacon.registerInitialSyncService(beacon.initialSyncComplete); err != nil {
		return errors.Wrap(err, "could not register initial sync service")
	}

	log.Debugln("Registering Sync Service")
	if err := beacon.registerSyncService(beacon.initialSyncComplete, bfs); err != nil {
		return errors.Wrap(err, "could not register sync service")
	}

	log.Debugln("Registering Slashing Pool Service")
	if err := beacon.registerSlashingPoolService(); err != nil {
		return errors.Wrap(err, "could not register slashing pool service")
	}

	log.WithField("enabled", beacon.slasherEnabled).Debugln("Registering Slasher Service")
	if err := beacon.registerSlasherService(); err != nil {
		return errors.Wrap(err, "could not register slasher service")
	}

	log.Debugln("Registering builder service")
	if err := beacon.registerBuilderService(cliCtx); err != nil {
		return errors.Wrap(err, "could not register builder service")
	}

	log.Debugln("Registering RPC Service")
	router := http.NewServeMux()
	if err := beacon.registerRPCService(router); err != nil {
		return errors.Wrap(err, "could not register RPC service")
	}

	log.Debugln("Registering HTTP Service")
	if err := beacon.registerHTTPService(router); err != nil {
		return errors.Wrap(err, "could not register HTTP service")
	}

	log.Debugln("Registering Validator Monitoring Service")
	if err := beacon.registerValidatorMonitorService(beacon.initialSyncComplete); err != nil {
		return errors.Wrap(err, "could not register validator monitoring service")
	}

	if !cliCtx.Bool(cmd.DisableMonitoringFlag.Name) {
		log.Debugln("Registering Prometheus Service")
		if err := beacon.registerPrometheusService(cliCtx); err != nil {
			return errors.Wrap(err, "could not register prometheus service")
		}
	}

	if cliCtx.Bool(flags.BeaconDBPruning.Name) {
		log.Debugln("Registering Pruner Service")
		if err := beacon.registerPrunerService(cliCtx); err != nil {
			return errors.Wrap(err, "could not register pruner service")
		}
	}

	return nil
}

func initSyncWaiter(ctx context.Context, complete chan struct{}) func() error {
	return func() error {
		select {
		case <-complete:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// StateFeed implements statefeed.Notifier.
func (b *BeaconNode) StateFeed() event.SubscriberSender {
	return b.stateFeed
}

// BlockFeed implements blockfeed.Notifier.
func (b *BeaconNode) BlockFeed() *event.Feed {
	return b.blockFeed
}

// OperationFeed implements opfeed.Notifier.
func (b *BeaconNode) OperationFeed() event.SubscriberSender {
	return b.opFeed
}

// Start the BeaconNode and kicks off every registered service.
func (b *BeaconNode) Start() {
	b.lock.Lock()

	log.WithFields(logrus.Fields{
		"version": version.Version(),
	}).Info("Starting beacon node")

	b.services.StartAll()

	stop := b.stop
	b.lock.Unlock()

	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)
		<-sigc
		log.Info("Got interrupt, shutting down...")
		go b.Close()
		for i := 10; i > 0; i-- {
			<-sigc
			if i > 1 {
				log.WithField("times", i-1).Info("Already shutting down, interrupt more to panic")
			}
		}
		panic("Panic closing the beacon node") // lint:nopanic -- Panic is requested by user.
	}()

	// Wait for stop channel to be closed.
	<-stop
}

// Close handles graceful shutdown of the system.
func (b *BeaconNode) Close() {
	b.lock.Lock()
	defer b.lock.Unlock()

	log.Info("Stopping beacon node")
	b.services.StopAll()
	if err := b.db.Close(); err != nil {
		log.WithError(err).Error("Failed to close database")
	}
	b.collector.unregister()
	b.cancel()
	close(b.stop)
}

func (b *BeaconNode) checkAndSaveDepositContract(depositAddress string) error {
	knownContract, err := b.db.DepositContractAddress(b.ctx)
	if err != nil {
		return errors.Wrap(err, "could not get deposit contract address")
	}

	addr := common.HexToAddress(depositAddress)
	if len(knownContract) == 0 {
		if err := b.db.SaveDepositContractAddress(b.ctx, addr); err != nil {
			return errors.Wrap(err, "could not save deposit contract")
		}
	}

	if len(knownContract) > 0 && !bytes.Equal(addr.Bytes(), knownContract) {
		return fmt.Errorf("database contract is %#x but tried to run with %#x. This likely means "+
			"you are trying to run on a different network than what the database contains. You can run once with "+
			"--%s to wipe the old database or use an alternative data directory with --%s",
			knownContract, addr.Bytes(), cmd.ClearDB.Name, cmd.DataDirFlag.Name)
	}

	return nil
}

func openDB(ctx context.Context, dbPath string, clearer *dbClearer) (*kv.Store, error) {
	log.WithField("databasePath", dbPath).Info("Checking DB")

	d, err := kv.NewKVStore(ctx, dbPath)
	if err != nil {
		return nil, errors.Wrapf(err, "could not create database at %s", dbPath)
	}

	d, err = clearer.clearKV(ctx, d)
	if err != nil {
		return nil, errors.Wrap(err, "could not clear database")
	}

	return d, d.RunMigrations(ctx)
}

func (b *BeaconNode) startDB(cliCtx *cli.Context, depositAddress string) error {
	depositCache, err := depositsnapshot.New()
	if err != nil {
		return errors.Wrap(err, "could not create deposit cache")
	}
	b.depositCache = depositCache

	if err := b.db.EnsureEmbeddedGenesis(b.ctx); err != nil {
		return errors.Wrap(err, "could not ensure embedded genesis")
	}

	if b.CheckpointInitializer != nil {
		log.Info("Checkpoint sync - Downloading origin state and block")
		if err := b.CheckpointInitializer.Initialize(b.ctx, b.db); err != nil {
			return err
		}
	}

	if err := b.checkAndSaveDepositContract(depositAddress); err != nil {
		return errors.Wrap(err, "could not check and save deposit contract")
	}

	log.WithField("address", depositAddress).Info("Deposit contract")
	return nil
}
func (b *BeaconNode) startSlasherDB(cliCtx *cli.Context, clearer *dbClearer) error {
	if !b.slasherEnabled {
		return nil
	}
	baseDir := cliCtx.String(cmd.DataDirFlag.Name)
	if cliCtx.IsSet(flags.SlasherDirFlag.Name) {
		baseDir = cliCtx.String(flags.SlasherDirFlag.Name)
	}

	dbPath := filepath.Join(baseDir, kv.BeaconNodeDbDirName)
	log.WithField("databasePath", dbPath).Info("Checking DB")
	d, err := slasherkv.NewKVStore(b.ctx, dbPath)
	if err != nil {
		return err
	}
	d, err = clearer.clearSlasher(b.ctx, d)
	if err != nil {
		return errors.Wrap(err, "could not clear slasher database")
	}
	b.slasherDB = d
	return nil
}

func (b *BeaconNode) startStateGen(ctx context.Context, bfs coverage.AvailableBlocker, fc forkchoice.ForkChoicer) error {
	opts := []stategen.Option{stategen.WithAvailableBlocker(bfs)}
	sg := stategen.New(b.db, fc, opts...)

	cp, err := b.db.FinalizedCheckpoint(ctx)
	if err != nil {
		return err
	}

	b.finalizedStateAtStartUp, err = sg.StateByRoot(ctx, [32]byte(cp.Root))
	if err != nil {
		return err
	}

	b.stateGen = sg
	return nil
}

func parseIPNetStrings(ipWhitelist []string) ([]*net.IPNet, error) {
	ipNets := make([]*net.IPNet, 0, len(ipWhitelist))
	for _, cidr := range ipWhitelist {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			log.WithError(err).WithField("cidr", cidr).Error("Invalid CIDR in IP colocation whitelist")
			return nil, err
		}
		ipNets = append(ipNets, ipNet)
		log.WithField("cidr", cidr).Info("Added IP to colocation whitelist")
	}
	return ipNets, nil
}

func (b *BeaconNode) registerP2P(cliCtx *cli.Context) error {
	bootstrapNodeAddrs, dataDir, err := registration.P2PPreregistration(cliCtx)
	if err != nil {
		return errors.Wrapf(err, "could not register p2p service")
	}

	colocationWhitelist, err := parseIPNetStrings(slice.SplitCommaSeparated(cliCtx.StringSlice(cmd.P2PColocationWhitelist.Name)))
	if err != nil {
		return fmt.Errorf("failed to register p2p service: %w", err)
	}

	svc, err := p2p.NewService(b.ctx, &p2p.Config{
		NoDiscovery:           cliCtx.Bool(cmd.NoDiscovery.Name),
		StaticPeers:           slice.SplitCommaSeparated(cliCtx.StringSlice(cmd.StaticPeers.Name)),
		Discv5BootStrapAddrs:  p2p.ParseBootStrapAddrs(bootstrapNodeAddrs),
		RelayNodeAddr:         cliCtx.String(cmd.RelayNode.Name),
		DataDir:               dataDir,
		DiscoveryDir:          filepath.Join(dataDir, "discovery"),
		LocalIP:               cliCtx.String(cmd.P2PIP.Name),
		HostAddress:           cliCtx.String(cmd.P2PHost.Name),
		HostDNS:               cliCtx.String(cmd.P2PHostDNS.Name),
		PrivateKey:            cliCtx.String(cmd.P2PPrivKey.Name),
		StaticPeerID:          cliCtx.Bool(cmd.P2PStaticID.Name),
		QUICPort:              cliCtx.Uint(cmd.P2PQUICPort.Name),
		TCPPort:               cliCtx.Uint(cmd.P2PTCPPort.Name),
		UDPPort:               cliCtx.Uint(cmd.P2PUDPPort.Name),
		MaxPeers:              cliCtx.Uint(cmd.P2PMaxPeers.Name),
		QueueSize:             cliCtx.Uint(cmd.PubsubQueueSize.Name),
		AllowListCIDR:         cliCtx.String(cmd.P2PAllowList.Name),
		DenyListCIDR:          slice.SplitCommaSeparated(cliCtx.StringSlice(cmd.P2PDenyList.Name)),
		IPColocationWhitelist: colocationWhitelist,
		EnableUPnP:            cliCtx.Bool(cmd.EnableUPnPFlag.Name),
		StateNotifier:         b,
		DB:                    b.db,
		ClockWaiter:           b.clockWaiter,
	})
	if err != nil {
		return err
	}
	return b.services.RegisterService(svc)
}

func (b *BeaconNode) fetchP2P() p2p.P2P {
	var p *p2p.Service
	if err := b.services.FetchService(&p); err != nil {
		panic(err) // lint:nopanic -- This could panic application start if the services are misconfigured.
	}
	return p
}

func (b *BeaconNode) fetchBuilderService() *builder.Service {
	var s *builder.Service
	if err := b.services.FetchService(&s); err != nil {
		panic(err) // lint:nopanic -- This could panic application start if the services are misconfigured.
	}
	return s
}

func (b *BeaconNode) registerAttestationPool() error {
	s, err := attestations.NewService(b.ctx, &attestations.Config{
		Cache:               b.attestationCache,
		Pool:                b.attestationPool,
		InitialSyncComplete: b.initialSyncComplete,
	})
	if err != nil {
		return errors.Wrap(err, "could not register atts pool service")
	}
	return b.services.RegisterService(s)
}

func (b *BeaconNode) registerSlashingPoolService() error {
	var chainService *blockchain.Service
	if err := b.services.FetchService(&chainService); err != nil {
		return err
	}

	s := slashings.NewPoolService(b.ctx, b.slashingsPool, slashings.WithElectraTimer(b.clockWaiter, chainService.CurrentSlot))
	return b.services.RegisterService(s)
}

func (b *BeaconNode) registerBlockchainService(fc forkchoice.ForkChoicer, gs *startup.ClockSynchronizer, syncComplete chan struct{}) error {
	var web3Service *execution.Service
	if err := b.services.FetchService(&web3Service); err != nil {
		return err
	}

	var attService *attestations.Service
	if err := b.services.FetchService(&attService); err != nil {
		return err
	}

	// skipcq: CRT-D0001
	opts := append(
		b.serviceFlagOpts.blockchainFlagOpts,
		blockchain.WithForkChoiceStore(fc),
		blockchain.WithDatabase(b.db),
		blockchain.WithDepositCache(b.depositCache),
		blockchain.WithChainStartFetcher(web3Service),
		blockchain.WithExecutionEngineCaller(web3Service),
		blockchain.WithAttestationCache(b.attestationCache),
		blockchain.WithAttestationPool(b.attestationPool),
		blockchain.WithExitPool(b.exitPool),
		blockchain.WithSlashingPool(b.slashingsPool),
		blockchain.WithBLSToExecPool(b.blsToExecPool),
		blockchain.WithP2PBroadcaster(b.fetchP2P()),
		blockchain.WithStateNotifier(b),
		blockchain.WithAttestationService(attService),
		blockchain.WithStateGen(b.stateGen),
		blockchain.WithSlasherAttestationsFeed(b.slasherAttestationsFeed),
		blockchain.WithFinalizedStateAtStartUp(b.finalizedStateAtStartUp),
		blockchain.WithClockSynchronizer(gs),
		blockchain.WithSyncComplete(syncComplete),
		blockchain.WithBlobStorage(b.BlobStorage),
		blockchain.WithDataColumnStorage(b.DataColumnStorage),
		blockchain.WithTrackedValidatorsCache(b.trackedValidatorsCache),
		blockchain.WithPayloadIDCache(b.payloadIDCache),
		blockchain.WithSyncChecker(b.syncChecker),
		blockchain.WithSlasherEnabled(b.slasherEnabled),
		blockchain.WithLightClientStore(b.lcStore),
	)

	blockchainService, err := blockchain.NewService(b.ctx, opts...)
	if err != nil {
		return errors.Wrap(err, "could not register blockchain service")
	}
	return b.services.RegisterService(blockchainService)
}

func (b *BeaconNode) registerPOWChainService() error {
	if b.cliCtx.Bool(testSkipPowFlag) {
		return b.services.RegisterService(&execution.Service{})
	}
	bs, err := execution.NewPowchainCollector(b.ctx)
	if err != nil {
		return err
	}
	depositContractAddr, err := execution.DepositContractAddress()
	if err != nil {
		return err
	}

	// skipcq: CRT-D0001
	opts := append(
		b.serviceFlagOpts.executionChainFlagOpts,
		execution.WithDepositContractAddress(common.HexToAddress(depositContractAddr)),
		execution.WithDatabase(b.db),
		execution.WithDepositCache(b.depositCache),
		execution.WithStateNotifier(b),
		execution.WithStateGen(b.stateGen),
		execution.WithBeaconNodeStatsUpdater(bs),
		execution.WithFinalizedStateAtStartup(b.finalizedStateAtStartUp),
		execution.WithJwtId(b.cliCtx.String(flags.JwtId.Name)),
		execution.WithVerifierWaiter(b.verifyInitWaiter),
	)
	web3Service, err := execution.NewService(b.ctx, opts...)
	if err != nil {
		return errors.Wrap(err, "could not register proof-of-work chain web3Service")
	}

	return b.services.RegisterService(web3Service)
}

func (b *BeaconNode) registerSyncService(initialSyncComplete chan struct{}, bFillStore *backfill.Store) error {
	var web3Service *execution.Service
	if err := b.services.FetchService(&web3Service); err != nil {
		return err
	}

	var chainService *blockchain.Service
	if err := b.services.FetchService(&chainService); err != nil {
		return err
	}

	var initSync *initialsync.Service
	if err := b.services.FetchService(&initSync); err != nil {
		return err
	}

	rs := regularsync.NewService(
		b.ctx,
		regularsync.WithDatabase(b.db),
		regularsync.WithP2P(b.fetchP2P()),
		regularsync.WithChainService(chainService),
		regularsync.WithInitialSync(initSync),
		regularsync.WithBlockNotifier(b),
		regularsync.WithAttestationNotifier(b),
		regularsync.WithOperationNotifier(b),
		regularsync.WithAttestationCache(b.attestationCache),
		regularsync.WithAttestationPool(b.attestationPool),
		regularsync.WithExitPool(b.exitPool),
		regularsync.WithSlashingPool(b.slashingsPool),
		regularsync.WithSyncCommsPool(b.syncCommitteePool),
		regularsync.WithBlsToExecPool(b.blsToExecPool),
		regularsync.WithStateGen(b.stateGen),
		regularsync.WithSlasherAttestationsFeed(b.slasherAttestationsFeed),
		regularsync.WithSlasherBlockHeadersFeed(b.slasherBlockHeadersFeed),
		regularsync.WithReconstructor(web3Service),
		regularsync.WithClockWaiter(b.clockWaiter),
		regularsync.WithInitialSyncComplete(initialSyncComplete),
		regularsync.WithStateNotifier(b),
		regularsync.WithBlobStorage(b.BlobStorage),
		regularsync.WithDataColumnStorage(b.DataColumnStorage),
		regularsync.WithVerifierWaiter(b.verifyInitWaiter),
		regularsync.WithAvailableBlocker(bFillStore),
		regularsync.WithTrackedValidatorsCache(b.trackedValidatorsCache),
		regularsync.WithSlasherEnabled(b.slasherEnabled),
		regularsync.WithLightClientStore(b.lcStore),
		regularsync.WithBatchVerifierLimit(b.cliCtx.Int(flags.BatchVerifierLimit.Name)),
	)
	return b.services.RegisterService(rs)
}

func (b *BeaconNode) registerInitialSyncService(complete chan struct{}) error {
	var chainService *blockchain.Service
	if err := b.services.FetchService(&chainService); err != nil {
		return err
	}

	opts := []initialsync.Option{
		initialsync.WithVerifierWaiter(b.verifyInitWaiter),
		initialsync.WithSyncChecker(b.syncChecker),
	}
	is := initialsync.NewService(b.ctx, &initialsync.Config{
		DB:                  b.db,
		Chain:               chainService,
		P2P:                 b.fetchP2P(),
		StateNotifier:       b,
		BlockNotifier:       b,
		ClockWaiter:         b.clockWaiter,
		InitialSyncComplete: complete,
		BlobStorage:         b.BlobStorage,
		DataColumnStorage:   b.DataColumnStorage,
	}, opts...)
	return b.services.RegisterService(is)
}

func (b *BeaconNode) registerSlasherService() error {
	if !b.slasherEnabled {
		return nil
	}
	var chainService *blockchain.Service
	if err := b.services.FetchService(&chainService); err != nil {
		return err
	}
	var syncService *initialsync.Service
	if err := b.services.FetchService(&syncService); err != nil {
		return err
	}

	slasherSrv, err := slasher.New(b.ctx, &slasher.ServiceConfig{
		IndexedAttestationsFeed: b.slasherAttestationsFeed,
		BeaconBlockHeadersFeed:  b.slasherBlockHeadersFeed,
		Database:                b.slasherDB,
		StateNotifier:           b,
		AttestationStateFetcher: chainService,
		StateGen:                b.stateGen,
		SlashingPoolInserter:    b.slashingsPool,
		SyncChecker:             syncService,
		HeadStateFetcher:        chainService,
		ClockWaiter:             b.clockWaiter,
	})
	if err != nil {
		return err
	}
	return b.services.RegisterService(slasherSrv)
}

func (b *BeaconNode) registerRPCService(router *http.ServeMux) error {
	var chainService *blockchain.Service
	if err := b.services.FetchService(&chainService); err != nil {
		return err
	}

	var web3Service *execution.Service
	if err := b.services.FetchService(&web3Service); err != nil {
		return err
	}

	var syncService *initialsync.Service
	if err := b.services.FetchService(&syncService); err != nil {
		return err
	}

	var slasherService *slasher.Service
	if b.slasherEnabled {
		if err := b.services.FetchService(&slasherService); err != nil {
			return err
		}
	}

	depositFetcher := b.depositCache
	chainStartFetcher := web3Service

	host := b.cliCtx.String(flags.RPCHost.Name)
	port := b.cliCtx.String(flags.RPCPort.Name)
	beaconMonitoringHost := b.cliCtx.String(cmd.MonitoringHostFlag.Name)
	beaconMonitoringPort := b.cliCtx.Int(flags.MonitoringPortFlag.Name)
	cert := b.cliCtx.String(flags.CertFlag.Name)
	key := b.cliCtx.String(flags.KeyFlag.Name)
	mockEth1DataVotes := b.cliCtx.Bool(flags.InteropMockEth1DataVotesFlag.Name)
	maxMsgSize := b.cliCtx.Int(cmd.GrpcMaxCallRecvMsgSizeFlag.Name)
	enableDebugRPCEndpoints := !b.cliCtx.Bool(flags.DisableDebugRPCEndpoints.Name)

	p2pService := b.fetchP2P()
	rpcService := rpc.NewService(b.ctx, &rpc.Config{
		ExecutionEngineCaller:     web3Service,
		ExecutionReconstructor:    web3Service,
		Host:                      host,
		Port:                      port,
		BeaconMonitoringHost:      beaconMonitoringHost,
		BeaconMonitoringPort:      beaconMonitoringPort,
		CertFlag:                  cert,
		KeyFlag:                   key,
		BeaconDB:                  b.db,
		Broadcaster:               p2pService,
		PeersFetcher:              p2pService,
		PeerManager:               p2pService,
		MetadataProvider:          p2pService,
		ChainInfoFetcher:          chainService,
		HeadFetcher:               chainService,
		CanonicalFetcher:          chainService,
		ForkFetcher:               chainService,
		ForkchoiceFetcher:         chainService,
		FinalizationFetcher:       chainService,
		BlockReceiver:             chainService,
		BlobReceiver:              chainService,
		DataColumnReceiver:        chainService,
		AttestationReceiver:       chainService,
		GenesisTimeFetcher:        chainService,
		GenesisFetcher:            chainService,
		OptimisticModeFetcher:     chainService,
		AttestationCache:          b.attestationCache,
		AttestationsPool:          b.attestationPool,
		ExitPool:                  b.exitPool,
		SlashingsPool:             b.slashingsPool,
		BLSChangesPool:            b.blsToExecPool,
		SyncCommitteeObjectPool:   b.syncCommitteePool,
		ExecutionChainService:     web3Service,
		ExecutionChainInfoFetcher: web3Service,
		ChainStartFetcher:         chainStartFetcher,
		MockEth1Votes:             mockEth1DataVotes,
		SyncService:               syncService,
		DepositFetcher:            depositFetcher,
		PendingDepositFetcher:     b.depositCache,
		BlockNotifier:             b,
		StateNotifier:             b,
		OperationNotifier:         b,
		StateGen:                  b.stateGen,
		EnableDebugRPCEndpoints:   enableDebugRPCEndpoints,
		MaxMsgSize:                maxMsgSize,
		BlockBuilder:              b.fetchBuilderService(),
		Router:                    router,
		ClockWaiter:               b.clockWaiter,
		BlobStorage:               b.BlobStorage,
		DataColumnStorage:         b.DataColumnStorage,
		TrackedValidatorsCache:    b.trackedValidatorsCache,
		PayloadIDCache:            b.payloadIDCache,
		LCStore:                   b.lcStore,
	})

	return b.services.RegisterService(rpcService)
}

func (b *BeaconNode) registerPrometheusService(_ *cli.Context) error {
	var additionalHandlers []prometheus.Handler
	var p *p2p.Service
	if err := b.services.FetchService(&p); err != nil {
		panic(err) // lint:nopanic -- This could panic application start if the services are misconfigured.
	}
	additionalHandlers = append(additionalHandlers, prometheus.Handler{Path: "/p2p", Handler: p.InfoHandler})

	var c *blockchain.Service
	if err := b.services.FetchService(&c); err != nil {
		panic(err) // lint:nopanic -- This could panic application start if the services are misconfigured.
	}

	service := prometheus.NewService(
		b.cliCtx.Context,
		fmt.Sprintf("%s:%d", b.cliCtx.String(cmd.MonitoringHostFlag.Name), b.cliCtx.Int(flags.MonitoringPortFlag.Name)),
		b.services,
		additionalHandlers...,
	)
	hook := prometheus.NewLogrusCollector()
	logrus.AddHook(hook)
	return b.services.RegisterService(service)
}

func (b *BeaconNode) registerHTTPService(router *http.ServeMux) error {
	host := b.cliCtx.String(flags.HTTPServerHost.Name)
	port := b.cliCtx.Int(flags.HTTPServerPort.Name)
	address := net.JoinHostPort(host, strconv.Itoa(port))
	var allowedOrigins []string
	if b.cliCtx.IsSet(flags.HTTPServerCorsDomain.Name) {
		allowedOrigins = strings.Split(b.cliCtx.String(flags.HTTPServerCorsDomain.Name), ",")
	} else {
		allowedOrigins = strings.Split(flags.HTTPServerCorsDomain.Value, ",")
	}

	middlewares := []middleware.Middleware{
		middleware.NormalizeQueryValuesHandler,
		middleware.CorsHandler(allowedOrigins),
	}

	opts := []httprest.Option{
		httprest.WithRouter(router),
		httprest.WithHTTPAddr(address),
		httprest.WithMiddlewares(middlewares),
	}
	if b.cliCtx.IsSet(cmd.ApiTimeoutFlag.Name) {
		opts = append(opts, httprest.WithTimeout(b.cliCtx.Duration(cmd.ApiTimeoutFlag.Name)))
	}
	g, err := httprest.New(b.ctx, opts...)
	if err != nil {
		return err
	}
	return b.services.RegisterService(g)
}

func (b *BeaconNode) registerValidatorMonitorService(initialSyncComplete chan struct{}) error {
	cliSlice := b.cliCtx.IntSlice(cmd.ValidatorMonitorIndicesFlag.Name)
	if cliSlice == nil {
		return nil
	}
	tracked := make([]primitives.ValidatorIndex, len(cliSlice))
	for i := range tracked {
		tracked[i] = primitives.ValidatorIndex(cliSlice[i])
	}

	var chainService *blockchain.Service
	if err := b.services.FetchService(&chainService); err != nil {
		return err
	}
	monitorConfig := &monitor.ValidatorMonitorConfig{
		StateNotifier:       b,
		AttestationNotifier: b,
		StateGen:            b.stateGen,
		HeadFetcher:         chainService,
		InitialSyncComplete: initialSyncComplete,
	}
	svc, err := monitor.NewService(b.ctx, monitorConfig, tracked)
	if err != nil {
		return err
	}
	return b.services.RegisterService(svc)
}

func (b *BeaconNode) registerBuilderService(cliCtx *cli.Context) error {
	var chainService *blockchain.Service
	if err := b.services.FetchService(&chainService); err != nil {
		return err
	}

	opts := b.serviceFlagOpts.builderOpts
	opts = append(opts, builder.WithHeadFetcher(chainService), builder.WithDatabase(b.db))

	// make cache the default.
	if !cliCtx.Bool(features.DisableRegistrationCache.Name) {
		opts = append(opts, builder.WithRegistrationCache())
	}
	svc, err := builder.NewService(b.ctx, opts...)
	if err != nil {
		return err
	}
	return b.services.RegisterService(svc)
}

func (b *BeaconNode) registerPrunerService(cliCtx *cli.Context) error {
	genesis := time.Unix(int64(params.BeaconConfig().MinGenesisTime+params.BeaconConfig().GenesisDelay), 0)
	var backfillService *backfill.Service
	if err := b.services.FetchService(&backfillService); err != nil {
		return err
	}

	var opts []pruner.ServiceOption
	if cliCtx.IsSet(flags.PrunerRetentionEpochs.Name) {
		uv := cliCtx.Uint64(flags.PrunerRetentionEpochs.Name)
		opts = append(opts, pruner.WithRetentionPeriod(primitives.Epoch(uv)))
	}

	p, err := pruner.New(
		cliCtx.Context,
		b.db,
		genesis,
		initSyncWaiter(cliCtx.Context, b.initialSyncComplete),
		backfillService.WaitForCompletion,
		b.fetchP2P(),
		opts...,
	)
	if err != nil {
		return err
	}

	return b.services.RegisterService(p)
}

func (b *BeaconNode) RegisterBackfillService(cliCtx *cli.Context, bfs *backfill.Store) error {
	pa := peers.NewAssigner(b.fetchP2P().Peers(), b.forkChoicer)
	bf, err := backfill.NewService(cliCtx.Context, bfs, b.BlobStorage, b.clockWaiter, b.fetchP2P(), pa, b.BackfillOpts...)
	if err != nil {
		return errors.Wrap(err, "error initializing backfill service")
	}

	return b.services.RegisterService(bf)
}

func (b *BeaconNode) registerLightClientStore() {
	lcs := lightclient.NewLightClientStore(b.fetchP2P(), b.StateFeed(), b.db)
	b.lcStore = lcs
}

func hasNetworkFlag(cliCtx *cli.Context) bool {
	for _, flag := range features.NetworkFlags {
		for _, name := range flag.Names() {
			if cliCtx.IsSet(name) {
				return true
			}
		}
	}
	return false
}
