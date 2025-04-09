package flags

import (
	"github.com/prysmaticlabs/prysm/v5/cmd"
	"github.com/urfave/cli/v2"
)

// GlobalFlags specifies all the global flags for the
// beacon node.
type GlobalFlags struct {
	SubscribeToAllSubnets      bool
	MinimumSyncPeers           int
	MinimumPeersPerSubnet      int
	MaxConcurrentDials         int
	BlockBatchLimit            int
	BlockBatchLimitBurstFactor int
	BlobBatchLimit             int
	BlobBatchLimitBurstFactor  int
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
func ConfigureGlobalFlags(ctx *cli.Context) {
	cfg := &GlobalFlags{}
	if ctx.Bool(SubscribeToAllSubnets.Name) {
		log.Warn("Subscribing to All Attestation Subnets")
		cfg.SubscribeToAllSubnets = true
	}
	cfg.BlockBatchLimit = ctx.Int(BlockBatchLimit.Name)
	cfg.BlockBatchLimitBurstFactor = ctx.Int(BlockBatchLimitBurstFactor.Name)
	cfg.BlobBatchLimit = ctx.Int(BlobBatchLimit.Name)
	cfg.BlobBatchLimitBurstFactor = ctx.Int(BlobBatchLimitBurstFactor.Name)
	cfg.MinimumPeersPerSubnet = ctx.Int(MinPeersPerSubnet.Name)
	cfg.MaxConcurrentDials = ctx.Int(MaxConcurrentDials.Name)
	configureMinimumPeers(ctx, cfg)

	Init(cfg)
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
