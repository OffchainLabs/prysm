package features

import (
	"time"

	backfill "github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/sync/backfill/flags"
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
	disableExperimentalState = &cli.BoolFlag{
		Name:  "disable-experimental-state",
		Usage: "Turns off the latest and greatest changes to the beacon state. Disabling this is safe to do after the feature has been enabled.",
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
		Name:  "disable-grpc-connection-logging",
		Usage: "Disables displaying logs for newly connected grpc clients.",
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
	attestTimely = &cli.BoolFlag{
		Name:  "attest-timely",
		Usage: "Fixes validator can attest timely after current block processes. See #8185 for more details.",
	}
	enableSlasherFlag = &cli.BoolFlag{
		Name:  "slasher",
		Usage: "Enables a slasher in the beacon node for detecting slashable offenses.",
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
		Name:  "enable-lightclient",
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
	DisableCommitteeAwarePacking = &cli.BoolFlag{
		Name:  "disable-committee-aware-packing",
		Usage: "Changes the attestation packing algorithm to one that is not aware of attesting committees.",
	}
	EnableDiscoveryReboot = &cli.BoolFlag{
		Name:  "enable-discovery-reboot",
		Usage: "Experimental: Enables the discovery listener to rebooted in the event of connectivity issues.",
	}
	enableExperimentalAttestationPool = &cli.BoolFlag{
		Name:  "enable-experimental-attestation-pool",
		Usage: "Enables an experimental attestation pool design.",
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
	Mainnet,
	dynamicKeyReloadDebounceInterval,
	attestTimely,
	enableSlashingProtectionPruning,
	EnableMinimalSlashingProtection,
	enableDoppelGangerProtection,
	EnableBeaconRESTApi,
}...)

// E2EValidatorFlags contains a list of the validator feature flags to be tested in E2E.
var E2EValidatorFlags = []string{
	"--enable-doppelganger",
}

// BeaconChainFlags contains a list of all the feature flags that apply to the beacon-chain client.
var BeaconChainFlags = append(deprecatedBeaconFlags, append(deprecatedFlags, []cli.Flag{
	devModeFlag,
	disableExperimentalState,
	writeSSZStateTransitionsFlag,
	saveInvalidBlockTempFlag,
	saveInvalidBlobTempFlag,
	disableGRPCConnectionLogging,
	HoleskyTestnet,
	SepoliaTestnet,
	Mainnet,
	disablePeerScorer,
	disableBroadcastSlashingFlag,
	enableSlasherFlag,
	enableHistoricalSpaceRepresentation,
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
	DisableCommitteeAwarePacking,
	EnableDiscoveryReboot,
	enableExperimentalAttestationPool,
}...)...)

// E2EBeaconChainFlags contains a list of the beacon chain feature flags to be tested in E2E.
var E2EBeaconChainFlags = []string{
	"--dev",
}

// NetworkFlags contains a list of network flags.
var NetworkFlags = []cli.Flag{
	Mainnet,
	SepoliaTestnet,
	HoleskyTestnet,
}
