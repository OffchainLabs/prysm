package params

import (
	_ "embed"
	"sync"
)

//go:embed testdata/e2e_config.yaml
var e2eConfigYAML []byte

var (
	e2eConfigOnce sync.Once
	e2eConfig     *BeaconChainConfig
)

// E2ETestConfig retrieves the configurations made specifically for E2E testing.
// The config is loaded from testdata/e2e_config.yaml and cached.
//
// WARNING: This config is only for testing, it is not meant for use outside of E2E.
func E2ETestConfig() *BeaconChainConfig {
	e2eConfigOnce.Do(func() {
		cfg, err := UnmarshalConfig(e2eConfigYAML, nil)
		if err != nil {
			log.WithError(err).Fatal("Failed to load embedded e2e config")
		}
		e2eConfig = cfg
	})
	return e2eConfig
}

func E2EMainnetTestConfig() *BeaconChainConfig {
	e2eConfig := MainnetConfig()
	e2eConfig.DepositContractAddress = "0x4242424242424242424242424242424242424242"
	e2eConfig.Eth1FollowDistance = 8

	// Misc.
	e2eConfig.MinGenesisActiveValidatorCount = 256
	e2eConfig.GenesisDelay = 25 // 25 seconds so E2E has enough time to process deposits and get started.
	e2eConfig.ChurnLimitQuotient = 65536

	// Time parameters.
	e2eConfig.SecondsPerSlot = 6
	e2eConfig.SlotDurationMilliseconds = 6000
	e2eConfig.SqrRootSlotsPerEpoch = 5
	e2eConfig.SecondsPerETH1Block = 2
	e2eConfig.ShardCommitteePeriod = 4
	e2eConfig.MinValidatorWithdrawabilityDelay = 1

	// PoW parameters.
	e2eConfig.DepositChainID = 1337   // Chain ID of eth1 dev net.
	e2eConfig.DepositNetworkID = 1337 // Network ID of eth1 dev net.

	// Altair Fork Parameters.
	e2eConfig.AltairForkEpoch = E2ETestConfig().AltairForkEpoch
	e2eConfig.BellatrixForkEpoch = E2ETestConfig().BellatrixForkEpoch
	e2eConfig.CapellaForkEpoch = E2ETestConfig().CapellaForkEpoch
	e2eConfig.DenebForkEpoch = E2ETestConfig().DenebForkEpoch
	e2eConfig.ElectraForkEpoch = E2ETestConfig().ElectraForkEpoch
	e2eConfig.FuluForkEpoch = E2ETestConfig().FuluForkEpoch

	// Terminal Total Difficulty.
	e2eConfig.TerminalTotalDifficulty = "480"

	// Prysm constants.
	e2eConfig.ConfigName = EndToEndMainnetName
	e2eConfig.GenesisForkVersion = []byte{0, 0, 0, 254}
	e2eConfig.AltairForkVersion = []byte{1, 0, 0, 254}
	e2eConfig.BellatrixForkVersion = []byte{2, 0, 0, 254}
	e2eConfig.CapellaForkVersion = []byte{3, 0, 0, 254}
	e2eConfig.DenebForkVersion = []byte{4, 0, 0, 254}
	e2eConfig.ElectraForkVersion = []byte{5, 0, 0, 254}
	e2eConfig.FuluForkVersion = []byte{6, 0, 0, 254}

	// Deneb changes.
	e2eConfig.MinPerEpochChurnLimit = 2

	e2eConfig.BlobSchedule = []BlobScheduleEntry{
		{Epoch: e2eConfig.DenebForkEpoch, MaxBlobsPerBlock: uint64(e2eConfig.DeprecatedMaxBlobsPerBlock)},
		{Epoch: e2eConfig.ElectraForkEpoch, MaxBlobsPerBlock: uint64(e2eConfig.DeprecatedMaxBlobsPerBlockElectra)},
		// BPO (Blob Parameter Optimization) schedule for Fulu
		{Epoch: e2eConfig.FuluForkEpoch + 1, MaxBlobsPerBlock: 15},
		{Epoch: e2eConfig.FuluForkEpoch + 2, MaxBlobsPerBlock: 21},
	}

	e2eConfig.InitializeForkSchedule()
	return e2eConfig
}
