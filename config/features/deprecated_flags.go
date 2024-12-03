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
	deprecatedEnableOptionalEngineMethods = &cli.BoolFlag{
		Name:   "enable-optional-engine-methods",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedDisableBuildBlockParallel = &cli.BoolFlag{
		Name:   "disable-build-block-parallel",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedDisableReorgLateBlocks = &cli.BoolFlag{
		Name:   "disable-reorg-late-blocks",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedDisableOptionalEngineMethods = &cli.BoolFlag{
		Name:   "disable-optional-engine-methods",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedDisableAggregateParallel = &cli.BoolFlag{
		Name:   "disable-aggregate-parallel",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedEnableEIP4881 = &cli.BoolFlag{
		Name:   "enable-eip-4881",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedDisableEIP4881 = &cli.BoolFlag{
		Name:   "disable-eip-4881",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedVerboseSigVerification = &cli.BoolFlag{
		Name:   "enable-verbose-sig-verification",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedEnableDebugRPCEndpoints = &cli.BoolFlag{
		Name:   "enable-debug-rpc-endpoints",
		Usage:  deprecatedUsage,
		Hidden: true,
	}

	deprecatedBeaconRPCGatewayProviderFlag = &cli.StringFlag{
		Name:   "beacon-rpc-gateway-provider",
		Usage:  deprecatedUsage,
		Hidden: true,
	}

	deprecatedDisableGRPCGateway = &cli.BoolFlag{
		Name:   "disable-grpc-gateway",
		Usage:  deprecatedUsage,
		Hidden: true,
	}

	deprecatedEnableExperimentalState = &cli.BoolFlag{
		Name:   "enable-experimental-state",
		Usage:  deprecatedUsage,
		Hidden: true,
	}

	deprecatedEnableCommitteeAwarePacking = &cli.BoolFlag{
		Name:   "enable-committee-aware-packing",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedInteropGenesisTimeFlag = &cli.Uint64Flag{
		Name:   "interop-genesis-time",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedInteropNumValidatorsFlag = &cli.Uint64Flag{
		Name:   "interop-num-validators",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
	deprecatedEnableQuic = &cli.BoolFlag{
		Name:   "enable-quic",
		Usage:  deprecatedUsage,
		Hidden: true,
	}
)

// Deprecated flags for both the beacon node and validator client.
var deprecatedFlags = []cli.Flag{
	exampleDeprecatedFeatureFlag,
	deprecatedEnableOptionalEngineMethods,
	deprecatedDisableBuildBlockParallel,
	deprecatedDisableReorgLateBlocks,
	deprecatedDisableOptionalEngineMethods,
	deprecatedDisableAggregateParallel,
	deprecatedEnableEIP4881,
	deprecatedDisableEIP4881,
	deprecatedVerboseSigVerification,
	deprecatedEnableDebugRPCEndpoints,
	deprecatedBeaconRPCGatewayProviderFlag,
	deprecatedDisableGRPCGateway,
	deprecatedEnableExperimentalState,
	deprecatedEnableCommitteeAwarePacking,
	deprecatedInteropGenesisTimeFlag,
	deprecatedEnableQuic,
}

// deprecatedBeaconFlags contains flags that are still used by other components
// and therefore cannot be added to deprecatedFlags
var deprecatedBeaconFlags = []cli.Flag{
	deprecatedInteropNumValidatorsFlag,
}
