package features

import (
	"time"

	backfill "github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/sync/backfill/flags"
	"github.com/urfave/cli/v2"
)

var (
	// SepoliaTestnet flag for the multiclient Ethereum consensus testnet.
	SepoliaTestnet = &cli.BoolFlag{
		Name:  "sepolia",
		Usage: "Runs Prysm configured for the Sepolia test network.",
	}
	// HoleskyTestnet flag for the multiclient Ethereum consensus testnet.
	HoleskyTestnet = &cli.BoolFlag{
		Name:  "holesky",
		Usage: "Runs Prysm configured for the Holesky test network.",
	}
	// HoodiTestnet flag for ethereum testnet.
	HoodiTestnet = &cli.BoolFlag{
		Name:  "hoodi",
		Usage: "Runs Prysm configured for the Hoodi test network.",
	}
	// Mainnet flag for easier tooling, no-op
	Mainnet = &cli.BoolFlag{
		Value: true,
		Name:  "mainnet",
		Usage: "Runs on Ethereum main network. This is the default and can be omitted.",
	}
	devModeFlag = &cli.BoolFlag{
		Name:  "dev",
		Usage: "Enables experimental features still in development. These features may not be stable.",
	}
	writeSSZStateTransitionsFlag = &cli.BoolFlag{
		Name:  "interop-write-ssz-state-transitions",
		Usage: "Writes SSZ states to disk after attempted state transitio.",
	}
	saveInvalidBlockTempFlag = &cli.BoolFlag{
		Name:  "save-invalid-block-temp",
		Usage: "Writes invalid blocks to temp directory.",
	}
	saveInvalidBlobTempFlag = &cli.BoolFlag{
		Name:  "save-invalid-blob-temp",
		Usage: "Writes invalid blobs to temp directory.",
	}
	disableGRPCConnectionLogging = &cli.BoolFlag{
		Name: "disable-grpc-connection-logging",
		Usage: `WARNING: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API..
		Disables displaying logs for newly connected grpc clients.`,
	}
	disablePeerScorer = &cli.BoolFlag{
		Name:  "disable-peer-scorer",
		Usage: "(Danger): Disables P2P peer scorer. Do NOT use this in production!",
	}
	writeWalletPasswordOnWebOnboarding = &cli.BoolFlag{
		Name: "write-wallet-password-on-web-onboarding",
		Usage: `(Danger): Writes the wallet password to the wallet directory on completing Prysm web onboarding.
	We recommend against this flag unless you are an advanced user.`,
	}
	aggregateFirstInterval = &cli.DurationFlag{
		Name:   "aggregate-first-interval",
		Usage:  "(Advanced): Specifies the first interval in which attestations are aggregated in the slot (typically unnaggregated attestations are aggregated in this interval).",
		Value:  7000 * time.Millisecond,
		Hidden: true,
	}
	aggregateSecondInterval = &cli.DurationFlag{
		Name:   "aggregate-second-interval",
		Usage:  "(Advanced): Specifies the second interval in which attestations are aggregated in the slot.",
		Value:  9500 * time.Millisecond,
		Hidden: true,
	}
	aggregateThirdInterval = &cli.DurationFlag{
		Name:   "aggregate-third-interval",
		Usage:  "(Advanced): Specifies the third interval in which attestations are aggregated in the slot.",
		Value:  11800 * time.Millisecond,
		Hidden: true,
	}
	dynamicKeyReloadDebounceInterval = &cli.DurationFlag{
		Name: "dynamic-key-reload-debounce-interval",
		Usage: `(Advanced): Specifies the time duration the validator waits to reload new keys if they have changed on disk.
	Can be any type of duration such as 1.5s, 1000ms, 1m.`,
		Value: time.Second,
	}
	disableBroadcastSlashingFlag = &cli.BoolFlag{
		Name:  "disable-broadcast-slashings",
		Usage: "Disables broadcasting slashings submitted to the beacon node.",
	}
	disableAttestTimely = &cli.BoolFlag{
		Name:  "disable-attest-timely",
		Usage: "Disable validator attesting timely after current block processes. See #8185 for more details.",
	}
	enableSlashingProtectionPruning = &cli.BoolFlag{
		Name:  "enable-slashing-protection-history-pruning",
		Usage: "Enables the pruning of the validator client's slashing protection database.",
	}
	EnableMinimalSlashingProtection = &cli.BoolFlag{
		Name:  "enable-minimal-slashing-protection",
		Usage: "(Experimental): Enables the minimal slashing protection. See EIP-3076 for more details.",
	}
	enableDoppelGangerProtection = &cli.BoolFlag{
		Name: "enable-doppelganger",
		Usage: `Enables the validator to perform a doppelganger check. 
		This is not a foolproof method to find duplicate instances in the network. 
		Your validator will still be vulnerable if it is being run in unsafe configurations.`,
	}
	disableStakinContractCheck = &cli.BoolFlag{
		Name:  "disable-staking-contract-check",
		Usage: "Disables checking of staking contract deposits when proposing blocks, useful for devnets.",
	}
	enableHistoricalSpaceRepresentation = &cli.BoolFlag{
		Name: "enable-historical-state-representation",
		Usage: "Enables the beacon chain to save historical states in a space efficient manner." +
			" (Warning): Once enabled, this feature migrates your database in to a new schema and " +
			"there is no going back. At worst, your entire database might get corrupted.",
	}
	enableStartupOptimistic = &cli.BoolFlag{
		Name:   "startup-optimistic",
		Usage:  "Treats every block as optimistically synced at launch. Use with caution.",
		Value:  false,
		Hidden: true,
	}
	enableFullSSZDataLogging = &cli.BoolFlag{
		Name:  "enable-full-ssz-data-logging",
		Usage: "Enables displaying logs for full ssz data on rejected gossip messages.",
	}
	SaveFullExecutionPayloads = &cli.BoolFlag{
		Name:  "save-full-execution-payloads",
		Usage: "Saves beacon blocks with full execution payloads instead of execution payload headers in the database.",
	}
	EnableBeaconRESTApi = &cli.BoolFlag{
		Name:  "enable-beacon-rest-api",
		Usage: "(Experimental): Enables of the beacon REST API when querying a beacon node.",
	}
	disableVerboseSigVerification = &cli.BoolFlag{
		Name:  "disable-verbose-sig-verification",
		Usage: "Disables identifying invalid signatures if batch verification fails when processing block.",
	}
	prepareAllPayloads = &cli.BoolFlag{
		Name:  "prepare-all-payloads",
		Usage: "Informs the engine to prepare all local payloads. Useful for relayers and builders.",
	}
	EnableLightClient = &cli.BoolFlag{
		Name:  "enable-light-client",
		Usage: "Enables the light client support in the beacon node",
	}
	disableResourceManager = &cli.BoolFlag{
		Name:  "disable-resource-manager",
		Usage: "Disables running the libp2p resource manager.",
	}
	// DisableRegistrationCache a flag for disabling the validator registration cache and use db instead.
	DisableRegistrationCache = &cli.BoolFlag{
		Name:  "disable-registration-cache",
		Usage: "Temporary flag for disabling the validator registration cache instead of using the DB. Note: registrations do not clear on restart while using the DB.",
	}
	// BlobSaveFsync enforces durable filesystem writes for use cases where blob availability is critical.
	BlobSaveFsync = &cli.BoolFlag{
		Name:  "blob-save-fsync",
		Usage: "Forces new blob files to be fysnc'd before continuing, ensuring durable blob writes.",
	}
	// DisableQUIC disables connecting to peers using the QUIC protocol.
	DisableQUIC = &cli.BoolFlag{
		Name:  "disable-quic",
		Usage: "Disables connecting using the QUIC protocol with peers.",
	}
	EnableDiscoveryReboot = &cli.BoolFlag{
		Name:  "enable-discovery-reboot",
		Usage: "Experimental: Enables the discovery listener to rebooted in the event of connectivity issues.",
	}
	enableExperimentalAttestationPool = &cli.BoolFlag{
		Name:  "enable-experimental-attestation-pool",
		Usage: "Enables an experimental attestation pool design.",
	}
	// forceHeadFlag is a flag to force the head of the beacon chain to a specific block.
	forceHeadFlag = &cli.StringFlag{
		Name: "sync-from",
		Usage: "Forces the head of the beacon chain to a specific block root. Values can be 'head' or a block root." +
			" The block root has to be known to the beacon node and correspond to a block newer than the current finalized checkpoint.",
	}
	// blacklistRoots is a flag for blacklisting block roots from gossip and
	// downscore peers that send them.
	blacklistRoots = &cli.StringSliceFlag{
		Name:  "blacklist-roots",
		Usage: "A comma-separatted list of 0x-prefixed hexstrings. Declares blocks with the given blockroots to be invalid. It downscores peers that send these blocks.",
	}

	// DisableDutiesV2 sets the validator client to use the get duties grpc endpoint
	DisableDutiesV2 = &cli.BoolFlag{
		Name:  "disable-duties-v2",
		Usage: "Forces use of get duties endpoint instead of v2.",
	}

	// EnableWebFlag enables controlling the validator client via the Prysm web ui. This is a work in progress.
	EnableWebFlag = &cli.BoolFlag{
		Name:  "web",
		Usage: "(Work in progress): Enables the web portal for the validator client.",
		Value: false,
	}
)

// devModeFlags holds list of flags that are set when development mode is on.
var devModeFlags = []cli.Flag{
	backfill.EnableExperimentalBackfill,
}

// ValidatorFlags contains a list of all the feature flags that apply to the validator client.
var ValidatorFlags = append(deprecatedFlags, []cli.Flag{
	writeWalletPasswordOnWebOnboarding,
	HoleskyTestnet,
	SepoliaTestnet,
	HoodiTestnet,
	Mainnet,
	dynamicKeyReloadDebounceInterval,
	disableAttestTimely,
	enableSlashingProtectionPruning,
	EnableMinimalSlashingProtection,
	enableDoppelGangerProtection,
	EnableBeaconRESTApi,
	DisableDutiesV2,
	EnableWebFlag,
}...)

// E2EValidatorFlags contains a list of the validator feature flags to be tested in E2E.
var E2EValidatorFlags = []string{
	"--enable-doppelganger",
}

// BeaconChainFlags contains a list of all the feature flags that apply to the beacon-chain client.
var BeaconChainFlags = combinedFlags([]cli.Flag{
	devModeFlag,
	writeSSZStateTransitionsFlag,
	saveInvalidBlockTempFlag,
	saveInvalidBlobTempFlag,
	disableGRPCConnectionLogging,
	HoleskyTestnet,
	SepoliaTestnet,
	HoodiTestnet,
	Mainnet,
	disablePeerScorer,
	disableBroadcastSlashingFlag,
	disableStakinContractCheck,
	SaveFullExecutionPayloads,
	enableStartupOptimistic,
	enableFullSSZDataLogging,
	disableVerboseSigVerification,
	prepareAllPayloads,
	aggregateFirstInterval,
	aggregateSecondInterval,
	aggregateThirdInterval,
	disableResourceManager,
	DisableRegistrationCache,
	EnableLightClient,
	BlobSaveFsync,
	DisableQUIC,
	EnableDiscoveryReboot,
	enableExperimentalAttestationPool,
	forceHeadFlag,
	blacklistRoots,
}, deprecatedBeaconFlags, deprecatedFlags, upcomingDeprecation)

func combinedFlags(flags ...[]cli.Flag) []cli.Flag {
	if len(flags) == 0 {
		return []cli.Flag{}
	}
	collected := flags[0]
	for _, f := range flags[1:] {
		collected = append(collected, f...)
	}
	return collected
}

// E2EBeaconChainFlags contains a list of the beacon chain feature flags to be tested in E2E.
var E2EBeaconChainFlags = []string{
	"--dev",
}

// NetworkFlags contains a list of network flags.
var NetworkFlags = []cli.Flag{
	Mainnet,
	SepoliaTestnet,
	HoleskyTestnet,
	HoodiTestnet,
}
