// This code was adapted from https://github.com/ethereum/go-ethereum/blob/master/cmd/geth/usage.go
package main

import (
	"io"
	"sort"

	"github.com/OffchainLabs/prysm/v6/cmd"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/genesis"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/storage"
	backfill "github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/sync/backfill/flags"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/sync/checkpoint"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/runtime/debug"
	"github.com/urfave/cli/v2"
)

var appHelpTemplate = `NAME:
   {{.App.Name}} - {{.App.Usage}}
USAGE:
   {{.App.HelpName}} [options]{{if .App.Commands}} command [command options]{{end}} {{if .App.ArgsUsage}}{{.App.ArgsUsage}}{{else}}[arguments...]{{end}}
   {{if .App.Version}}
AUTHOR:
   {{range .App.Authors}}{{ . }}{{end}}
   {{end}}{{if .App.Commands}}
GLOBAL OPTIONS:
   {{range .App.Commands}}{{join .Names ", "}}{{ "\t" }}{{.Usage}}
   {{end}}{{end}}{{if .FlagGroups}}
{{range .FlagGroups}}{{.Name}} OPTIONS:
   {{range .Flags}}{{.}}
   {{end}}
{{end}}{{end}}{{if .App.Copyright }}
COPYRIGHT:
   {{.App.Copyright}}
VERSION:
   {{.App.Version}}
   {{end}}{{if len .App.Authors}}
   {{end}}
`

type flagGroup struct {
	Name  string
	Flags []cli.Flag
}

var appHelpFlagGroups = []flagGroup{
	{ // Flags relevant to running the process.
		Name: "cmd",
		Flags: []cli.Flag{
			cmd.AcceptTosFlag,
			cmd.ConfigFileFlag,
		},
	},
	{ // Flags relevant to configuring the beacon chain and APIs.
		Name: "beacon-chain",
		Flags: []cli.Flag{
			cmd.ApiTimeoutFlag,
			cmd.ChainConfigFileFlag,
			cmd.E2EConfigFlag,
			cmd.GrpcMaxCallRecvMsgSizeFlag,
			cmd.MinimalConfigFlag,
			cmd.RPCMaxPageSizeFlag,
			flags.CertFlag,
			flags.ChainID,
			flags.DisableDebugRPCEndpoints,
			flags.HTTPModules,
			flags.HTTPServerCorsDomain,
			flags.HTTPServerHost,
			flags.HTTPServerPort,
			flags.KeyFlag,
			flags.NetworkID,
			flags.RPCHost,
			flags.RPCPort,
			flags.BatchVerifierLimit,
		},
	},
	{
		// p2p flags configure the p2p side of beacon-chain.
		Name: "p2p",
		Flags: []cli.Flag{
			cmd.BootstrapNode,
			cmd.EnableUPnPFlag,
			cmd.NoDiscovery,
			cmd.P2PAllowList,
			cmd.P2PDenyList,
			cmd.P2PColocationWhitelist,
			cmd.P2PHost,
			cmd.P2PHostDNS,
			cmd.P2PIP,
			cmd.P2PMaxPeers,
			cmd.P2PPrivKey,
			cmd.P2PQUICPort,
			cmd.P2PStaticID,
			cmd.P2PTCPPort,
			cmd.P2PUDPPort,
			cmd.PubsubQueueSize,
			cmd.RelayNode,
			cmd.StaticPeers,
			flags.BlobBatchLimit,
			flags.BlobBatchLimitBurstFactor,
			flags.DataColumnBatchLimit,
			flags.DataColumnBatchLimitBurstFactor,
			flags.BlockBatchLimit,
			flags.BlockBatchLimitBurstFactor,
			flags.MaxConcurrentDials,
			flags.MinPeersPerSubnet,
			flags.MinSyncPeers,
			flags.SubscribeToAllSubnets,
			flags.SubscribeAllDataSubnets,
		},
	},
	{ // Flags relevant to storing data on disk and configuring the beacon chain database.
		Name: "db",
		Flags: []cli.Flag{
			backfill.BackfillBatchSize,
			backfill.BackfillOldestSlot,
			backfill.BackfillWorkerCount,
			backfill.EnableExperimentalBackfill,
			cmd.ClearDB,
			cmd.DataDirFlag,
			cmd.ForceClearDB,
			cmd.RestoreSourceFileFlag,
			cmd.RestoreTargetDirFlag,
			flags.BeaconDBPruning,
			flags.PrunerRetentionEpochs,
			flags.SlotsPerArchivedPoint,
			storage.BlobRetentionEpochFlag,
			storage.BlobStorageLayout,
			storage.BlobStoragePathFlag,
			storage.DataColumnStoragePathFlag,
		},
	},
	{ // Flags relevant to configuring local block production or external builders such as mev-boost.
		Name: "builder",
		Flags: []cli.Flag{
			flags.LocalBlockValueBoost,
			flags.MaxBuilderConsecutiveMissedSlots,
			flags.MaxBuilderEpochMissedSlots,
			flags.MevRelayEndpoint,
			flags.MinBuilderBid,
			flags.MinBuilderDiff,
			flags.SuggestedFeeRecipient,
			flags.EnableBuilderSSZ,
		},
	},
	{ // Flags relevant to syncing the beacon chain.
		Name: "sync",
		Flags: []cli.Flag{
			checkpoint.BlockPath,
			checkpoint.RemoteURL,
			checkpoint.StatePath,
			flags.WeakSubjectivityCheckpoint,
			genesis.BeaconAPIURL,
			genesis.StatePath,
		},
	},
	{ // Flags relevant to interacting with the execution layer.
		Name: "execution layer",
		Flags: []cli.Flag{
			flags.ContractDeploymentBlock,
			flags.DepositContractFlag,
			flags.EngineEndpointTimeoutSeconds,
			flags.Eth1HeaderReqLimit,
			flags.ExecutionEngineEndpoint,
			flags.ExecutionEngineHeaders,
			flags.ExecutionJWTSecretFlag,
			flags.JwtId,
			flags.InteropMockEth1DataVotesFlag,
		},
	},
	{ // Flags relevant to configuring beacon chain monitoring.
		Name: "monitoring",
		Flags: []cli.Flag{
			cmd.DisableMonitoringFlag,
			cmd.EnableTracingFlag,
			cmd.MonitoringHostFlag,
			cmd.TraceSampleFractionFlag,
			cmd.TracingEndpointFlag,
			cmd.TracingProcessNameFlag,
			cmd.ValidatorMonitorIndicesFlag,
			flags.MonitoringPortFlag,
		},
	},
	{ // Flags relevant to slasher operation.
		Name: "slasher",
		Flags: []cli.Flag{
			flags.HistoricalSlasherNode,
			flags.SlasherDirFlag,
			flags.SlasherFlag,
		},
	},
	{
		// Flags in the "log" section control how Prysm handles logging.
		Name: "log",
		Flags: []cli.Flag{
			cmd.LogFormat,
			cmd.LogFileName,
			cmd.VerbosityFlag,
		},
	},
	{ // Feature flags.
		Name:  "features",
		Flags: features.ActiveFlags(features.BeaconChainFlags),
	},
	{ // Flags required to configure the merge.
		Name: "merge",
		Flags: []cli.Flag{
			flags.TerminalTotalDifficultyOverride,
			flags.TerminalBlockHashOverride,
			flags.TerminalBlockHashActivationEpochOverride,
		},
	},
	{ // The deprecated section represents beacon flags that still have use, but should not be used
		// as they are expected to be deleted in a feature release.
		Name:  "deprecated",
		Flags: []cli.Flag{},
	},
	{ // Flags used in debugging Prysm. These are flags not usually run by end users.
		Name: "debug",
		Flags: []cli.Flag{
			cmd.MaxGoroutines,
			debug.BlockProfileRateFlag,
			debug.MemProfileRateFlag,
			debug.MutexProfileFractionFlag,
			debug.PProfAddrFlag,
			debug.PProfFlag,
			debug.PProfPortFlag,
			flags.SetGCPercent,
		},
	},
}

func init() {
	cli.AppHelpTemplate = appHelpTemplate

	type helpData struct {
		App        interface{}
		FlagGroups []flagGroup
	}

	originalHelpPrinter := cli.HelpPrinter
	cli.HelpPrinter = func(w io.Writer, tmpl string, data interface{}) {
		if tmpl == appHelpTemplate {
			for _, group := range appHelpFlagGroups {
				sort.Sort(cli.FlagsByName(group.Flags))
			}
			originalHelpPrinter(w, tmpl, helpData{data, appHelpFlagGroups})
		} else {
			originalHelpPrinter(w, tmpl, data)
		}
	}
}
