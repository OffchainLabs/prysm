package params_test

import (
	"bytes"
	"math"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/genesis"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

// Test cases can be executed in an arbitrary order. TestOverrideBeaconConfigTestTeardown checks
// that there's no state mutation leak from the previous test, therefore we need a sentinel flag,
// to make sure that previous test case has already been completed and check can be run.
var testOverrideBeaconConfigExecuted bool

func TestConfig_OverrideBeaconConfig(t *testing.T) {
	// Ensure that param modifications are safe.
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.SlotsPerEpoch = 5
	params.OverrideBeaconConfig(cfg)
	if c := params.BeaconConfig(); c.SlotsPerEpoch != 5 {
		t.Errorf("Shardcount in BeaconConfig incorrect. Wanted %d, got %d", 5, c.SlotsPerEpoch)
	}
	testOverrideBeaconConfigExecuted = true
}

func TestConfig_OverrideBeaconConfigTestTeardown(t *testing.T) {
	if !testOverrideBeaconConfigExecuted {
		t.Skip("State leak can occur only if state mutating test has already completed")
	}
	cfg := params.BeaconConfig()
	if cfg.SlotsPerEpoch == 5 {
		t.Fatal("Parameter update has been leaked out of previous test")
	}
}

func TestConfig_DataRace(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	wg := new(sync.WaitGroup)
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			cfg := params.BeaconConfig()
			params.OverrideBeaconConfig(cfg)
		}()
		go func() uint64 {
			defer wg.Done()
			return params.BeaconConfig().MaxDeposits
		}()
	}
	wg.Wait()
}

func TestConfig_WithinDAPeriod(t *testing.T) {
	cases := []struct {
		name    string
		block   primitives.Epoch
		current primitives.Epoch
		within  bool
	}{
		{
			name:    "before",
			block:   0,
			current: params.BeaconConfig().MinEpochsForBlobsSidecarsRequest + 1,
			within:  false,
		},
		{
			name:    "same",
			block:   0,
			current: 0,
			within:  true,
		},
		{
			name:    "boundary",
			block:   0,
			current: params.BeaconConfig().MinEpochsForBlobsSidecarsRequest,
			within:  true,
		},
		{
			name:    "one less",
			block:   params.BeaconConfig().MinEpochsForBlobsSidecarsRequest - 1,
			current: params.BeaconConfig().MinEpochsForBlobsSidecarsRequest,
			within:  true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.within, params.WithinDAPeriod(c.block, c.current))
		})
	}
}

func TestConfigGenesisValidatorRoot(t *testing.T) {
	params.SetActiveTestCleanup(t, params.MainnetBeaconConfig)
	genesis.StoreEmbeddedDuringTest(t, params.BeaconConfig().ConfigName)
	g, err := genesis.State()
	require.NoError(t, err, "failed to load genesis state")
	if !bytes.Equal(g.GenesisValidatorsRoot(), params.BeaconConfig().GenesisValidatorsRoot[:]) {
		t.Fatal("mainnet params genesis validator root does not match the mainnet genesis state value")
	}
	require.Equal(t, params.BeaconConfig().GenesisValidatorsRoot, genesis.ValidatorsRoot())
}

func TestMaxBlobsPerBlock(t *testing.T) {
	t.Run("Before all forks and no BlobSchedule", func(t *testing.T) {
		cfg := params.MainnetConfig()
		cfg.BlobSchedule = nil
		cfg.ElectraForkEpoch = 100
		cfg.FuluForkEpoch = 200
		require.Equal(t, cfg.MaxBlobsPerBlock(0), cfg.DeprecatedMaxBlobsPerBlock)
	})

	t.Run("Uses latest matching BlobSchedule entry", func(t *testing.T) {
		cfg := params.MainnetConfig()
		cfg.BlobSchedule = []params.BlobScheduleEntry{
			{Epoch: 5, MaxBlobsPerBlock: 7},
			{Epoch: 10, MaxBlobsPerBlock: 11},
		}
		slot := 11 * cfg.SlotsPerEpoch
		require.Equal(t, cfg.MaxBlobsPerBlock(slot), 11)
	})

	t.Run("Uses earlier matching BlobSchedule entry", func(t *testing.T) {
		cfg := params.MainnetConfig()
		cfg.BlobSchedule = []params.BlobScheduleEntry{
			{Epoch: 5, MaxBlobsPerBlock: 7},
			{Epoch: 10, MaxBlobsPerBlock: 11},
		}
		slot := 6 * cfg.SlotsPerEpoch
		require.Equal(t, cfg.MaxBlobsPerBlock(slot), 7)
	})

	t.Run("Before first BlobSchedule entry falls back to fork logic", func(t *testing.T) {
		cfg := params.MainnetConfig()
		cfg.FuluForkEpoch = 1
		cfg.BlobSchedule = []params.BlobScheduleEntry{
			{Epoch: 5, MaxBlobsPerBlock: 7},
		}
		slot := primitives.Slot(2) // Epoch 0
		require.Equal(t, cfg.MaxBlobsPerBlock(slot), cfg.DeprecatedMaxBlobsPerBlock)
	})

	t.Run("Unsorted BlobSchedule still picks latest matching entry", func(t *testing.T) {
		cfg := params.MainnetConfig()
		cfg.BlobSchedule = []params.BlobScheduleEntry{
			{Epoch: 10, MaxBlobsPerBlock: 11},
			{Epoch: 5, MaxBlobsPerBlock: 7},
		}
		slot := 11 * cfg.SlotsPerEpoch
		require.Equal(t, cfg.MaxBlobsPerBlock(slot), 11)
	})

	t.Run("Unsorted BlobSchedule picks earlier matching entry correctly", func(t *testing.T) {
		cfg := params.MainnetConfig()
		cfg.BlobSchedule = []params.BlobScheduleEntry{
			{Epoch: 10, MaxBlobsPerBlock: 11},
			{Epoch: 5, MaxBlobsPerBlock: 7},
		}
		slot := 6 * cfg.SlotsPerEpoch
		require.Equal(t, cfg.MaxBlobsPerBlock(slot), 7)
	})

	t.Run("Unsorted BlobSchedule falls back to fork logic when epoch is before all entries", func(t *testing.T) {
		cfg := params.MainnetConfig()
		cfg.ElectraForkEpoch = 2
		cfg.BlobSchedule = []params.BlobScheduleEntry{
			{Epoch: 10, MaxBlobsPerBlock: 11},
			{Epoch: 5, MaxBlobsPerBlock: 7},
		}
		slot := primitives.Slot(1) // Epoch 0
		require.Equal(t, cfg.MaxBlobsPerBlock(slot), cfg.DeprecatedMaxBlobsPerBlock)
	})
}

func Test_TargetBlobCount(t *testing.T) {
	cfg := params.MainnetConfig()
	cfg.ElectraForkEpoch = 10
	require.Equal(t, cfg.TargetBlobsPerBlock(primitives.Slot(cfg.ElectraForkEpoch)*cfg.SlotsPerEpoch-1), 3)
	require.Equal(t, cfg.TargetBlobsPerBlock(primitives.Slot(cfg.ElectraForkEpoch)*cfg.SlotsPerEpoch), 6)
	cfg.ElectraForkEpoch = math.MaxUint64
}
