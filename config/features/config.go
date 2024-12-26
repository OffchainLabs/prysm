/*
Package features defines which features are enabled for runtime
in order to selectively enable certain features to maintain a stable runtime.

The process for implementing new features using this package is as follows:
 1. Add a new CMD flag in flags.go, and place it in the proper list(s) var for its client.
 2. Add a condition for the flag in the proper Configure function(s) below.
 3. Place any "new" behavior in the `if flagEnabled` statement.
 4. Place any "previous" behavior in the `else` statement.
 5. Ensure any tests using the new feature fail if the flag isn't enabled.
    5a. Use the following to enable your flag for tests:
    cfg := &featureconfig.Flags{
    VerifyAttestationSigs: true,
    }
    resetCfg := featureconfig.InitWithReset(cfg)
    defer resetCfg()
 6. Add the string for the flags that should be running within E2E to E2EValidatorFlags
    and E2EBeaconChainFlags.
*/
package features

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var log = logrus.WithField("prefix", "flags")

const enabledFeatureFlag = "Enabled feature flag"
const disabledFeatureFlag = "Disabled feature flag"

// Flags is a struct to represent which features the client will perform on runtime.
type Flags struct {
	// Feature related flags.
	EnableExperimentalState             bool // EnableExperimentalState turns on the latest and greatest (but potentially unstable) changes to the beacon state.
	WriteSSZStateTransitions            bool // WriteSSZStateTransitions to tmp directory.
	EnablePeerScorer                    bool // EnablePeerScorer enables experimental peer scoring in p2p.
	EnableLightClient                   bool // EnableLightClient enables light client APIs.
	EnableQUIC                          bool // EnableQUIC specifies whether to enable QUIC transport for libp2p.
	WriteWalletPasswordOnWebOnboarding  bool // WriteWalletPasswordOnWebOnboarding writes the password to disk after Prysm web signup.
	EnableDoppelGanger                  bool // EnableDoppelGanger enables doppelganger protection on startup for the validator.
	EnableHistoricalSpaceRepresentation bool // EnableHistoricalSpaceRepresentation enables the saving of registry validators in separate buckets to save space
	EnableBeaconRESTApi                 bool // EnableBeaconRESTApi enables experimental usage of the beacon REST API by the validator when querying a beacon node
	DisableCommitteeAwarePacking        bool // DisableCommitteeAwarePacking changes the attestation packing algorithm to one that is not aware of attesting committees.
	EnableExperimentalAttestationPool   bool // EnableExperimentalAttestationPool enables an experimental attestation pool design.
	// Logging related toggles.
	DisableGRPCConnectionLogs bool // Disables logging when a new grpc client has connected.
	EnableFullSSZDataLogging  bool // Enables logging for full ssz data on rejected gossip messages

	// Slasher toggles.
	DisableBroadcastSlashings bool // DisableBroadcastSlashings disables p2p broadcasting of proposer and attester slashings.

	// Bug fixes related flags.
	AttestTimely bool // AttestTimely fixes #8185. It is gated behind a flag to ensure beacon node's fix can safely roll out first. We'll invert this in v1.1.0.

	EnableSlasher                   bool // Enable slasher in the beacon node runtime.
	EnableSlashingProtectionPruning bool // Enable slashing protection pruning for the validator client.
	EnableMinimalSlashingProtection bool // Enable minimal slashing protection database for the validator client.

	SaveFullExecutionPayloads bool // Save full beacon blocks with execution payloads in the database.
	EnableStartOptimistic     bool // EnableStartOptimistic treats every block as optimistic at startup.

	DisableResourceManager     bool // Disables running the node with libp2p's resource manager.
	DisableStakinContractCheck bool // Disables check for deposit contract when proposing blocks

	EnableVerboseSigVerification bool // EnableVerboseSigVerification specifies whether to verify individual signature if batch verification fails

	PrepareAllPayloads bool // PrepareAllPayloads informs the engine to prepare a block on every slot.
	// BlobSaveFsync requires blob saving to block on fsync to ensure blobs are durably persisted before passing DA.
	BlobSaveFsync bool

	SaveInvalidBlock bool // SaveInvalidBlock saves invalid block to temp.
	SaveInvalidBlob  bool // SaveInvalidBlob saves invalid blob to temp.

	EnableDiscoveryReboot bool // EnableDiscoveryReboot allows the node to have its local listener to be rebooted in the event of discovery issues.

	// KeystoreImportDebounceInterval specifies the time duration the validator waits to reload new keys if they have
	// changed on disk. This feature is for advanced use cases only.
	KeystoreImportDebounceInterval time.Duration

	// AggregateIntervals specifies the time durations at which we aggregate attestations preparing for forkchoice.
	AggregateIntervals [3]time.Duration
}

var featureConfig *Flags
var featureConfigLock sync.RWMutex

// Get retrieves feature config.
func Get() *Flags {
	featureConfigLock.RLock()
	defer featureConfigLock.RUnlock()

	if featureConfig == nil {
		return &Flags{}
	}
	return featureConfig
}

// Init sets the global config equal to the config that is passed in.
func Init(c *Flags) {
	featureConfigLock.Lock()
	defer featureConfigLock.Unlock()

	featureConfig = c
}

// InitWithReset sets the global config and returns function that is used to reset configuration.
func InitWithReset(c *Flags) func() {
	var prevConfig Flags
	if featureConfig != nil {
		prevConfig = *featureConfig
	} else {
		prevConfig = Flags{}
	}
	resetFunc := func() {
		Init(&prevConfig)
	}
	Init(c)
	return resetFunc
}

// configureTestnet sets the config according to specified testnet flag
func configureTestnet(ctx *cli.Context) error {
	if ctx.Bool(SepoliaTestnet.Name) {
		log.Info("Running on the Sepolia Beacon Chain Testnet")
		if err := params.SetActive(params.SepoliaConfig().Copy()); err != nil {
			return err
		}
		applySepoliaFeatureFlags(ctx)
		params.UseSepoliaNetworkConfig()
	} else if ctx.Bool(HoleskyTestnet.Name) {
		log.Info("Running on the Holesky Beacon Chain Testnet")
		if err := params.SetActive(params.HoleskyConfig().Copy()); err != nil {
			return err
		}
		applyHoleskyFeatureFlags(ctx)
		params.UseHoleskyNetworkConfig()
	} else {
		if ctx.IsSet(cmd.ChainConfigFileFlag.Name) {
			log.Warn("Running on custom Ethereum network specified in a chain configuration yaml file")
		} else {
			log.Info("Running on Ethereum Mainnet")
		}
		if err := params.SetActive(params.MainnetConfig().Copy()); err != nil {
			return err
		}
	}
	return nil
}

// Insert feature flags within the function to be enabled for Sepolia testnet.
func applySepoliaFeatureFlags(ctx *cli.Context) {
}

// Insert feature flags within the function to be enabled for Holesky testnet.
func applyHoleskyFeatureFlags(ctx *cli.Context) {
}

// ConfigureBeaconChain sets the global config based
// on what flags are enabled for the beacon-chain client.
func ConfigureBeaconChain(ctx *cli.Context) error {
	complainOnDeprecatedFlags(ctx)
	cfg := &Flags{}
	if ctx.Bool(devModeFlag.Name) {
		enableDevModeFlags(ctx)
	}
	if err := configureTestnet(ctx); err != nil {
		return err
	}

	cfg.EnableExperimentalState = true
	if ctx.Bool(disableExperimentalState.Name) {
		logEnabled(disableExperimentalState)
		cfg.EnableExperimentalState = false
	}

	if ctx.Bool(writeSSZStateTransitionsFlag.Name) {
		logEnabled(writeSSZStateTransitionsFlag)
		cfg.WriteSSZStateTransitions = true
	}

	if ctx.Bool(saveInvalidBlockTempFlag.Name) {
		logEnabled(saveInvalidBlockTempFlag)
		cfg.SaveInvalidBlock = true
	}

	if ctx.Bool(saveInvalidBlobTempFlag.Name) {
		logEnabled(saveInvalidBlobTempFlag)
		cfg.SaveInvalidBlob = true
	}

	if ctx.IsSet(disableGRPCConnectionLogging.Name) {
		logDisabled(disableGRPCConnectionLogging)
		cfg.DisableGRPCConnectionLogs = true
	}

	cfg.EnablePeerScorer = true
	if ctx.Bool(disablePeerScorer.Name) {
		logDisabled(disablePeerScorer)
		cfg.EnablePeerScorer = false
	}
	if ctx.Bool(disableBroadcastSlashingFlag.Name) {
		logDisabled(disableBroadcastSlashingFlag)
		cfg.DisableBroadcastSlashings = true
	}
	if ctx.Bool(enableSlasherFlag.Name) {
		log.WithField(enableSlasherFlag.Name, enableSlasherFlag.Usage).Warn(enabledFeatureFlag)
		cfg.EnableSlasher = true
	}
	if ctx.Bool(enableHistoricalSpaceRepresentation.Name) {
		log.WithField(enableHistoricalSpaceRepresentation.Name, enableHistoricalSpaceRepresentation.Usage).Warn(enabledFeatureFlag)
		cfg.EnableHistoricalSpaceRepresentation = true
	}
	if ctx.Bool(disableStakinContractCheck.Name) {
		logEnabled(disableStakinContractCheck)
		cfg.DisableStakinContractCheck = true
	}
	if ctx.Bool(SaveFullExecutionPayloads.Name) {
		logEnabled(SaveFullExecutionPayloads)
		cfg.SaveFullExecutionPayloads = true
	}
	if ctx.Bool(enableStartupOptimistic.Name) {
		logEnabled(enableStartupOptimistic)
		cfg.EnableStartOptimistic = true
	}
	if ctx.IsSet(enableFullSSZDataLogging.Name) {
		logEnabled(enableFullSSZDataLogging)
		cfg.EnableFullSSZDataLogging = true
	}
	cfg.EnableVerboseSigVerification = true
	if ctx.IsSet(disableVerboseSigVerification.Name) {
		logEnabled(disableVerboseSigVerification)
		cfg.EnableVerboseSigVerification = false
	}
	if ctx.IsSet(prepareAllPayloads.Name) {
		logEnabled(prepareAllPayloads)
		cfg.PrepareAllPayloads = true
	}
	if ctx.IsSet(disableResourceManager.Name) {
		logEnabled(disableResourceManager)
		cfg.DisableResourceManager = true
	}
	if ctx.IsSet(EnableLightClient.Name) {
		logEnabled(EnableLightClient)
		cfg.EnableLightClient = true
	}
	if ctx.IsSet(BlobSaveFsync.Name) {
		logEnabled(BlobSaveFsync)
		cfg.BlobSaveFsync = true
	}
	cfg.EnableQUIC = true
	if ctx.IsSet(DisableQUIC.Name) {
		logDisabled(DisableQUIC)
		cfg.EnableQUIC = false
	}
	if ctx.IsSet(DisableCommitteeAwarePacking.Name) {
		logEnabled(DisableCommitteeAwarePacking)
		cfg.DisableCommitteeAwarePacking = true
	}
	if ctx.IsSet(EnableDiscoveryReboot.Name) {
		logEnabled(EnableDiscoveryReboot)
		cfg.EnableDiscoveryReboot = true
	}
	if ctx.IsSet(enableExperimentalAttestationPool.Name) {
		logEnabled(enableExperimentalAttestationPool)
		cfg.EnableExperimentalAttestationPool = true
	}

	cfg.AggregateIntervals = [3]time.Duration{aggregateFirstInterval.Value, aggregateSecondInterval.Value, aggregateThirdInterval.Value}
	Init(cfg)
	return nil
}

// ConfigureValidator sets the global config based
// on what flags are enabled for the validator client.
func ConfigureValidator(ctx *cli.Context) error {
	complainOnDeprecatedFlags(ctx)
	cfg := &Flags{}
	if err := configureTestnet(ctx); err != nil {
		return err
	}
	if ctx.Bool(writeWalletPasswordOnWebOnboarding.Name) {
		logEnabled(writeWalletPasswordOnWebOnboarding)
		cfg.WriteWalletPasswordOnWebOnboarding = true
	}
	if ctx.Bool(attestTimely.Name) {
		logEnabled(attestTimely)
		cfg.AttestTimely = true
	}
	if ctx.Bool(enableSlashingProtectionPruning.Name) {
		logEnabled(enableSlashingProtectionPruning)
		cfg.EnableSlashingProtectionPruning = true
	}
	if ctx.Bool(EnableMinimalSlashingProtection.Name) {
		logEnabled(EnableMinimalSlashingProtection)
		cfg.EnableMinimalSlashingProtection = true
	}
	if ctx.Bool(enableDoppelGangerProtection.Name) {
		logEnabled(enableDoppelGangerProtection)
		cfg.EnableDoppelGanger = true
	}
	if ctx.Bool(EnableBeaconRESTApi.Name) {
		logEnabled(EnableBeaconRESTApi)
		cfg.EnableBeaconRESTApi = true
	}
	cfg.KeystoreImportDebounceInterval = ctx.Duration(dynamicKeyReloadDebounceInterval.Name)
	Init(cfg)
	return nil
}

// enableDevModeFlags switches development mode features on.
func enableDevModeFlags(ctx *cli.Context) {
	log.Warn("Enabling development mode flags")
	for _, f := range devModeFlags {
		log.WithField("flag", f.Names()[0]).Debug("Enabling development mode flag")
		if !ctx.IsSet(f.Names()[0]) {
			if err := ctx.Set(f.Names()[0], "true"); err != nil {
				log.WithError(err).Debug("Error enabling development mode flag")
			}
		}
	}
}

func complainOnDeprecatedFlags(ctx *cli.Context) {
	for _, f := range deprecatedFlags {
		if ctx.IsSet(f.Names()[0]) {
			log.Errorf("%s is deprecated and has no effect. Do not use this flag, it will be deleted soon.", f.Names()[0])
		}
	}
}

func logEnabled(flag cli.DocGenerationFlag) {
	var name string
	if names := flag.Names(); len(names) > 0 {
		name = names[0]
	}
	log.WithField(name, flag.GetUsage()).Warn(enabledFeatureFlag)
}

func logDisabled(flag cli.DocGenerationFlag) {
	var name string
	if names := flag.Names(); len(names) > 0 {
		name = names[0]
	}
	log.WithField(name, flag.GetUsage()).Warn(disabledFeatureFlag)
}

// ValidateNetworkFlags validates provided flags and
// prevents beacon node or validator to start
// if more than one network flag is provided
func ValidateNetworkFlags(ctx *cli.Context) error {
	networkFlagsCount := 0
	for _, flag := range NetworkFlags {
		if ctx.IsSet(flag.Names()[0]) {
			networkFlagsCount++
			if networkFlagsCount > 1 {
				// using a forLoop so future addition
				// doesn't require changes in this function
				var flagNames []string
				for _, flag := range NetworkFlags {
					flagNames = append(flagNames, "--"+flag.Names()[0])
				}
				return fmt.Errorf("cannot use more than one network flag at the same time. Possible network flags are: %s", strings.Join(flagNames, ", "))
			}
		}
	}
	return nil
}
