package rpc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/httprest"
	"github.com/OffchainLabs/prysm/v7/api/server/middleware"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/wallet"
	iface "github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/OffchainLabs/prysm/v7/validator/db"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	remoteweb3signer "github.com/OffchainLabs/prysm/v7/validator/keymanager/remote-web3signer"
	"github.com/pkg/errors"
)

// ValidatorService is what the keymanager and wallet handlers need from the running validator client.
type ValidatorService interface {
	Keymanager() (keymanager.IKeymanager, error)
	RemoteSignerConfig() *remoteweb3signer.SetupConfig
	ProposerSettings() *proposer.Settings
	SetProposerSettings(ctx context.Context, settings *proposer.Settings) error
	Graffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) ([]byte, error)
	SetGraffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, graffiti []byte) error
	DeleteGraffiti(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte) error
}

// Config options for the HTTP server.
type Config struct {
	HTTPHost               string
	HTTPPort               int
	GRPCMaxCallRecvMsgSize int
	GRPCRetries            uint
	GRPCRetryDelay         time.Duration
	GRPCHeaders            []string
	BeaconNodeGRPCEndpoint string
	BeaconApiEndpoint      string
	BeaconAPIHeaders       map[string][]string
	BeaconApiTimeout       time.Duration
	BeaconNodeCert         string
	DB                     db.Database
	Wallet                 *wallet.Wallet
	WalletDir              string
	ValidatorService       ValidatorService
	AuthTokenPath          string
	Middlewares            []middleware.Middleware
	Router                 *http.ServeMux
}

// Server defining a HTTP server for the remote signer API and registering clients
type Server struct {
	walletInitialized         bool
	grpcMaxCallRecvMsgSize    int
	beaconApiTimeout          time.Duration
	wallet                    *wallet.Wallet
	validatorService          ValidatorService
	httpPort                  int
	cancel                    context.CancelFunc
	grpcRetries               uint
	grpcRetryDelay            time.Duration
	server                    *httprest.Server
	router                    *http.ServeMux
	authTokenPath             string
	beaconNodeCert            string
	beaconApiEndpoint         string
	beaconApiHeaders          map[string][]string
	beaconNodeEndpoint        string
	nodeClient                iface.NodeClient
	beaconNodeValidatorClient iface.ValidatorClient
	httpHost                  string
	authToken                 string
	db                        db.Database
	startFailure              error
	ctx                       context.Context
	walletDir                 string
	grpcHeaders               []string
}

// NewServer instantiates a new HTTP server.
func NewServer(ctx context.Context, cfg *Config) *Server {
	ctx, cancel := context.WithCancel(ctx)

	// TODO(17165): walletInitialized should be removed anyway.
	walletInitialized := cfg.Wallet != nil
	if cfg.ValidatorService != nil && cfg.ValidatorService.RemoteSignerConfig() != nil {
		// Consider the wallet initialized when remote signer is configured,
		// as this flag blocks the VC from starting up and serving requests, even though the keymanager is ready.
		walletInitialized = true
	}

	server := &Server{
		ctx:                    ctx,
		cancel:                 cancel,
		httpHost:               cfg.HTTPHost,
		httpPort:               cfg.HTTPPort,
		grpcMaxCallRecvMsgSize: cfg.GRPCMaxCallRecvMsgSize,
		grpcRetries:            cfg.GRPCRetries,
		grpcRetryDelay:         cfg.GRPCRetryDelay,
		grpcHeaders:            cfg.GRPCHeaders,
		validatorService:       cfg.ValidatorService,
		authTokenPath:          cfg.AuthTokenPath,
		db:                     cfg.DB,
		walletDir:              cfg.WalletDir,
		walletInitialized:      walletInitialized,
		wallet:                 cfg.Wallet,
		beaconApiTimeout:       cfg.BeaconApiTimeout,
		beaconApiEndpoint:      cfg.BeaconApiEndpoint,
		beaconApiHeaders:       cfg.BeaconAPIHeaders,
		beaconNodeEndpoint:     cfg.BeaconNodeGRPCEndpoint,
		router:                 cfg.Router,
	}

	if server.authTokenPath == "" && server.walletDir != "" {
		// if a wallet dir is passed without an auth token then override the default with the wallet dir
		server.authTokenPath = filepath.Join(server.walletDir, api.AuthTokenFileName)
	}

	if server.authTokenPath != "" {
		if err := server.initializeAuthToken(); err != nil {
			log.WithError(err).Error("Could not initialize auth token")
		}
		logAuthTokenPath(server.authTokenPath)
		go server.refreshAuthTokenFromFileChanges(server.ctx, server.authTokenPath)
	}

	// Register a gRPC or HTTP client to the beacon node.
	// Used for proxy calls to beacon node from validator REST handlers
	if err := server.registerBeaconClient(); err != nil {
		log.WithError(err).Fatal("Could not register beacon chain gRPC or HTTP client")
	}

	// Adding AuthTokenHandler to the list of middlewares
	cfg.Middlewares = append(cfg.Middlewares, server.AuthTokenHandler)
	opts := []httprest.Option{
		httprest.WithRouter(cfg.Router),
		httprest.WithHTTPAddr(net.JoinHostPort(server.httpHost, fmt.Sprintf("%d", server.httpPort))),
		httprest.WithMiddlewares(cfg.Middlewares),
	}

	if err := server.InitializeRoutes(); err != nil {
		log.WithError(err).Fatal("Could not initialize routes")
	}
	// create and set a new http server
	s, err := httprest.New(server.ctx, opts...)
	if err != nil {
		log.WithError(err).Fatal("Failed to create HTTP server")
	}
	server.server = s

	return server
}

// Start the HTTP server and registers clients that can communicate via HTTP or gRPC.
func (s *Server) Start() {
	s.server.Start()
}

// InitializeRoutes initializes pure HTTP REST endpoints for the validator client.
// needs to be called before the Serve function
func (s *Server) InitializeRoutes() error {
	if s.router == nil {
		return errors.New("no router found on server")
	}
	// Register all services, HandleFunc calls, etc.
	// ...
	s.router.HandleFunc("GET /eth/v1/keystores", s.ListKeystores)
	s.router.HandleFunc("POST /eth/v1/keystores", s.ImportKeystores)
	s.router.HandleFunc("DELETE /eth/v1/keystores", s.DeleteKeystores)
	s.router.HandleFunc("GET /eth/v1/remotekeys", s.ListRemoteKeys)
	s.router.HandleFunc("POST /eth/v1/remotekeys", s.ImportRemoteKeys)
	s.router.HandleFunc("DELETE /eth/v1/remotekeys", s.DeleteRemoteKeys)
	s.router.HandleFunc("GET /eth/v1/validator/{pubkey}/gas_limit", s.GetGasLimit)
	s.router.HandleFunc("POST /eth/v1/validator/{pubkey}/gas_limit", s.SetGasLimit)
	s.router.HandleFunc("DELETE /eth/v1/validator/{pubkey}/gas_limit", s.DeleteGasLimit)
	s.router.HandleFunc("GET /eth/v1/validator/{pubkey}/feerecipient", s.ListFeeRecipientByPubkey)
	s.router.HandleFunc("POST /eth/v1/validator/{pubkey}/feerecipient", s.SetFeeRecipientByPubkey)
	s.router.HandleFunc("DELETE /eth/v1/validator/{pubkey}/feerecipient", s.DeleteFeeRecipientByPubkey)
	s.router.HandleFunc("POST /eth/v1/validator/{pubkey}/voluntary_exit", s.SetVoluntaryExit)
	s.router.HandleFunc("GET /eth/v1/validator/{pubkey}/graffiti", s.GetGraffiti)
	s.router.HandleFunc("POST /eth/v1/validator/{pubkey}/graffiti", s.SetGraffiti)
	s.router.HandleFunc("DELETE /eth/v1/validator/{pubkey}/graffiti", s.DeleteGraffiti)

	log.Info("Initialized REST API routes")
	return nil
}

// Stop the HTTP server.
func (s *Server) Stop() error {
	return s.server.Stop()
}

// Status returns an error if the service is unhealthy.
func (s *Server) Status() error {
	if s.startFailure != nil {
		return s.startFailure
	}
	return nil
}

// keymanagerKind returns the kind of the configured keymanager.
// Return boolean as well as the kind to indicate whether the wallet is ready or not.
func (s *Server) keymanagerKind() (keymanager.Kind, bool) {
	// If remote signer is configured, return Web3Signer kind.
	if s.validatorService != nil && s.validatorService.RemoteSignerConfig() != nil {
		return keymanager.Web3Signer, true
	}

	// Prysm wallet is not set. This path is only reachable for Web/RPC path.
	// Alert caller with false to indicate that the wallet is not initialized.
	if s.wallet == nil {
		return 0, false
	}

	return s.wallet.KeymanagerKind(), true
}
