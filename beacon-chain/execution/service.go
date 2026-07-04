// Package execution defines a runtime service which is tasked with
// communicating with an eth1 endpoint, providing the execution engine API
// client and the latest eth1 data headers for usage in the beacon node.
package execution

import (
	"context"
	"math/big"
	"runtime/debug"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/monitoring/clientstats"
	"github.com/OffchainLabs/prysm/v7/network"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

var blockNumberGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "powchain_block_number",
	Help: "The current block number in the proof-of-work chain",
})

var (
	// time to wait before trying to reconnect with the eth1 node.
	backOffPeriod = 15 * time.Second
	// amount of times before we log the status of the eth1 dial attempt.
	logThreshold = 8
)

// ChainInfoFetcher retrieves information about the execution client connection.
type ChainInfoFetcher interface {
	ExecutionClientConnected() bool
	ExecutionClientEndpoint() string
	ExecutionClientConnectionErr() error
}

// POWBlockFetcher defines a struct that can retrieve mainchain blocks.
type POWBlockFetcher interface {
	BlockTimeByHeight(ctx context.Context, height *big.Int) (uint64, error)
	BlockByTimestamp(ctx context.Context, time uint64) (*types.HeaderInfo, error)
	BlockHashByHeight(ctx context.Context, height *big.Int) (common.Hash, error)
	BlockExists(ctx context.Context, hash common.Hash) (bool, *big.Int, error)
}

// Chain defines a standard interface for the powchain service in Prysm.
type Chain interface {
	ChainInfoFetcher
	POWBlockFetcher
}

// RPCClient defines the rpc methods required to interact with the eth1 node.
type RPCClient interface {
	Close()
	BatchCall(b []gethRPC.BatchElem) error
	CallContext(ctx context.Context, result any, method string, args ...any) error
}

type RPCClientEmpty struct {
}

func (RPCClientEmpty) Close() {}
func (RPCClientEmpty) BatchCall([]gethRPC.BatchElem) error {
	return errors.New("rpc client is not initialized")
}

func (RPCClientEmpty) CallContext(context.Context, any, string, ...any) error {
	return errors.New("rpc client is not initialized")
}

type latestEth1Data struct {
	BlockHeight uint64
	BlockTime   uint64
	BlockHash   []byte
}

// config defines a config struct for dependencies into the service.
type config struct {
	beaconNodeStatsUpdater BeaconNodeStatsUpdater
	currHttpEndpoint       network.Endpoint
	headers                []string
	jwtId                  string
}

// Service fetches important information about the canonical
// eth1 chain via a web3 endpoint using an ethclient.
// The beacon chain requires synchronization with the eth1 chain's current
// block hash, block number, and access to logs within the
// Validator Registration Contract on the eth1 chain to kick off the beacon
// chain's validator registration process.
type Service struct {
	partialColumnsSupported bool
	connectedETH1           bool
	isRunning               bool
	processingLock          sync.RWMutex
	latestEth1DataLock      sync.RWMutex
	cfg                     *config
	ctx                     context.Context
	cancel                  context.CancelFunc
	eth1HeadTicker          *time.Ticker
	httpLogger              bind.ContractFilterer
	rpcClient               RPCClient
	headerCache             *headerCache // cache to store block hash/block height.
	latestEth1Data          *latestEth1Data
	runError                error
	verifierWaiter          *verification.InitializerWaiter
	blobVerifier            verification.NewBlobVerifier
	capabilityCache         *capabilityCache
	graffitiInfo            *GraffitiInfo
}

// NewService sets up a new instance with an ethclient when given a web3 endpoint as a string in the config.
func NewService(ctx context.Context, opts ...Option) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)
	_ = cancel // govet fix for lost cancel. Cancel is handled in service.Stop()

	s := &Service{
		ctx:       ctx,
		cancel:    cancel,
		rpcClient: RPCClientEmpty{},
		cfg: &config{
			beaconNodeStatsUpdater: &NopBeaconNodeStatsUpdater{},
		},
		latestEth1Data:  &latestEth1Data{},
		headerCache:     newHeaderCache(),
		eth1HeadTicker:  time.NewTicker(time.Duration(params.BeaconConfig().SecondsPerETH1Block) * time.Second),
		capabilityCache: &capabilityCache{},
	}

	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Start the powchain service's main event loop.
func (s *Service) Start() {
	if err := s.setupExecutionClientConnections(s.ctx, s.cfg.currHttpEndpoint); err != nil {
		log.WithError(err).Error("Could not connect to execution endpoint")
	}

	v, err := s.verifierWaiter.WaitForInitializer(s.ctx)
	if err != nil {
		log.WithError(err).Error("Could not get verification initializer")
		return
	}
	s.blobVerifier = newBlobVerifierFromInitializer(v)

	s.isRunning = true

	// Poll the execution client connection and fallback if errors occur.
	s.pollConnectionStatus(s.ctx)

	go s.run(s.ctx.Done())
}

// Stop the web3 service's main event loop and associated goroutines.
func (s *Service) Stop() error {
	if s.cancel != nil {
		defer s.cancel()
	}
	if s.rpcClient != nil {
		s.rpcClient.Close()
	}
	return nil
}

// Status is service health checks. Return nil or error.
func (s *Service) Status() error {
	// Service don't start
	if !s.isRunning {
		return nil
	}
	// get error from run function
	return s.runError
}

// ExecutionClientConnected checks whether are connected via RPC.
func (s *Service) ExecutionClientConnected() bool {
	return s.connectedETH1
}

// ExecutionClientEndpoint returns the URL of the current, connected execution client.
func (s *Service) ExecutionClientEndpoint() string {
	return s.cfg.currHttpEndpoint.Url
}

// ExecutionClientConnectionErr returns the error (if any) of the current connection.
func (s *Service) ExecutionClientConnectionErr() error {
	return s.runError
}

func (s *Service) updateBeaconNodeStats() {
	bs := clientstats.BeaconNodeStats{}
	if s.ExecutionClientConnected() {
		bs.SyncEth1Connected = true
	}
	s.cfg.beaconNodeStatsUpdater.Update(bs)
}

func (s *Service) updateConnectedETH1(state bool) {
	s.connectedETH1 = state
	s.updateBeaconNodeStats()
}

// GraffitiInfo returns the GraffitiInfo struct for graffiti generation.
func (s *Service) GraffitiInfo() *GraffitiInfo {
	return s.graffitiInfo
}

// updateGraffitiInfo fetches EL client version and updates the graffiti info.
func (s *Service) updateGraffitiInfo() {
	if s.graffitiInfo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, time.Second)
	defer cancel()
	versions, err := s.GetClientVersionV1(ctx)
	if err != nil {
		log.WithError(err).Debug("Could not get execution client version for graffiti")
		return
	}
	if len(versions) >= 1 {
		s.graffitiInfo.UpdateFromEngine(versions[0].Code, versions[0].Commit)
	}
}

// processBlockHeader adds a newly observed eth1 block to the block cache and
// updates the latest blockHeight, blockHash, and blockTime properties of the service.
func (s *Service) processBlockHeader(header *types.HeaderInfo) {
	defer safelyHandlePanic()
	blockNumberGauge.Set(float64(header.Number.Int64()))
	s.latestEth1DataLock.Lock()
	s.latestEth1Data.BlockHeight = header.Number.Uint64()
	s.latestEth1Data.BlockHash = header.Hash.Bytes()
	s.latestEth1Data.BlockTime = header.Time
	s.latestEth1DataLock.Unlock()
	log.WithFields(logrus.Fields{
		"blockNumber": s.latestEth1Data.BlockHeight,
		"blockHash":   hexutil.Encode(s.latestEth1Data.BlockHash),
	}).Debug("Latest eth1 chain event")
}

// safelyHandleHeader will recover and log any panic that occurs from the block
func safelyHandlePanic() {
	if r := recover(); r != nil {
		log.WithFields(logrus.Fields{
			"r": r,
		}).Error("Panicked when handling data from ETH 1.0 Chain! Recovering...")

		debug.PrintStack()
	}
}

// initPOWService seeds the latest eth1 header so that block lookups have an
// upper bound to search from, retrying until the execution client responds.
func (s *Service) initPOWService() {
	// Use a custom logger to only log errors
	logCounter := 0
	errorLogger := func(err error, msg string) {
		if logCounter > logThreshold {
			log.WithError(err).Error(msg)
			logCounter = 0
		}
		logCounter++
	}

	// Run in a select loop to retry in the event of any failures.
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			ctx := s.ctx
			header, err := s.HeaderByNumber(ctx, nil)
			if err != nil {
				err = errors.Wrap(err, "HeaderByNumber")
				s.retryExecutionClientConnection(ctx, err)
				errorLogger(err, "Unable to retrieve latest execution client header")
				continue
			}

			s.latestEth1DataLock.Lock()
			s.latestEth1Data.BlockHeight = header.Number.Uint64()
			s.latestEth1Data.BlockHash = header.Hash.Bytes()
			s.latestEth1Data.BlockTime = header.Time
			s.latestEth1DataLock.Unlock()

			return
		}
	}
}

// run subscribes to all the services for the eth1 chain.
func (s *Service) run(done <-chan struct{}) {
	s.runError = nil

	s.initPOWService()

	// Update graffiti info 4 times per epoch (~96 seconds with 12s slots and 32 slots/epoch)
	graffitiTicker := time.NewTicker(96 * time.Second)
	defer graffitiTicker.Stop()
	// Initial update
	s.updateGraffitiInfo()

	for {
		select {
		case <-done:
			s.isRunning = false
			s.runError = nil
			s.rpcClient.Close()
			s.updateConnectedETH1(false)
			log.Debug("Context closed, exiting goroutine")
			return
		case <-s.eth1HeadTicker.C:
			head, err := s.HeaderByNumber(s.ctx, nil)
			if err != nil {
				s.pollConnectionStatus(s.ctx)
				log.WithError(err).Debug("Could not fetch latest eth1 header")
				continue
			}
			s.processBlockHeader(head)
		case <-graffitiTicker.C:
			s.updateGraffitiInfo()
		}
	}
}

func newBlobVerifierFromInitializer(ini *verification.Initializer) verification.NewBlobVerifier {
	return func(b blocks.ROBlob, reqs []verification.Requirement) verification.BlobVerifier {
		return ini.NewBlobVerifier(b, reqs)
	}
}

type capabilityCache struct {
	capabilities     map[string]any
	capabilitiesLock sync.RWMutex
}

func (c *capabilityCache) save(cs []string) {
	c.capabilitiesLock.Lock()
	defer c.capabilitiesLock.Unlock()

	if c.capabilities == nil {
		c.capabilities = make(map[string]any)
	}

	for _, capability := range cs {
		c.capabilities[capability] = struct{}{}
	}
}

func (c *capabilityCache) has(capability string) bool {
	c.capabilitiesLock.RLock()
	defer c.capabilitiesLock.RUnlock()

	if c.capabilities == nil {
		return false
	}

	_, ok := c.capabilities[capability]
	return ok
}
