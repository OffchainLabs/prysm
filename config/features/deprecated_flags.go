package features

import (
	"github.com/urfave/cli/v2"
)

// Deprecated flags list.
const deprecatedUsage = "DEPRECATED. DO NOT USE."

var (
	// To deprecate a feature flag, first copy the example below, then insert deprecated flag in `deprecatedFlags`.
	exampleDeprecatedFeatureFlag = &cli.StringFlag{
		Name:   "name",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedHTTPModules = &cli.StringFlag{
		Name:   "http-modules",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedEnableDBBackupWebhook = &cli.BoolFlag{
		Name:   "enable-db-backup-webhook",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedSlasherRPCProvider = &cli.StringFlag{
		Name:   "slasher-rpc-provider",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedSlasherTLSCert = &cli.StringFlag{
		Name:   "slasher-tls-cert",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
)

// Deprecated flags for both the beacon node and validator client.
var deprecatedFlags = []cli.Flag{
	deprecatedHTTPModules,
	deprecatedEnableDBBackupWebhook,
	deprecatedSlasherRPCProvider,
	deprecatedSlasherTLSCert,
}

var upcomingDeprecation = []cli.Flag{
	enableHistoricalSpaceRepresentation,
}

// deprecatedBeaconFlags contains flags that are still used by other components
// and therefore cannot be added to deprecatedFlags
var deprecatedBeaconFlags = []cli.Flag{
	deprecatedDisableLastEpochTargets,
}
