// Package beacon-chain defines the entire runtime of an Ethereum beacon node.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	runtimeDebug "runtime/debug"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/node"
	"github.com/OffchainLabs/prysm/v6/cmd"
	blockchaincmd "github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/blockchain"
	dbcommands "github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/genesis"
	jwtcommands "github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/jwt"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/storage"
	backfill "github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/sync/backfill"
	bflags "github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/sync/backfill/flags"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/sync/checkpoint"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/OffchainLabs/prysm/v6/io/logs"
	"github.com/OffchainLabs/prysm/v6/monitoring/journald"
	"github.com/OffchainLabs/prysm/v6/runtime/debug"
	"github.com/OffchainLabs/prysm/v6/runtime/fdlimits"
	prefixed "github.com/OffchainLabs/prysm/v6/runtime/logging/logrus-prefixed-formatter"
	_ "github.com/OffchainLabs/prysm/v6/runtime/maxprocs"
	"github.com/OffchainLabs/prysm/v6/runtime/tos"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	gethlog "github.com/ethereum/go-ethereum/log"
	golog "github.com/ipfs/go-log/v2"
	joonix "github.com/joonix/log"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var appFlags = []cli.Flag{
	flags.DepositContractFlag,
	flags.ExecutionEngineEndpoint,
	flags.ExecutionEngineHeaders,
	flags.ExecutionJWTSecretFlag,
	flags.RPCHost,
	flags.RPCPort,
	flags.CertFlag,
	flags.KeyFlag,
	flags.HTTPModules,
	flags.HTTPServerHost,
	flags.HTTPServerPort,
	flags.HTTPServerCorsDomain,
	flags.MinSyncPeers,
	flags.ContractDeploymentBlock,
	flags.SetGCPercent,
	flags.BlockBatchLimit,
	flags.BlockBatchLimitBurstFactor,
	flags.BlobBatchLimit,
	flags.BlobBatchLimitBurstFactor,
	flags.DataColumnBatchLimit,
	flags.DataColumnBatchLimitBurstFactor,
	flags.InteropMockEth1DataVotesFlag,
	flags.SlotsPerArchivedPoint,
	flags.DisableDebugRPCEndpoints,
	flags.SubscribeToAllSubnets,
	flags.SubscribeAllDataSubnets,
	flags.HistoricalSlasherNode,
	flags.ChainID,
	flags.NetworkID,
	flags.WeakSubjectivityCheckpoint,
	flags.Eth1HeaderReqLimit,
	flags.MinPeersPerSubnet,
	flags.MaxConcurrentDials,
	flags.SuggestedFeeRecipient,
	flags.TerminalTotalDifficultyOverride,
	flags.TerminalBlockHashOverride,
	flags.TerminalBlockHashActivationEpochOverride,
	flags.MevRelayEndpoint,
	flags.MaxBuilderEpochMissedSlots,
	flags.MaxBuilderConsecutiveMissedSlots,
	flags.EngineEndpointTimeoutSeconds,
	flags.LocalBlockValueBoost,
	flags.MinBuilderBid,
	flags.MinBuilderDiff,
	flags.BeaconDBPruning,
	flags.PrunerRetentionEpochs,
	flags.EnableBuilderSSZ,
	cmd.MinimalConfigFlag,
	cmd.E2EConfigFlag,
	cmd.RPCMaxPageSizeFlag,
	cmd.BootstrapNode,
	cmd.NoDiscovery,
	cmd.StaticPeers,
	cmd.RelayNode,
	cmd.P2PUDPPort,
	cmd.P2PQUICPort,
	cmd.P2PTCPPort,
	cmd.P2PIP,
	cmd.P2PHost,
	cmd.P2PHostDNS,
	cmd.P2PMaxPeers,
	cmd.P2PPrivKey,
	cmd.P2PStaticID,
	cmd.P2PAllowList,
	cmd.P2PDenyList,
	cmd.P2PColocationWhitelist,
	cmd.PubsubQueueSize,
	cmd.DataDirFlag,
	cmd.VerbosityFlag,
	cmd.EnableTracingFlag,
	cmd.TracingProcessNameFlag,
	cmd.TracingEndpointFlag,
	cmd.TraceSampleFractionFlag,
	cmd.MonitoringHostFlag,
	flags.MonitoringPortFlag,
	cmd.DisableMonitoringFlag,
	cmd.ClearDB,
	cmd.ForceClearDB,
	cmd.LogFormat,
	cmd.MaxGoroutines,
	debug.PProfFlag,
	debug.PProfAddrFlag,
	debug.PProfPortFlag,
	debug.MemProfileRateFlag,
	debug.BlockProfileRateFlag,
	debug.MutexProfileFractionFlag,
	cmd.LogFileName,
	cmd.EnableUPnPFlag,
	cmd.ConfigFileFlag,
	cmd.ChainConfigFileFlag,
	cmd.GrpcMaxCallRecvMsgSizeFlag,
	cmd.AcceptTosFlag,
	cmd.RestoreSourceFileFlag,
	cmd.RestoreTargetDirFlag,
	cmd.ValidatorMonitorIndicesFlag,
	cmd.ApiTimeoutFlag,
	checkpoint.BlockPath,
	checkpoint.StatePath,
	checkpoint.RemoteURL,
	genesis.StatePath,
	genesis.BeaconAPIURL,
	flags.SlasherDirFlag,
	flags.SlasherFlag,
	flags.JwtId,
	storage.BlobStoragePathFlag,
	storage.DataColumnStoragePathFlag,
	storage.BlobRetentionEpochFlag,
	storage.BlobStorageLayout,
	bflags.EnableExperimentalBackfill,
	bflags.BackfillBatchSize,
	bflags.BackfillWorkerCount,
	bflags.BackfillOldestSlot,
	flags.BatchVerifierLimit,
}

func init() {
	appFlags = cmd.WrapFlags(append(appFlags, features.BeaconChainFlags...))
}

func before(ctx *cli.Context) error {
	// Load flags from config file, if specified.
	if err := cmd.LoadFlagsFromConfig(ctx, appFlags); err != nil {
		return errors.Wrap(err, "failed to load flags from config file")
	}

	format := ctx.String(cmd.LogFormat.Name)

	switch format {
	case "text":
		formatter := new(prefixed.TextFormatter)
		formatter.TimestampFormat = "2006-01-02 15:04:05.00"
		formatter.FullTimestamp = true

		// If persistent log files are written - we disable the log messages coloring because
		// the colors are ANSI codes and seen as gibberish in the log files.
		formatter.DisableColors = ctx.String(cmd.LogFileName.Name) != ""
		logrus.SetFormatter(formatter)
	case "fluentd":
		f := joonix.NewFormatter()

		if err := joonix.DisableTimestampFormat(f); err != nil {
			panic(err) // lint:nopanic -- This shouldn't happen, but crashing immediately at startup is OK.
		}

		logrus.SetFormatter(f)
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05.00",
		})
	case "journald":
		if err := journald.Enable(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown log format %s", format)
	}

	logFileName := ctx.String(cmd.LogFileName.Name)
	if logFileName != "" {
		if err := logs.ConfigurePersistentLogging(logFileName); err != nil {
			log.WithError(err).Error("Failed to configuring logging to disk.")
		}
	}

	if err := cmd.ExpandSingleEndpointIfFile(ctx, flags.ExecutionEngineEndpoint); err != nil {
		return errors.Wrap(err, "failed to expand single endpoint")
	}

	if ctx.IsSet(flags.SetGCPercent.Name) {
		runtimeDebug.SetGCPercent(ctx.Int(flags.SetGCPercent.Name))
	}

	if err := debug.Setup(ctx); err != nil {
		return errors.Wrap(err, "failed to setup debug")
	}

	if err := fdlimits.SetMaxFdLimits(); err != nil {
		return errors.Wrap(err, "failed to set max fd limits")
	}

	if err := features.ValidateNetworkFlags(ctx); err != nil {
		return errors.Wrap(err, "provided multiple network flags")
	}

	return cmd.ValidateNoArgs(ctx)
}

func main() {
	// rctx = root context with cancellation.
	// note other instances of ctx in this func are *cli.Context.
	rctx, cancel := context.WithCancel(context.Background())
	app := cli.App{
		Name:  "beacon-chain",
		Usage: "this is a beacon chain implementation for Ethereum",
		Action: func(ctx *cli.Context) error {
			if err := startNode(ctx, cancel); err != nil {
				log.Fatal(err.Error())
				return err
			}
			return nil
		},
		Version: version.Version(),
		Commands: []*cli.Command{
			dbcommands.Commands,
			jwtcommands.Commands,
		},
		Flags:  appFlags,
		Before: before,
	}

	defer func() {
		if x := recover(); x != nil {
			log.Errorf("Runtime panic: %v\n%v", x, string(runtimeDebug.Stack()))
			panic(x) // lint:nopanic -- This is just resurfacing the original panic.
		}
	}()

	if err := app.RunContext(rctx, os.Args); err != nil {
		log.Error(err.Error())
	}
}

func startNode(ctx *cli.Context, cancel context.CancelFunc) error {
	// Fix data dir for Windows users.
	outdatedDataDir := filepath.Join(file.HomeDir(), "AppData", "Roaming", "Eth2")
	currentDataDir := ctx.String(cmd.DataDirFlag.Name)
	if err := cmd.FixDefaultDataDir(outdatedDataDir, currentDataDir); err != nil {
		return err
	}

	// verify if ToS accepted
	if err := tos.VerifyTosAcceptedOrPrompt(ctx); err != nil {
		return err
	}

	verbosity := ctx.String(cmd.VerbosityFlag.Name)
	level, err := logrus.ParseLevel(verbosity)
	if err != nil {
		return err
	}
	logrus.SetLevel(level)
	// Set libp2p logger to only panic logs for the info level.
	golog.SetAllLoggers(golog.LevelPanic)

	if level == logrus.DebugLevel {
		// Set libp2p logger to error logs for the debug level.
		golog.SetAllLoggers(golog.LevelError)
	}
	if level == logrus.TraceLevel {
		// libp2p specific logging.
		golog.SetAllLoggers(golog.LevelDebug)
		// Geth specific logging.
		gethlog.SetDefault(gethlog.NewLogger(gethlog.NewTerminalHandlerWithLevel(os.Stderr, gethlog.LvlTrace, true)))
	}

	blockchainFlagOpts, err := blockchaincmd.FlagOptions(ctx)
	if err != nil {
		return err
	}
	executionFlagOpts, err := execution.FlagOptions(ctx)
	if err != nil {
		return err
	}
	builderFlagOpts, err := builder.FlagOptions(ctx)
	if err != nil {
		return err
	}
	opts := []node.Option{
		node.WithBlockchainFlagOptions(blockchainFlagOpts),
		node.WithExecutionChainOptions(executionFlagOpts),
		node.WithBuilderFlagOptions(builderFlagOpts),
	}

	optFuncs := []func(*cli.Context) ([]node.Option, error){
		genesis.BeaconNodeOptions,
		checkpoint.BeaconNodeOptions,
		storage.BeaconNodeOptions,
		backfill.BeaconNodeOptions,
	}
	for _, of := range optFuncs {
		ofo, err := of(ctx)
		if err != nil {
			return err
		}
		if ofo != nil {
			opts = append(opts, ofo...)
		}
	}

	beacon, err := node.New(ctx, cancel, opts...)
	if err != nil {
		return fmt.Errorf("unable to start beacon node: %w", err)
	}
	beacon.Start()
	return nil
}
