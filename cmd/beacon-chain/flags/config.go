package flags

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/cmd"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

const maxStateDiffExponents = 30

// GlobalFlags specifies all the global flags for the
// beacon node.
type GlobalFlags struct {
	SubscribeToAllSubnets           bool
	Supernode                       bool
	SemiSupernode                   bool
	MinimumSyncPeers                int
	MinimumPeersPerSubnet           int
	MaxConcurrentDials              int
	BlockBatchLimit                 int
	BlockBatchLimitBurstFactor      int
	BlobBatchLimit                  int
	BlobBatchLimitBurstFactor       int
	DataColumnBatchLimit            int
	DataColumnBatchLimitBurstFactor int
	StateDiffExponents              []int
}

var globalConfig *GlobalFlags

// Get retrieves the global config.
func Get() *GlobalFlags {
	if globalConfig == nil {
		return &GlobalFlags{}
	}
	return globalConfig
}

// Init sets the global config equal to the config that is passed in.
func Init(c *GlobalFlags) {
	globalConfig = c
}

// ConfigureGlobalFlags initializes the global config.
// based on the provided cli context.
func ConfigureGlobalFlags(ctx *cli.Context) error {
	cfg := &GlobalFlags{}

	if ctx.Bool(SubscribeToAllSubnets.Name) {
		log.Warning("Subscribing to all attestation subnets")
		cfg.SubscribeToAllSubnets = true
	}

	supernodeSet := ctx.Bool(Supernode.Name)
	semiSupernodeSet := ctx.Bool(SemiSupernode.Name)

	// Ensure mutual exclusivity between supernode and semi-supernode modes
	if supernodeSet && semiSupernodeSet {
		return fmt.Errorf("cannot set both --%s and --%s flags; choose one mode", Supernode.Name, SemiSupernode.Name)
	}

	if supernodeSet {
		log.Info("Operating in supernode mode")
		cfg.Supernode = true
	}

	if semiSupernodeSet {
		log.Info("Operating in semi-supernode mode (custody just enough data to serve the blobs and blob sidecars beacon API)")
		cfg.SemiSupernode = true
	}

	// State-diff-exponents
	cfg.StateDiffExponents = ctx.IntSlice(StateDiffExponents.Name)
	if features.Get().EnableStateDiff {
		if err := validateStateDiffExponents(cfg.StateDiffExponents); err != nil {
			return err
		}
	} else {
		if ctx.IsSet(StateDiffExponents.Name) {
			log.Warn("--state-diff-exponents is set but --enable-state-diff is not; the value will be ignored.")
		}
	}

	cfg.BlockBatchLimit = ctx.Int(BlockBatchLimit.Name)
	cfg.BlockBatchLimitBurstFactor = ctx.Int(BlockBatchLimitBurstFactor.Name)
	cfg.BlobBatchLimit = ctx.Int(BlobBatchLimit.Name)
	cfg.BlobBatchLimitBurstFactor = ctx.Int(BlobBatchLimitBurstFactor.Name)
	cfg.DataColumnBatchLimit = ctx.Int(DataColumnBatchLimit.Name)
	cfg.DataColumnBatchLimitBurstFactor = ctx.Int(DataColumnBatchLimitBurstFactor.Name)
	cfg.MinimumPeersPerSubnet = ctx.Int(MinPeersPerSubnet.Name)
	cfg.MaxConcurrentDials = ctx.Int(MaxConcurrentDials.Name)

	configureMinimumPeers(ctx, cfg)

	Init(cfg)
	return nil
}

// MaxDialIsActive checks if the user has enabled the max dial flag.
func MaxDialIsActive() bool {
	return Get().MaxConcurrentDials > 0
}

func configureMinimumPeers(ctx *cli.Context, cfg *GlobalFlags) {
	cfg.MinimumSyncPeers = ctx.Int(MinSyncPeers.Name)
	maxPeers := ctx.Int(cmd.P2PMaxPeers.Name)
	if cfg.MinimumSyncPeers > maxPeers {
		log.Warnf("Changing Minimum Sync Peers to %d", maxPeers)
		cfg.MinimumSyncPeers = maxPeers
	}
}

// validateStateDiffExponents validates the provided exponents for state diffs with these constraints in mind:
//   - Must contain between 1 and 15 values.
//   - Exponents must be in strictly decreasing order.
//   - Every exponent must be <= 30. (2^30 slots is more than 300 years at 12s slots)
//   - The last (smallest) exponent must be >= 5. (This ensures diffs are at least 1 epoch apart)
func validateStateDiffExponents(exponents []int) error {
	length := len(exponents)
	if length == 0 || length > 15 {
		return errors.New("state diff exponents must contain between 1 and 15 values")
	}
	if exponents[length-1] < 5 {
		return errors.New("the last state diff exponent must be at least 5")
	}
	prev := maxStateDiffExponents + 1
	for _, exp := range exponents {
		if exp >= prev {
			return errors.New("state diff exponents must be in strictly decreasing order, and each exponent must be <= 30")
		}
		prev = exp
	}
	return nil
}
