package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	ev "github.com/OffchainLabs/prysm/v7/testing/endtoend/evaluators"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// TestEndToEnd_MinimalConfig is the pre-submit e2e test. It starts from a Fulu
// genesis (no in-test fork transition) with compressed epochs: 10 epochs with exit
// at epoch 4 (the earliest possible due to ShardCommitteePeriod=4), allowing all
// evaluators to complete:
//   - Participation at epoch 2
//   - Finalization at epoch 3
//   - BPO 1 at epoch 3 (15 blobs)
//   - BPO 2 at epoch 4 (21 blobs)
//   - Exit proposed at epoch 4
//   - Exit confirmed at epoch 5
//   - Withdrawal submitted at epoch 5
//   - Withdrawal verified at epoch 8 (exit epoch 4 + 1 + MaxSeedLookahead + MinValidatorWithdrawabilityDelay + 1)
func TestEndToEnd_MinimalConfig(t *testing.T) {
	cfg := params.E2ETestConfig()
	cfg = types.InitForkCfg(version.Fulu, version.Fulu, cfg)
	// BPO (Blob Parameter Optimization) schedule. Bumps are kept at epochs 3 and 4
	// (the same absolute epochs as the previous Electra->Fulu setup) so blob timing
	// is unchanged; the base limit at genesis is Electra's.
	cfg.BlobSchedule = []params.BlobScheduleEntry{
		{Epoch: 0, MaxBlobsPerBlock: uint64(cfg.DeprecatedMaxBlobsPerBlockElectra)},
		{Epoch: 3, MaxBlobsPerBlock: 15},
		{Epoch: 4, MaxBlobsPerBlock: 21},
	}
	cfg.InitializeForkSchedule()

	r := e2eMinimal(t, cfg,
		types.WithCheckpointSync(),
		types.WithEpochs(10),
		types.WithExitEpoch(4), // Minimum due to ShardCommitteePeriod=4
		types.WithLargeBlobs(), // Use large blob transactions for BPO testing
		// With a Fulu genesis FuluForkEpoch is 0, so addIfForkSet skips the BPO
		// evaluator; register it explicitly to preserve the prior coverage.
		types.WithExtraEvaluators(ev.BlobLimitsRespected),
	)
	r.run()
}
