package client

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/dgraph-io/ristretto"
	middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	grpcopentracing "github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/pkg/errors"
	grpcutil "github.com/prysmaticlabs/prysm/v5/api/grpc"
	"github.com/prysmaticlabs/prysm/v5/async/event"
	lruwrpr "github.com/prysmaticlabs/prysm/v5/cache/lru"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/accounts/wallet"
	beaconApi "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api"
	beaconChainClientFactory "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-chain-client-factory"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	nodeclientfactory "github.com/prysmaticlabs/prysm/v5/validator/client/node-client-factory"
	validatorclientfactory "github.com/prysmaticlabs/prysm/v5/validator/client/validator-client-factory"
	"github.com/prysmaticlabs/prysm/v5/validator/db"
	"github.com/prysmaticlabs/prysm/v5/validator/graffiti"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/local"
	remoteweb3signer "github.com/prysmaticlabs/prysm/v5/validator/keymanager/remote-web3signer"
	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// ValidatorService represents a service to manage the validator client
// routine.
type ValidatorService struct {
	ctx                     context.Context
	cancel                  context.CancelFunc
	validator               iface.Validator
	db                      db.Database
	conn                    validatorHelpers.NodeConnection
	wallet                  *wallet.Wallet
	walletInitializedFeed   *event.Feed
	graffiti                []byte
	graffitiStruct          *graffiti.Graffiti
	interopKeysConfig       *local.InteropKeymanagerConfig
	web3SignerConfig        *remoteweb3signer.SetupConfig
	proposerSettings        *proposer.Settings
	validatorsRegBatchSize  int
	useWeb                  bool
	emitAccountMetrics      bool
	logValidatorPerformance bool
	distributed             bool
}

// Config for the validator service.
type Config struct {
	Validator               iface.Validator
	DB                      db.Database
	Wallet                  *wallet.Wallet
	WalletInitializedFeed   *event.Feed
	GRPCMaxCallRecvMsgSize  int
	GRPCRetries             uint
	GRPCRetryDelay          time.Duration
	GRPCHeaders             []string
	BeaconNodeGRPCEndpoint  string
	BeaconNodeCert          string
	BeaconApiEndpoint       string
	BeaconApiTimeout        time.Duration
	Graffiti                string
	GraffitiStruct          *graffiti.Graffiti
	InteropKmConfig         *local.InteropKeymanagerConfig
	Web3SignerConfig        *remoteweb3signer.SetupConfig
	ProposerSettings        *proposer.Settings
	ValidatorsRegBatchSize  int
	UseWeb                  bool
	LogValidatorPerformance bool
	EmitAccountMetrics      bool
	Distributed             bool
}

// NewValidatorService creates a new validator service for the service
// registry.
func NewValidatorService(ctx context.Context, cfg *Config) (*ValidatorService, error) {
	ctx, cancel := context.WithCancel(ctx)
	s := &ValidatorService{
		ctx:                     ctx,
		cancel:                  cancel,
		validator:               cfg.Validator,
		db:                      cfg.DB,
		wallet:                  cfg.Wallet,
		walletInitializedFeed:   cfg.WalletInitializedFeed,
		graffiti:                []byte(cfg.Graffiti),
		graffitiStruct:          cfg.GraffitiStruct,
		interopKeysConfig:       cfg.InteropKmConfig,
		web3SignerConfig:        cfg.Web3SignerConfig,
		proposerSettings:        cfg.ProposerSettings,
		validatorsRegBatchSize:  cfg.ValidatorsRegBatchSize,
		useWeb:                  cfg.UseWeb,
		emitAccountMetrics:      cfg.EmitAccountMetrics,
		logValidatorPerformance: cfg.LogValidatorPerformance,
		distributed:             cfg.Distributed,
	}

	dialOpts := ConstructDialOptions(
		cfg.GRPCMaxCallRecvMsgSize,
		cfg.BeaconNodeCert,
		cfg.GRPCRetries,
		cfg.GRPCRetryDelay,
	)
	if dialOpts == nil {
		return s, nil
	}

	s.ctx = grpcutil.AppendHeaders(ctx, cfg.GRPCHeaders)

	grpcConn, err := grpc.DialContext(ctx, cfg.BeaconNodeGRPCEndpoint, dialOpts...)
	if err != nil {
		return s, err
	}
	if cfg.BeaconNodeCert != "" {
		log.Info("Established secure gRPC connection")
	}
	s.conn = validatorHelpers.NewNodeConnection(
		grpcConn,
		cfg.BeaconApiEndpoint,
		cfg.BeaconApiTimeout,
	)

	return s, nil
}

// Start the validator service. Launches the main go routine for the validator
// client.
func (v *ValidatorService) Start() {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1920, // number of keys to track.
		MaxCost:     192,  // maximum cost of cache, 1 item = 1 cost.
		BufferItems: 64,   // number of keys per Get buffer.
	})
	if err != nil {
		panic(err)
	}

	aggregatedSlotCommitteeIDCache := lruwrpr.New(int(params.BeaconConfig().MaxCommitteesPerSlot))

	sPubKeys, err := v.db.EIPImportBlacklistedPublicKeys(v.ctx)
	if err != nil {
		log.WithError(err).Error("Could not read slashable public keys from disk")
		return
	}
	slashablePublicKeys := make(map[[fieldparams.BLSPubkeyLength]byte]bool)
	for _, pubKey := range sPubKeys {
		slashablePublicKeys[pubKey] = true
	}

	graffitiOrderedIndex, err := v.db.GraffitiOrderedIndex(v.ctx, v.graffitiStruct.Hash)
	if err != nil {
		log.WithError(err).Error("Could not read graffiti ordered index from disk")
		return
	}

	u := strings.ReplaceAll(v.conn.GetBeaconApiUrl(), " ", "")
	hosts := strings.Split(u, ",")
	if len(hosts) == 0 {
		log.WithError(err).Error("No API hosts provided")
		return
	}
	restHandler := beaconApi.NewBeaconApiJsonRestHandler(
		http.Client{Timeout: v.conn.GetBeaconApiTimeout()},
		hosts[0],
	)

	validatorClient := validatorclientfactory.NewValidatorClient(v.conn, restHandler)

	valStruct := &validator{
		slotFeed:                       new(event.Feed),
		startBalances:                  make(map[[fieldparams.BLSPubkeyLength]byte]uint64),
		prevEpochBalances:              make(map[[fieldparams.BLSPubkeyLength]byte]uint64),
		blacklistedPubkeys:             slashablePublicKeys,
		pubkeyToStatus:                 make(map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus),
		wallet:                         v.wallet,
		walletInitializedChan:          make(chan *wallet.Wallet, 1),
		walletInitializedFeed:          v.walletInitializedFeed,
		graffiti:                       v.graffiti,
		graffitiStruct:                 v.graffitiStruct,
		graffitiOrderedIndex:           graffitiOrderedIndex,
		beaconNodeHosts:                hosts,
		currentHostIndex:               0,
		validatorClient:                validatorClient,
		chainClient:                    beaconChainClientFactory.NewChainClient(v.conn, restHandler),
		nodeClient:                     nodeclientfactory.NewNodeClient(v.conn, restHandler),
		prysmChainClient:               beaconChainClientFactory.NewPrysmChainClient(v.conn, restHandler),
		db:                             v.db,
		km:                             nil,
		web3SignerConfig:               v.web3SignerConfig,
		proposerSettings:               v.proposerSettings,
		signedValidatorRegistrations:   make(map[[fieldparams.BLSPubkeyLength]byte]*ethpb.SignedValidatorRegistrationV1),
		validatorsRegBatchSize:         v.validatorsRegBatchSize,
		interopKeysConfig:              v.interopKeysConfig,
		attSelections:                  make(map[attSelectionKey]iface.BeaconCommitteeSelection),
		aggregatedSlotCommitteeIDCache: aggregatedSlotCommitteeIDCache,
		domainDataCache:                cache,
		voteStats:                      voteStats{startEpoch: primitives.Epoch(^uint64(0))},
		syncCommitteeStats:             syncCommitteeStats{},
		submittedAtts:                  make(map[submittedAttKey]*submittedAtt),
		submittedAggregates:            make(map[submittedAttKey]*submittedAtt),
		logValidatorPerformance:        v.logValidatorPerformance,
		emitAccountMetrics:             v.emitAccountMetrics,
		useWeb:                         v.useWeb,
		distributed:                    v.distributed,
	}

	v.validator = valStruct
	go run(v.ctx, v.validator)
}

// Stop the validator service.
func (v *ValidatorService) Stop() error {
	v.cancel()
	log.Info("Stopping service")
	if v.conn != nil {
		return v.conn.GetGrpcClientConn().Close()
	}
	return nil
}

// Status of the validator service.
func (v *ValidatorService) Status() error {
	if v.conn == nil {
		return errors.New("no connection to beacon RPC")
	}
	return nil
}

// InteropKeysConfig returns the useInteropKeys flag.
func (v *ValidatorService) InteropKeysConfig() *local.InteropKeymanagerConfig {
	return v.interopKeysConfig
}

// Keymanager returns the underlying keymanager in the validator
func (v *ValidatorService) Keymanager() (keymanager.IKeymanager, error) {
	return v.validator.Keymanager()
}

// RemoteSignerConfig returns the web3signer configuration
func (v *ValidatorService) RemoteSignerConfig() *remoteweb3signer.SetupConfig {
	return v.web3SignerConfig
}

// ProposerSettings returns a deep copy of the underlying proposer settings in the validator
func (v *ValidatorService) ProposerSettings() *proposer.Settings {
	settings := v.validator.ProposerSettings()
	if settings != nil {
		return settings.Clone()
	}
	return nil
}

// SetProposerSettings sets the proposer settings on the validator service as well as the underlying validator
func (v *ValidatorService) SetProposerSettings(ctx context.Context, settings *proposer.Settings) error {
	// validator service proposer settings is only used for pass through from node -> validator service -> validator.
	// in memory use of proposer settings happens on validator.
	v.proposerSettings = settings

	// passes settings down to be updated in database and saved in memory.
	// updates to validator proposer settings will be in the validator object and not validator service.
	return v.validator.SetProposerSettings(ctx, settings)
}

// ConstructDialOptions constructs a list of grpc dial options
func ConstructDialOptions(
	maxCallRecvMsgSize int,
	withCert string,
	grpcRetries uint,
	grpcRetryDelay time.Duration,
	extraOpts ...grpc.DialOption,
) []grpc.DialOption {
	var transportSecurity grpc.DialOption
	if withCert != "" {
		creds, err := credentials.NewClientTLSFromFile(withCert, "")
		if err != nil {
			log.WithError(err).Error("Could not get valid credentials")
			return nil
		}
		transportSecurity = grpc.WithTransportCredentials(creds)
	} else {
		transportSecurity = grpc.WithInsecure()
		log.Warn("You are using an insecure gRPC connection. If you are running your beacon node and " +
			"validator on the same machines, you can ignore this message. If you want to know " +
			"how to enable secure connections, see: https://docs.prylabs.network/docs/prysm-usage/secure-grpc")
	}

	if maxCallRecvMsgSize == 0 {
		maxCallRecvMsgSize = 10 * 5 << 20 // Default 50Mb
	}

	dialOpts := []grpc.DialOption{
		transportSecurity,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxCallRecvMsgSize),
			grpcretry.WithMax(grpcRetries),
			grpcretry.WithBackoff(grpcretry.BackoffLinear(grpcRetryDelay)),
		),
		grpc.WithStatsHandler(&ocgrpc.ClientHandler{}),
		grpc.WithUnaryInterceptor(middleware.ChainUnaryClient(
			grpcopentracing.UnaryClientInterceptor(),
			grpcprometheus.UnaryClientInterceptor,
			grpcretry.UnaryClientInterceptor(),
			grpcutil.LogRequests,
		)),
		grpc.WithChainStreamInterceptor(
			grpcutil.LogStream,
			grpcopentracing.StreamClientInterceptor(),
			grpcprometheus.StreamClientInterceptor,
			grpcretry.StreamClientInterceptor(),
		),
		grpc.WithResolvers(&multipleEndpointsGrpcResolverBuilder{}),
	}

	dialOpts = append(dialOpts, extraOpts...)
	return dialOpts
}

func (v *ValidatorService) Graffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error) {
	if v.validator == nil {
		return nil, errors.New("validator is unavailable")
	}
	return v.validator.Graffiti(ctx, pubKey)
}

func (v *ValidatorService) SetGraffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, graffiti []byte) error {
	if v.validator == nil {
		return errors.New("validator is unavailable")
	}
	return v.validator.SetGraffiti(ctx, pubKey, graffiti)
}

func (v *ValidatorService) DeleteGraffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) error {
	if v.validator == nil {
		return errors.New("validator is unavailable")
	}
	return v.validator.DeleteGraffiti(ctx, pubKey)
}
