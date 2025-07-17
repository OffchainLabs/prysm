// Package main defines a validator client, a critical actor in Ethereum which manages
// a keystore of private keys, connects to a beacon node to receive assignments,
// and submits blocks/attestations as needed.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	runtimeDebug "runtime/debug"

	"github.com/OffchainLabs/prysm/v6/cmd"
	accountcommands "github.com/OffchainLabs/prysm/v6/cmd/validator/accounts"
	dbcommands "github.com/OffchainLabs/prysm/v6/cmd/validator/db"
	"github.com/OffchainLabs/prysm/v6/cmd/validator/flags"
	slashingprotectioncommands "github.com/OffchainLabs/prysm/v6/cmd/validator/slashing-protection"
	walletcommands "github.com/OffchainLabs/prysm/v6/cmd/validator/wallet"
	"github.com/OffchainLabs/prysm/v6/cmd/validator/web"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/OffchainLabs/prysm/v6/io/logs"
	"github.com/OffchainLabs/prysm/v6/monitoring/journald"
	"github.com/OffchainLabs/prysm/v6/runtime/debug"
	prefixed "github.com/OffchainLabs/prysm/v6/runtime/logging/logrus-prefixed-formatter"
	_ "github.com/OffchainLabs/prysm/v6/runtime/maxprocs"
	"github.com/OffchainLabs/prysm/v6/runtime/tos"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/validator/node"
	joonix "github.com/joonix/log"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var log = logrus.WithField("prefix", "main")

func startNode(ctx *cli.Context) error {
	// Verify if ToS is accepted.
	if err := tos.VerifyTosAcceptedOrPrompt(ctx); err != nil {
		return err
	}

	validatorClient, err := node.NewValidatorClient(ctx)
	if err != nil {
		return err
	}
	validatorClient.Start()
	return nil
}

var appFlags = []cli.Flag{
	flags.BeaconRPCProviderFlag,
	flags.BeaconRESTApiProviderFlag,
	flags.CertFlag,
	flags.GraffitiFlag,
	flags.DisablePenaltyRewardLogFlag,
	flags.InteropStartIndex,
	flags.InteropNumValidators,
	flags.EnableRPCFlag,
	flags.RPCHost,
	flags.RPCPort,
	flags.HTTPServerPort,
	flags.HTTPServerHost,
	flags.GRPCRetriesFlag,
	flags.GRPCRetryDelayFlag,
	flags.GRPCHeadersFlag,
	flags.HTTPServerCorsDomain,
	flags.DisableAccountMetricsFlag,
	flags.MonitoringPortFlag,
	flags.SlasherRPCProviderFlag,
	flags.SlasherCertFlag,
	flags.WalletPasswordFileFlag,
	flags.WalletDirFlag,
	flags.GraffitiFileFlag,
	flags.EnableDistributed,
	flags.AuthTokenPathFlag,
	flags.DisableDutiesPolling,
	flags.MaxHealthChecksFlag,
	// Consensys' Web3Signer flags
	flags.Web3SignerURLFlag,
	flags.Web3SignerPublicValidatorKeysFlag,
	flags.Web3SignerKeyFileFlag,
	flags.SuggestedFeeRecipientFlag,
	flags.ProposerSettingsURLFlag,
	flags.ProposerSettingsFlag,
	flags.EnableBuilderFlag,
	flags.BuilderGasLimitFlag,
	flags.ValidatorsRegistrationBatchSizeFlag,
	////////////////////
	cmd.DisableMonitoringFlag,
	cmd.MonitoringHostFlag,
	cmd.EnableBackupWebhookFlag,
	cmd.MinimalConfigFlag,
	cmd.E2EConfigFlag,
	cmd.VerbosityFlag,
	cmd.DataDirFlag,
	cmd.ClearDB,
	cmd.ForceClearDB,
	cmd.EnableTracingFlag,
	cmd.TracingProcessNameFlag,
	cmd.TracingEndpointFlag,
	cmd.TraceSampleFractionFlag,
	cmd.LogFormat,
	cmd.LogFileName,
	cmd.ConfigFileFlag,
	cmd.ChainConfigFileFlag,
	cmd.GrpcMaxCallRecvMsgSizeFlag,
	cmd.ApiTimeoutFlag,
	debug.PProfFlag,
	debug.PProfAddrFlag,
	debug.PProfPortFlag,
	debug.MemProfileRateFlag,
	debug.BlockProfileRateFlag,
	debug.MutexProfileFractionFlag,
	cmd.AcceptTosFlag,
}

func init() {
	appFlags = cmd.WrapFlags(append(appFlags, features.ValidatorFlags...))
}

func main() {
	app := cli.App{
		Name:    "validator",
		Usage:   "Launches an Ethereum validator client that interacts with a beacon chain, starts proposer and attester services, p2p connections, and more.",
		Version: version.Version(),
		Action: func(ctx *cli.Context) error {
			if err := startNode(ctx); err != nil {
				log.Fatal(err.Error())
				return err
			}
			return nil
		},
		Commands: []*cli.Command{
			walletcommands.Commands,
			accountcommands.Commands,
			slashingprotectioncommands.Commands,
			dbcommands.Commands,
			web.Commands,
		},
		Flags: appFlags,
		Before: func(ctx *cli.Context) error {
			// Load flags from config file, if specified.
			if err := cmd.LoadFlagsFromConfig(ctx, appFlags); err != nil {
				return err
			}

			logFileName := ctx.String(cmd.LogFileName.Name)

			format := ctx.String(cmd.LogFormat.Name)
			switch format {
			case "text":
				formatter := new(prefixed.TextFormatter)
				formatter.TimestampFormat = "2006-01-02 15:04:05.00"
				formatter.FullTimestamp = true
				// If persistent log files are written - we disable the log messages coloring because
				// the colors are ANSI codes and seen as gibberish in the log files.
				formatter.DisableColors = logFileName != ""
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

			if logFileName != "" {
				if err := logs.ConfigurePersistentLogging(logFileName); err != nil {
					log.WithError(err).Error("Failed to configuring logging to disk.")
				}
			}

			// Fix data dir for Windows users.
			outdatedDataDir := filepath.Join(file.HomeDir(), "AppData", "Roaming", "Eth2Validators")
			currentDataDir := flags.DefaultValidatorDir()
			if err := cmd.FixDefaultDataDir(outdatedDataDir, currentDataDir); err != nil {
				log.WithError(err).Error("Cannot update data directory")
			}

			if err := debug.Setup(ctx); err != nil {
				return errors.Wrap(err, "failed to setup debug")
			}

			if err := features.ValidateNetworkFlags(ctx); err != nil {
				return errors.Wrap(err, "provided multiple network flags")
			}

			return cmd.ValidateNoArgs(ctx)
		},
	}

	defer func() {
		if x := recover(); x != nil {
			log.Errorf("Runtime panic: %v\n%v", x, string(runtimeDebug.Stack()))
			panic(x) // lint:nopanic -- This is just resurfacing the original panic.
		}
	}()

	if err := app.Run(os.Args); err != nil {
		log.Error(err.Error())
	}
}
