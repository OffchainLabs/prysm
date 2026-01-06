package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// TestEndToEnd_MinimalConfig is the pre-submit e2e test from Electra to Fulu
// with compressed epochs. It runs 10 epochs with exit at epoch 4 (the earliest
// possible due to ShardCommitteePeriod=4), allowing all evaluators to complete:
//   - Participation at epoch 2
//   - Finalization at epoch 3
//   - Fulu fork transition at epoch 2
//   - BPO 1 at epoch 3 (15 blobs)
//   - BPO 2 at epoch 4 (21 blobs)
//   - Exit proposed at epoch 4
//   - Exit confirmed at epoch 5
//   - Withdrawal submitted at epoch 5
//   - Withdrawal verified at epoch 8 (exit epoch 4 + 1 + MaxSeedLookahead + MinValidatorWithdrawabilityDelay + 1)
func TestEndToEnd_MinimalConfig(t *testing.T) {
	cfg := params.E2ETestConfig()
	cfg = types.InitForkCfg(version.Electra, version.Fulu, cfg)
	// Set Fulu fork at epoch 2 for a quick fork transition test
	cfg.FuluForkEpoch = 2
	// Update BlobSchedule to use the new FuluForkEpoch for BPO testing
	cfg.BlobSchedule = []params.BlobScheduleEntry{
		{Epoch: cfg.DenebForkEpoch, MaxBlobsPerBlock: uint64(cfg.DeprecatedMaxBlobsPerBlock)},
		{Epoch: cfg.ElectraForkEpoch, MaxBlobsPerBlock: uint64(cfg.DeprecatedMaxBlobsPerBlockElectra)},
		// BPO (Blob Parameter Optimization) schedule for Fulu
		{Epoch: cfg.FuluForkEpoch + 1, MaxBlobsPerBlock: 15},
		{Epoch: cfg.FuluForkEpoch + 2, MaxBlobsPerBlock: 21},
	}
	cfg.InitializeForkSchedule()

	r := e2eMinimal(t, cfg,
		types.WithCheckpointSync(),
		types.WithEpochs(10),
		types.WithExitEpoch(4),    // Minimum due to ShardCommitteePeriod=4
		types.WithLargeBlobs(),    // Use large blob transactions for BPO testing
	)
	r.run()
}