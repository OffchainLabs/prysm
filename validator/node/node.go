// Package node is the main process which handles the lifecycle of
// the runtime services in a validator client process, gracefully shutting
// everything down upon close.
package node

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/api/server/middleware"
	"github.com/prysmaticlabs/prysm/v5/async/event"
	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/prysmaticlabs/prysm/v5/cmd/validator/flags"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/config/proposer/loader"
	"github.com/prysmaticlabs/prysm/v5/io/file"
	"github.com/prysmaticlabs/prysm/v5/monitoring/backup"
	"github.com/prysmaticlabs/prysm/v5/monitoring/prometheus"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/runtime"
	"github.com/prysmaticlabs/prysm/v5/runtime/debug"
	"github.com/prysmaticlabs/prysm/v5/runtime/prereqs"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/validator/accounts/wallet"
	"github.com/prysmaticlabs/prysm/v5/validator/client"
	"github.com/prysmaticlabs/prysm/v5/validator/db"
	"github.com/prysmaticlabs/prysm/v5/validator/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/validator/db/iface"
	"github.com/prysmaticlabs/prysm/v5/validator/db/kv"
	g "github.com/prysmaticlabs/prysm/v5/validator/graffiti"
	"github.com/prysmaticlabs/prysm/v5/validator/keymanager/local"
	remoteweb3signer "github.com/prysmaticlabs/prysm/v5/validator/keymanager/remote-web3signer"
	"github.com/prysmaticlabs/prysm/v5/validator/rpc"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// ValidatorClient defines an instance of an Ethereum validator that manages
// the entire lifecycle of services attached to it participating in proof of stake.
type ValidatorClient struct {
	cliCtx                *cli.Context
	ctx                   context.Context
	cancel                context.CancelFunc
	db                    iface.ValidatorDB
	services              *runtime.ServiceRegistry // Lifecycle and service store.
	lock                  sync.RWMutex
	wallet                *wallet.Wallet
	walletInitializedFeed *event.Feed
	stop                  chan struct{} // Channel to wait for termination notifications.
}

// NewValidatorClient creates a new instance of the Prysm validator client.
func NewValidatorClient(cliCtx *cli.Context) (*ValidatorClient, error) {
	// TODO(#9883) - Maybe we can pass in a new validator client config instead of the cliCTX to abstract away the use of flags here .
	if err := tracing.Setup(
		cliCtx.Context,
		"validator", // service name
		cliCtx.String(cmd.TracingProcessNameFlag.Name),
		cliCtx.String(cmd.TracingEndpointFlag.Name),
		cliCtx.Float64(cmd.TraceSampleFractionFlag.Name),
		cliCtx.Bool(cmd.EnableTracingFlag.Name),
	); err != nil {
		return nil, err
	}

	verbosity := cliCtx.String(cmd.VerbosityFlag.Name)
	level, err := logrus.ParseLevel(verbosity)
	if err != nil {
		return nil, err
	}
	logrus.SetLevel(level)

	// Warn if user's platform is not supported
	prereqs.WarnIfPlatformNotSupported(cliCtx.Context)

	registry := runtime.NewServiceRegistry()
	ctx, cancel := context.WithCancel(cliCtx.Context)
	validatorClient := &ValidatorClient{
		cliCtx:                cliCtx,
		ctx:                   ctx,
		cancel:                cancel,
		services:              registry,
		walletInitializedFeed: new(event.Feed),
		stop:                  make(chan struct{}),
	}

	if err := features.ConfigureValidator(cliCtx); err != nil {
		return nil, err
	}
	if err := cmd.ConfigureValidator(cliCtx); err != nil {
		return nil, err
	}

	if cliCtx.IsSet(cmd.ChainConfigFileFlag.Name) {
		chainConfigFileName := cliCtx.String(cmd.ChainConfigFileFlag.Name)
		if err := params.LoadChainConfigFile(chainConfigFileName, nil); err != nil {
			return nil, err
		}
	}

	// initialize router used for endpoints
	router := http.NewServeMux()
	// If the --web flag is enabled to administer the validator
	// client via a web portal, we start the validator client in a different way.
	// Change Web flag name to enable keymanager API, look at merging initializeFromCLI and initializeForWeb maybe after WebUI DEPRECATED.
	if cliCtx.IsSet(flags.EnableWebFlag.Name) {
		if cliCtx.IsSet(flags.Web3SignerURLFlag.Name) || cliCtx.IsSet(flags.Web3SignerPublicValidatorKeysFlag.Name) {
			log.Warn("Remote Keymanager API enabled. Prysm web does not properly support web3signer at this time")
		}
		log.Info("Enabling web portal to manage the validator client")
		if err := validatorClient.initializeForWeb(cliCtx, router); err != nil {
			return nil, err
		}
		return validatorClient, nil
	}

	if err := validatorClient.initializeFromCLI(cliCtx, router); err != nil {
		return nil, err
	}

	return validatorClient, nil
}

// Start every service in the validator client.
func (c *ValidatorClient) Start() {
	c.lock.Lock()

	log.WithFields(logrus.Fields{
		"version": version.Version(),
	}).Info("Starting validator node")

	c.services.StartAll()

	stop := c.stop
	c.lock.Unlock()

	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)
		<-sigc
		log.Info("Got interrupt, shutting down...")
		debug.Exit(c.cliCtx) // Ensure trace and CPU profile data are flushed.
		go c.Close()
		for i := 10; i > 0; i-- {
			<-sigc
			if i > 1 {
				log.WithField("times", i-1).Info("Already shutting down, interrupt more to panic.")
			}
		}
		panic("Panic closing the validator client")
	}()

	// Wait for stop channel to be closed.
	<-stop
}

// Close handles graceful shutdown of the system.
func (c *ValidatorClient) Close() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.services.StopAll()
	log.Info("Stopping Prysm validator")
	c.cancel()
	close(c.stop)
}

// checkLegacyDatabaseLocation checks is a database exists in the specified location.
// If it does not, it checks if a database exists in the legacy location.
// If it does, it returns the legacy location.
func (c *ValidatorClient) getLegacyDatabaseLocation(
	isInteropNumValidatorsSet bool,
	isWeb3SignerURLFlagSet bool,
	dataDir string,
	dataFile string,
	walletDir string,
) (string, string, error) {
	exists, err := file.Exists(dataFile, file.Regular)
	if err != nil {
		return "", "", errors.Wrapf(err, "could not check if file exists: %s", dataFile)
	}

	if isInteropNumValidatorsSet || dataDir != cmd.DefaultDataDir() || exists || c.wallet == nil {
		return dataDir, dataFile, nil
	}

	// We look in the previous, legacy directories.
	// See https://github.com/prysmaticlabs/prysm/issues/13391
	legacyDataDir := c.wallet.AccountsDir()
	if isWeb3SignerURLFlagSet {
		legacyDataDir = walletDir
	}

	legacyDataFile := filepath.Join(legacyDataDir, kv.ProtectionDbFileName)

	legacyDataFileExists, err := file.Exists(legacyDataFile, file.Regular)
	if err != nil {
		return "", "", errors.Wrapf(err, "could not check if file exists: %s", legacyDataFile)
	}

	if legacyDataFileExists {
		log.Infof(`Database not found in the --datadir directory (%s)
		but found in the --wallet-dir directory (%s),
		which was the legacy default.
		The next time you run the validator client without a database,
		it will be created into the --datadir directory (%s).
		To silence this message, you can move the database from (%s)
		to (%s).`,
			dataDir, legacyDataDir, dataDir, legacyDataFile, dataFile)

		dataDir = legacyDataDir
		dataFile = legacyDataFile
	}

	return dataDir, dataFile, nil
}

func (c *ValidatorClient) initializeFromCLI(cliCtx *cli.Context, router *http.ServeMux) error {
	isInteropNumValidatorsSet := cliCtx.IsSet(flags.InteropNumValidators.Name)
	isWeb3SignerURLFlagSet := cliCtx.IsSet(flags.Web3SignerURLFlag.Name)

	if !isInteropNumValidatorsSet {
		// Custom Check For Web3Signer
		if isWeb3SignerURLFlagSet {
			c.wallet = wallet.NewWalletForWeb3Signer(cliCtx)
		} else {
			w, err := wallet.OpenWalletOrElseCli(cliCtx, func(cliCtx *cli.Context) (*wallet.Wallet, error) {
				return nil, wallet.ErrNoWalletFound
			})
			if err != nil {
				return errors.Wrap(err, "could not open wallet")
			}
			c.wallet = w
			// TODO(#9883) - Remove this when we have a better way to handle this.
			log.WithFields(logrus.Fields{
				"wallet":         w.AccountsDir(),
				"keymanagerKind": w.KeymanagerKind().String(),
			}).Info("Opened validator wallet")
		}
	}

	if err := c.initializeDB(cliCtx); err != nil {
		return errors.Wrapf(err, "could not initialize database")
	}

	if !cliCtx.Bool(cmd.DisableMonitoringFlag.Name) {
		if err := c.registerPrometheusService(cliCtx); err != nil {
			return err
		}
	}
	if err := c.registerValidatorService(cliCtx); err != nil {
		return err
	}
	if cliCtx.Bool(flags.EnableRPCFlag.Name) {
		if err := c.registerRPCService(router); err != nil {
			return err
		}
	}
	return nil
}

func (c *ValidatorClient) initializeForWeb(cliCtx *cli.Context, router *http.ServeMux) error {
	if cliCtx.IsSet(flags.Web3SignerURLFlag.Name) {
		// Custom Check For Web3Signer
		c.wallet = wallet.NewWalletForWeb3Signer(cliCtx)
	} else {
		// Read the wallet password file from the cli context.
		if err := setWalletPasswordFilePath(cliCtx); err != nil {
			return errors.Wrap(err, "could not read wallet password file")
		}

		// Read the wallet from the specified path.
		w, err := wallet.OpenWalletOrElseCli(cliCtx, func(cliCtx *cli.Context) (*wallet.Wallet, error) {
			return nil, nil
		})
		if err != nil {
			return errors.Wrap(err, "could not open wallet")
		}
		c.wallet = w
	}

	if err := c.initializeDB(cliCtx); err != nil {
		return errors.Wrapf(err, "could not initialize database")
	}

	if !cliCtx.Bool(cmd.DisableMonitoringFlag.Name) {
		if err := c.registerPrometheusService(cliCtx); err != nil {
			return err
		}
	}
	if err := c.registerValidatorService(cliCtx); err != nil {
		return err
	}

	if err := c.registerRPCService(router); err != nil {
		return err
	}

	host := cliCtx.String(flags.HTTPServerHost.Name)
	port := cliCtx.Int(flags.HTTPServerPort.Name)
	webAddress := fmt.Sprintf("http://%s:%d", host, port)
	log.WithField("address", webAddress).Info(
		"Starting Prysm web UI on address, open in browser to access",
	)
	return nil
}

func (c *ValidatorClient) initializeDB(cliCtx *cli.Context) error {
	fileSystemDataDir := cliCtx.String(cmd.DataDirFlag.Name)
	kvDataDir := cliCtx.String(cmd.DataDirFlag.Name)
	kvDataFile := filepath.Join(kvDataDir, kv.ProtectionDbFileName)
	walletDir := cliCtx.String(flags.WalletDirFlag.Name)
	isInteropNumValidatorsSet := cliCtx.IsSet(flags.InteropNumValidators.Name)
	isWeb3SignerURLFlagSet := cliCtx.IsSet(flags.Web3SignerURLFlag.Name)
	clearFlag := cliCtx.Bool(cmd.ClearDB.Name)
	forceClearFlag := cliCtx.Bool(cmd.ForceClearDB.Name)

	// Workaround for https://github.com/prysmaticlabs/prysm/issues/13391
	kvDataDir, _, err := c.getLegacyDatabaseLocation(
		isInteropNumValidatorsSet,
		isWeb3SignerURLFlagSet,
		kvDataDir,
		kvDataFile,
		walletDir,
	)

	if err != nil {
		return errors.Wrap(err, "could not get legacy database location")
	}

	// Check if minimal slashing protection is requested.
	isMinimalSlashingProtectionRequested := cliCtx.Bool(features.EnableMinimalSlashingProtection.Name)

	if clearFlag || forceClearFlag {
		var err error

		if isMinimalSlashingProtectionRequested {
			err = clearDB(cliCtx.Context, fileSystemDataDir, forceClearFlag, true)
		} else {
			err = clearDB(cliCtx.Context, kvDataDir, forceClearFlag, false)
			// Reset the BoltDB datadir to the requested location, so the new one is not located any more in the legacy location.
			kvDataDir = cliCtx.String(cmd.DataDirFlag.Name)
		}

		if err != nil {
			return errors.Wrap(err, "could not clear database")
		}
	}

	// Check if a minimal database exists.
	minimalDatabasePath := path.Join(fileSystemDataDir, filesystem.DatabaseDirName)
	minimalDatabaseExists, err := file.Exists(minimalDatabasePath, file.Directory)
	if err != nil {
		return errors.Wrapf(err, "could not check if minimal slashing protection database exists")
	}

	// Check if a complete database exists.
	completeDatabasePath := path.Join(kvDataDir, kv.ProtectionDbFileName)
	completeDatabaseExists, err := file.Exists(completeDatabasePath, file.Regular)
	if err != nil {
		return errors.Wrapf(err, "could not check if complete slashing protection database exists")
	}

	// If both a complete and minimal database exist, return on error.
	if completeDatabaseExists && minimalDatabaseExists {
		log.Fatalf(
			"Both complete (%s) and minimal slashing (%s) protection databases exist. Please delete one of them.",
			path.Join(kvDataDir, kv.ProtectionDbFileName),
			path.Join(fileSystemDataDir, filesystem.DatabaseDirName),
		)
		return nil
	}

	// If a minimal database exists AND complete slashing protection is requested, convert the minimal
	// database to a complete one and use the complete database.
	if !isMinimalSlashingProtectionRequested && minimalDatabaseExists {
		log.Warning("Complete slashing protection database requested, while minimal slashing protection database currently used. Converting.")

		if err := db.ConvertDatabase(cliCtx.Context, fileSystemDataDir, kvDataDir, true); err != nil {
			return errors.Wrapf(err, "could not convert minimal slashing protection database to complete slashing protection database")
		}
	}

	// If a complete database exists AND minimal slashing protection is requested, use complete database.
	useMinimalSlashingProtection := isMinimalSlashingProtectionRequested
	if isMinimalSlashingProtectionRequested && completeDatabaseExists {
		log.Warningf(`Minimal slashing protection database requested, while complete slashing protection database currently used.
		Will continue to use complete slashing protection database.
		Please convert your database by using 'validator db convert-complete-to-minimal --source-data-dir %s --target-data-dir %s'`,
			kvDataDir, fileSystemDataDir,
		)

		useMinimalSlashingProtection = false
	}

	// Create / get the database.
	var valDB iface.ValidatorDB
	if useMinimalSlashingProtection {
		log.WithField("databasePath", fileSystemDataDir).Info("Checking DB")
		valDB, err = filesystem.NewStore(fileSystemDataDir, nil)
	} else {
		log.WithField("databasePath", kvDataDir).Info("Checking DB")
		valDB, err = kv.NewKVStore(cliCtx.Context, kvDataDir, nil)
	}

	if err != nil {
		return errors.Wrap(err, "could not create validator database")
	}

	// Assign the database to the validator client.
	c.db = valDB

	// Migrate the database
	if err := valDB.RunUpMigrations(cliCtx.Context); err != nil {
		return errors.Wrap(err, "could not run database migration")
	}

	return nil
}

func (c *ValidatorClient) registerPrometheusService(cliCtx *cli.Context) error {
	var additionalHandlers []prometheus.Handler
	if cliCtx.IsSet(cmd.EnableBackupWebhookFlag.Name) {
		additionalHandlers = append(
			additionalHandlers,
			prometheus.Handler{
				Path:    "/db/backup",
				Handler: backup.Handler(c.db, cliCtx.String(cmd.BackupWebhookOutputDir.Name)),
			},
		)
	}
	service := prometheus.NewService(
		fmt.Sprintf("%s:%d", c.cliCtx.String(cmd.MonitoringHostFlag.Name), c.cliCtx.Int(flags.MonitoringPortFlag.Name)),
		c.services,
		additionalHandlers...,
	)
	logrus.AddHook(prometheus.NewLogrusCollector())
	return c.services.RegisterService(service)
}

func (c *ValidatorClient) registerValidatorService(cliCtx *cli.Context) error {
	var (
		interopKmConfig *local.InteropKeymanagerConfig
		err             error
	)

	// Configure interop.
	if c.cliCtx.IsSet(flags.InteropNumValidators.Name) {
		interopKmConfig = &local.InteropKeymanagerConfig{
			Offset:           cliCtx.Uint64(flags.InteropStartIndex.Name),
			NumValidatorKeys: cliCtx.Uint64(flags.InteropNumValidators.Name),
		}
	}

	// Configure graffiti.
	graffitiStruct := &g.Graffiti{}
	if c.cliCtx.IsSet(flags.GraffitiFileFlag.Name) {
		graffitiFilePath := c.cliCtx.String(flags.GraffitiFileFlag.Name)

		graffitiStruct, err = g.ParseGraffitiFile(graffitiFilePath)
		if err != nil {
			log.WithError(err).Warn("Could not parse graffiti file")
		}
	}

	web3signerConfig, err := Web3SignerConfig(c.cliCtx)
	if err != nil {
		return err
	}

	ps, err := proposerSettings(c.cliCtx, c.db)
	if err != nil {
		return err
	}

	validatorService, err := client.NewValidatorService(c.cliCtx.Context, &client.Config{
		DB:                      c.db,
		Wallet:                  c.wallet,
		WalletInitializedFeed:   c.walletInitializedFeed,
		GRPCMaxCallRecvMsgSize:  c.cliCtx.Int(cmd.GrpcMaxCallRecvMsgSizeFlag.Name),
		GRPCRetries:             c.cliCtx.Uint(flags.GRPCRetriesFlag.Name),
		GRPCRetryDelay:          c.cliCtx.Duration(flags.GRPCRetryDelayFlag.Name),
		GRPCHeaders:             strings.Split(c.cliCtx.String(flags.GRPCHeadersFlag.Name), ","),
		BeaconNodeGRPCEndpoint:  c.cliCtx.String(flags.BeaconRPCProviderFlag.Name),
		BeaconNodeCert:          c.cliCtx.String(flags.CertFlag.Name),
		BeaconApiEndpoint:       c.cliCtx.String(flags.BeaconRESTApiProviderFlag.Name),
		BeaconApiTimeout:        time.Second * 30,
		Graffiti:                g.ParseHexGraffiti(c.cliCtx.String(flags.GraffitiFlag.Name)),
		GraffitiStruct:          graffitiStruct,
		InteropKmConfig:         interopKmConfig,
		Web3SignerConfig:        web3signerConfig,
		ProposerSettings:        ps,
		ValidatorsRegBatchSize:  c.cliCtx.Int(flags.ValidatorsRegistrationBatchSizeFlag.Name),
		UseWeb:                  c.cliCtx.Bool(flags.EnableWebFlag.Name),
		LogValidatorPerformance: !c.cliCtx.Bool(flags.DisablePenaltyRewardLogFlag.Name),
		EmitAccountMetrics:      !c.cliCtx.Bool(flags.DisableAccountMetricsFlag.Name),
		Distributed:             c.cliCtx.Bool(flags.EnableDistributed.Name),
	})
	if err != nil {
		return errors.Wrap(err, "could not initialize validator service")
	}

	return c.services.RegisterService(validatorService)
}

func Web3SignerConfig(cliCtx *cli.Context) (*remoteweb3signer.SetupConfig, error) {
	var web3signerConfig *remoteweb3signer.SetupConfig
	if cliCtx.IsSet(flags.Web3SignerURLFlag.Name) {
		urlStr := cliCtx.String(flags.Web3SignerURLFlag.Name)
		u, err := url.ParseRequestURI(urlStr)
		if err != nil {
			return nil, errors.Wrapf(err, "web3signer url %s is invalid", urlStr)
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("web3signer url must be in the format of http(s)://host:port url used: %v", urlStr)
		}
		web3signerConfig = &remoteweb3signer.SetupConfig{
			BaseEndpoint:          u.String(),
			GenesisValidatorsRoot: nil,
		}
		if cliCtx.IsSet(flags.WalletPasswordFileFlag.Name) {
			log.Warnf("%s was provided while using web3signer and will be ignored", flags.WalletPasswordFileFlag.Name)
		}
		if cliCtx.IsSet(flags.Web3SignerPublicValidatorKeysFlag.Name) {
			publicKeysSlice := cliCtx.StringSlice(flags.Web3SignerPublicValidatorKeysFlag.Name)
			if len(publicKeysSlice) == 1 {
				pURL, err := url.ParseRequestURI(publicKeysSlice[0])
				if err == nil && pURL.Scheme != "" && pURL.Host != "" {
					web3signerConfig.PublicKeysURL = publicKeysSlice[0]
				} else {
					web3signerConfig.ProvidedPublicKeys = strings.Split(publicKeysSlice[0], ",")
				}
			} else {
				web3signerConfig.ProvidedPublicKeys = publicKeysSlice
			}
		}
		if cliCtx.IsSet(flags.Web3SignerKeyFileFlag.Name) {
			web3signerConfig.KeyFilePath = cliCtx.String(flags.Web3SignerKeyFileFlag.Name)
		}
	}
	return web3signerConfig, nil
}

func proposerSettings(cliCtx *cli.Context, db iface.ValidatorDB) (*proposer.Settings, error) {
	l, err := loader.NewProposerSettingsLoader(
		cliCtx,
		db,
		loader.WithBuilderConfig(),
		loader.WithGasLimit(),
	)
	if err != nil {
		return nil, err
	}
	return l.Load(cliCtx)
}

func (c *ValidatorClient) registerRPCService(router *http.ServeMux) error {
	var vs *client.ValidatorService
	if err := c.services.FetchService(&vs); err != nil {
		return err
	}
	authTokenPath := c.cliCtx.String(flags.AuthTokenPathFlag.Name)
	walletDir := c.cliCtx.String(flags.WalletDirFlag.Name)
	// if no auth token path flag was passed try to set a default value
	if authTokenPath == "" {
		authTokenPath = flags.AuthTokenPathFlag.Value
		// if a wallet dir is passed without an auth token then override the default with the wallet dir
		if walletDir != "" {
			authTokenPath = filepath.Join(walletDir, api.AuthTokenFileName)
		}
	}
	host := c.cliCtx.String(flags.HTTPServerHost.Name)
	if host != flags.DefaultHTTPServerHost {
		log.WithField("webHost", host).Warn(
			"You are using a non-default web host. Web traffic is served by HTTP, so be wary of " +
				"changing this parameter if you are exposing this host to the Internet!",
		)
	}
	port := c.cliCtx.Int(flags.HTTPServerPort.Name)
	var allowedOrigins []string
	if c.cliCtx.IsSet(flags.HTTPServerCorsDomain.Name) {
		allowedOrigins = strings.Split(c.cliCtx.String(flags.HTTPServerCorsDomain.Name), ",")
	} else {
		allowedOrigins = strings.Split(flags.HTTPServerCorsDomain.Value, ",")
	}

	middlewares := []middleware.Middleware{
		middleware.NormalizeQueryValuesHandler,
		middleware.CorsHandler(allowedOrigins),
	}
	s := rpc.NewServer(c.cliCtx.Context, &rpc.Config{
		HTTPHost:               host,
		HTTPPort:               port,
		GRPCMaxCallRecvMsgSize: c.cliCtx.Int(cmd.GrpcMaxCallRecvMsgSizeFlag.Name),
		GRPCRetries:            c.cliCtx.Uint(flags.GRPCRetriesFlag.Name),
		GRPCRetryDelay:         c.cliCtx.Duration(flags.GRPCRetryDelayFlag.Name),
		GRPCHeaders:            strings.Split(c.cliCtx.String(flags.GRPCHeadersFlag.Name), ","),
		BeaconNodeGRPCEndpoint: c.cliCtx.String(flags.BeaconRPCProviderFlag.Name),
		BeaconApiEndpoint:      c.cliCtx.String(flags.BeaconRESTApiProviderFlag.Name),
		BeaconApiTimeout:       time.Second * 30,
		BeaconNodeCert:         c.cliCtx.String(flags.CertFlag.Name),
		DB:                     c.db,
		Wallet:                 c.wallet,
		WalletDir:              walletDir,
		WalletInitializedFeed:  c.walletInitializedFeed,
		ValidatorService:       vs,
		AuthTokenPath:          authTokenPath,
		Middlewares:            middlewares,
		Router:                 router,
	})
	return c.services.RegisterService(s)
}

func setWalletPasswordFilePath(cliCtx *cli.Context) error {
	walletDir := cliCtx.String(flags.WalletDirFlag.Name)
	defaultWalletPasswordFilePath := filepath.Join(walletDir, wallet.DefaultWalletPasswordFile)
	exists, err := file.Exists(defaultWalletPasswordFilePath, file.Regular)
	if err != nil {
		return errors.Wrap(err, "could not check if default wallet password file exists")
	}

	if exists {
		// Ensure file has proper permissions.
		hasPerms, err := file.HasReadWritePermissions(defaultWalletPasswordFilePath)
		if err != nil {
			return err
		}
		if !hasPerms {
			return fmt.Errorf(
				"wallet password file %s does not have proper 0600 permissions",
				defaultWalletPasswordFilePath,
			)
		}

		// Set the filepath into the cli context.
		if err := cliCtx.Set(flags.WalletPasswordFileFlag.Name, defaultWalletPasswordFilePath); err != nil {
			return errors.Wrap(err, "could not set default wallet password file path")
		}
	}
	return nil
}

func clearDB(ctx context.Context, dataDir string, force bool, isDatabaseMinimal bool) error {
	var (
		valDB iface.ValidatorDB
		err   error
	)

	clearDBConfirmed := force

	if !force {
		actionText := "This will delete your validator's historical actions database stored in your data directory. " +
			"This may lead to potential slashing - do you want to proceed? (Y/N)"
		deniedText := "The historical actions database will not be deleted. No changes have been made."
		clearDBConfirmed, err = cmd.ConfirmAction(actionText, deniedText)
		if err != nil {
			return errors.Wrapf(err, "Could not clear DB in dir %s", dataDir)
		}
	}

	if clearDBConfirmed {
		if isDatabaseMinimal {
			valDB, err = filesystem.NewStore(dataDir, nil)
		} else {
			valDB, err = kv.NewKVStore(ctx, dataDir, nil)
		}

		if err != nil {
			return errors.Wrap(err, "could not create validator database")
		}

		if err := valDB.Close(); err != nil {
			return errors.Wrapf(err, "could not close DB in dir %s", dataDir)
		}

		log.Warning("Removing database")
		if err := valDB.ClearDB(); err != nil {
			return errors.Wrapf(err, "Could not clear DB in dir %s", dataDir)
		}
	}

	return nil
}
