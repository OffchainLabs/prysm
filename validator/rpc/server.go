package rpc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/httprest"
	"github.com/OffchainLabs/prysm/v6/api/server/middleware"
	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/io/logs"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/validator/accounts/wallet"
	"github.com/OffchainLabs/prysm/v6/validator/client"
	iface "github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/OffchainLabs/prysm/v6/validator/db"
	"github.com/OffchainLabs/prysm/v6/validator/web"
	"github.com/pkg/errors"
)

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
	BeaconApiTimeout       time.Duration
	BeaconNodeCert         string
	DB                     db.Database
	Wallet                 *wallet.Wallet
	WalletDir              string
	WalletInitializedFeed  *event.Feed
	ValidatorService       *client.ValidatorService
	AuthTokenPath          string
	Middlewares            []middleware.Middleware
	Router                 *http.ServeMux
}

// Server defining a HTTP server for the remote signer API and registering clients
type Server struct {
	walletInitialized         bool
	logStreamerBufferSize     int
	grpcMaxCallRecvMsgSize    int
	walletInitializedFeed     *event.Feed
	beaconApiTimeout          time.Duration
	wallet                    *wallet.Wallet
	validatorService          *client.ValidatorService
	httpPort                  int
	cancel                    context.CancelFunc
	grpcRetries               uint
	grpcRetryDelay            time.Duration
	server                    *httprest.Server
	router                    *http.ServeMux
	authTokenPath             string
	beaconNodeCert            string
	beaconApiEndpoint         string
	beaconNodeEndpoint        string
	healthClient              ethpb.HealthClient
	nodeClient                iface.NodeClient
	chainClient               iface.ChainClient
	beaconNodeValidatorClient iface.ValidatorClient
	httpHost                  string
	authToken                 string
	db                        db.Database
	logStreamer               logs.Streamer
	startFailure              error
	ctx                       context.Context
	walletDir                 string
	jwtSecret                 []byte
	grpcHeaders               []string
}

// NewServer instantiates a new HTTP server.
func NewServer(ctx context.Context, cfg *Config) *Server {
	ctx, cancel := context.WithCancel(ctx)
	server := &Server{
		ctx:                    ctx,
		cancel:                 cancel,
		logStreamer:            logs.NewStreamServer(),
		logStreamerBufferSize:  1000, // Enough to handle most bursts of logs in the validator client.
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
		walletInitializedFeed:  cfg.WalletInitializedFeed,
		walletInitialized:      cfg.Wallet != nil,
		wallet:                 cfg.Wallet,
		beaconApiTimeout:       cfg.BeaconApiTimeout,
		beaconApiEndpoint:      cfg.BeaconApiEndpoint,
		beaconNodeEndpoint:     cfg.BeaconNodeGRPCEndpoint,
		router:                 cfg.Router,
	}

	if server.authTokenPath == "" && server.walletDir != "" {
		// if a wallet dir is passed without an auth token then override the default with the wallet dir
		server.authTokenPath = filepath.Join(server.walletDir, api.AuthTokenFileName)
	}

	if server.authTokenPath != "" {
		if err := server.initializeAuthToken(); err != nil {
			log.WithError(err).Error("Could not initialize web auth token")
		}
		validatorWebAddr := fmt.Sprintf("%s:%d", server.httpHost, server.httpPort)
		logValidatorWebAuth(validatorWebAddr, server.authToken, server.authTokenPath)
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

	if err := server.InitializeRoutesWithWebHandler(); err != nil {
		log.WithError(err).Fatal("Could not initialize routes with web handler")
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

// InitializeRoutesWithWebHandler adds a catchall wrapper for web handling
func (s *Server) InitializeRoutesWithWebHandler() error {
	if err := s.InitializeRoutes(); err != nil {
		return err
	}
	s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			r.URL.Path = strings.Replace(r.URL.Path, "/api", "", 1) // used to redirect apis to standard rest APIs
			s.router.ServeHTTP(w, r)
			return
		}
		if features.Get().EnableWeb {
			web.Handler(w, r)
		}
	})
	return nil
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

	// auth endpoint
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"initialize", s.Initialize)
	// accounts endpoints
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"accounts", s.ListAccounts)
	s.router.HandleFunc("POST "+api.WebUrlPrefix+"accounts/backup", s.BackupAccounts)
	s.router.HandleFunc("POST "+api.WebUrlPrefix+"accounts/voluntary-exit", s.VoluntaryExit)
	// web health endpoints
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"health/version", s.GetVersion)
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"health/logs/validator/stream", s.StreamValidatorLogs)
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"health/logs/beacon/stream", s.StreamBeaconLogs)
	// Beacon calls
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"beacon/status", s.GetBeaconStatus)
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"beacon/summary", s.GetValidatorPerformance)
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"beacon/validators", s.GetValidators)
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"beacon/balances", s.GetValidatorBalances)
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"beacon/peers", s.GetPeers)
	// web wallet endpoints
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"wallet", s.WalletConfig)
	s.router.HandleFunc("POST "+api.WebUrlPrefix+"wallet/create", s.CreateWallet)
	s.router.HandleFunc("POST "+api.WebUrlPrefix+"wallet/keystores/validate", s.ValidateKeystores)
	s.router.HandleFunc("POST "+api.WebUrlPrefix+"wallet/recover", s.RecoverWallet)
	// slashing protection endpoints
	s.router.HandleFunc("GET "+api.WebUrlPrefix+"slashing-protection/export", s.ExportSlashingProtection)
	s.router.HandleFunc("POST "+api.WebUrlPrefix+"slashing-protection/import", s.ImportSlashingProtection)

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
