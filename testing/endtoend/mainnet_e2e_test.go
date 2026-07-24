package endtoend

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
)

// Run mainnet e2e config with the current release validator against latest beacon node.
func TestEndToEnd_MainnetConfig_ValidatorAtCurrentRelease(t *testing.T) {
	r := e2eMainnet(t, true, false, types.InitForkCfg(version.Bellatrix, version.Fulu, params.E2EMainnetTestConfig()))
	r.run()
}

func TestEndToEnd_MainnetConfig_MultiClient(t *testing.T) {
	const (
		electraForkEpoch = 0
		fuluForkEpoch    = 2

		BPO1ForkEpoch        = fuluForkEpoch + 2
		BPO1MaxBlobsPerBlock = 15

		BPO2ForkEpoch        = fuluForkEpoch + 4
		BPO2MaxBlobsPerBlock = 21
	)

	cfg := types.InitForkCfg(version.Electra, version.Fulu, params.E2EMainnetTestConfig())
	cfg.FuluForkEpoch = fuluForkEpoch
	cfg.BlobSchedule = []params.BlobScheduleEntry{
		{Epoch: electraForkEpoch, MaxBlobsPerBlock: uint64(cfg.DeprecatedMaxBlobsPerBlockElectra)},
		{Epoch: BPO1ForkEpoch, MaxBlobsPerBlock: BPO1MaxBlobsPerBlock},
		{Epoch: BPO2ForkEpoch, MaxBlobsPerBlock: BPO2MaxBlobsPerBlock},
	}
	cfg.InitializeForkSchedule()

	e2eMainnet(t, false, true, cfg, types.WithEpochs(8), types.WithLargeBlobs()).run()
}
