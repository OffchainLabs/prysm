package integration

import (
	"os"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// Config controls the integration test cluster.
type Config struct {
	// Cluster topology.
	NumBeaconNodes int // default 2
	NumGethNodes   int // default 2
	NumValidators  int // default 64

	// Timing.
	SecondsPerSlot uint64 // default 2
	SlotsPerEpoch  uint64 // default 8

	// Genesis fork version.
	GenesisFork int // default version.Gloas

	// Geth source. If GethBinary is set, use that binary directly (skip build).
	// Otherwise if GethRepo is set, clone and build from that repo+branch.
	// Otherwise build from the go module dependency.
	GethBinary string // env: GETH_BINARY
	GethRepo   string // env: GETH_REPO (e.g. "github.com/ethpandaops/go-ethereum")
	GethBranch string // env: GETH_BRANCH (e.g. "epbs-devnet-1")

	// How long to wait for the cluster to produce blocks.
	Timeout time.Duration // default 2 minutes
}

// DefaultConfig returns a sensible default for Gloas integration tests.
// Geth source can be overridden via environment variables:
//
//	GETH_BINARY=/path/to/geth    — use a pre-built binary
//	GETH_REPO=github.com/x/y    — clone and build from this repo
//	GETH_BRANCH=epbs-devnet-1   — branch to checkout (requires GETH_REPO)
func DefaultConfig() *Config {
	return &Config{
		NumBeaconNodes: 2,
		NumGethNodes:   2,
		NumValidators:  2048,
		SecondsPerSlot: 4,
		SlotsPerEpoch:  8,
		GenesisFork:    version.Gloas,
		GethBinary:     os.Getenv("GETH_BINARY"),
		GethRepo:       os.Getenv("GETH_REPO"),
		GethBranch:     os.Getenv("GETH_BRANCH"),
		Timeout:        2 * time.Minute,
	}
}

// BeaconConfig returns a minimal beacon chain config for the integration test.
func (c *Config) BeaconConfig() *params.BeaconChainConfig {
	// Use the config matching the compile-time field params.
	var cfg *params.BeaconChainConfig
	if fieldparams.Preset == "minimal" {
		cfg = params.MinimalSpecConfig().Copy()
	} else {
		cfg = params.InteropConfig().Copy()
	}
	cfg.ConfigName = params.DevnetName
	// Unique fork versions that don't conflict with any built-in config.
	cfg.GenesisForkVersion = []byte{0, 0, 0, 99}
	cfg.AltairForkVersion = []byte{1, 0, 0, 99}
	cfg.BellatrixForkVersion = []byte{2, 0, 0, 99}
	cfg.CapellaForkVersion = []byte{3, 0, 0, 99}
	cfg.DenebForkVersion = []byte{4, 0, 0, 99}
	cfg.ElectraForkVersion = []byte{5, 0, 0, 99}
	cfg.FuluForkVersion = []byte{6, 0, 0, 99}
	cfg.GloasForkVersion = []byte{7, 0, 0, 99}
	cfg.SecondsPerSlot = c.SecondsPerSlot
	cfg.SlotDurationMilliseconds = c.SecondsPerSlot * 1000
	cfg.SlotsPerEpoch = primitives.Slot(c.SlotsPerEpoch)
	cfg.MinGenesisActiveValidatorCount = uint64(c.NumValidators)

	// Match geth's chain ID.
	cfg.DepositChainID = 1337
	cfg.DepositNetworkID = 1337

	// All forks at genesis.
	cfg.AltairForkEpoch = 0
	cfg.BellatrixForkEpoch = 0
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0

	if c.GenesisFork >= version.Gloas {
		cfg.GloasForkEpoch = 0
	}

	return cfg
}

// ValidatorsPerNode returns how many validators each beacon node manages.
func (c *Config) ValidatorsPerNode() int {
	return c.NumValidators / c.NumBeaconNodes
}
