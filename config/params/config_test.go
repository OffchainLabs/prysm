package params_test

import (
	"bytes"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/genesis"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
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

func TestMaxBlobsJumbled(t *testing.T) {
	params.SetActiveTestCleanup(t, params.MainnetBeaconConfig)
	cfg := params.MainnetConfig()
	cfg.FuluForkEpoch = cfg.ElectraForkEpoch + 4098*2
	electraMaxBlobs := uint64(cfg.DeprecatedMaxBlobsPerBlockElectra)
	offsets := []primitives.Epoch{cfg.FuluForkEpoch}
	for _, offset := range []primitives.Epoch{320, 640, 960, 1080} {
		offsets = append(offsets, cfg.FuluForkEpoch+offset)
	}
	maxBlobs := map[primitives.Epoch]uint64{
		cfg.FuluForkEpoch: electraMaxBlobs,
		offsets[0]:        electraMaxBlobs + 3,
		offsets[1]:        electraMaxBlobs + 6,
		offsets[2]:        electraMaxBlobs + 9,
		offsets[3]:        electraMaxBlobs + 12,
	}
	schedule := make([]params.BlobScheduleEntry, 0, len(maxBlobs))
	for _, epoch := range offsets[1:] {
		schedule = append(schedule, params.BlobScheduleEntry{Epoch: epoch, MaxBlobsPerBlock: maxBlobs[epoch]})
	}
	cfg.BlobSchedule = schedule
	cfg.InitializeForkSchedule()
	for i := 1; i < len(cfg.BlobSchedule); i++ {
		beforeEpoch, epoch := cfg.BlobSchedule[i-1].Epoch, cfg.BlobSchedule[i].Epoch
		before, after := maxBlobs[beforeEpoch], maxBlobs[epoch]
		require.Equal(t, before, uint64(cfg.MaxBlobsPerBlockAtEpoch(epoch-1)))
		require.Equal(t, after, uint64(cfg.MaxBlobsPerBlockAtEpoch(epoch)))
		beforeSlot, err := cfg.SlotsPerEpoch.SafeMul(uint64(beforeEpoch))
		require.NoError(t, err)
		afterSlot, err := cfg.SlotsPerEpoch.SafeMul(uint64(epoch))
		require.NoError(t, err)
		require.Equal(t, before, uint64(cfg.MaxBlobsPerBlock(beforeSlot)))
		require.Equal(t, after, uint64(cfg.MaxBlobsPerBlock(afterSlot)))
	}

	require.Equal(t, electraMaxBlobs, uint64(cfg.MaxBlobsPerBlockAtEpoch(cfg.FuluForkEpoch-1)))
	require.Equal(t, electraMaxBlobs, uint64(cfg.MaxBlobsPerBlockAtEpoch(cfg.ElectraForkEpoch)))
	require.Equal(t, cfg.DeprecatedMaxBlobsPerBlock, cfg.MaxBlobsPerBlockAtEpoch(cfg.ElectraForkEpoch-1))
	require.Equal(t, cfg.DeprecatedMaxBlobsPerBlock, cfg.MaxBlobsPerBlockAtEpoch(cfg.DenebForkEpoch))
	preBlobEpochs := []primitives.Epoch{cfg.DenebForkEpoch - 1, cfg.CapellaForkEpoch, cfg.BellatrixForkEpoch, cfg.AltairForkEpoch, 0}
	for _, epoch := range preBlobEpochs {
		require.Equal(t, 0, cfg.MaxBlobsPerBlockAtEpoch(epoch))
	}
}

func TestFirstBPOAtFork(t *testing.T) {
	params.SetActiveTestCleanup(t, params.MainnetBeaconConfig)
	cfg := params.MainnetConfig()
	cfg.FuluForkEpoch = cfg.ElectraForkEpoch + 4096*2
	electraMaxBlobs := uint64(cfg.DeprecatedMaxBlobsPerBlockElectra)
	cfg.BlobSchedule = []params.BlobScheduleEntry{
		{Epoch: cfg.FuluForkEpoch, MaxBlobsPerBlock: electraMaxBlobs + 1},
		{Epoch: cfg.FuluForkEpoch + 1, MaxBlobsPerBlock: electraMaxBlobs + 2},
	}
	cfg.InitializeForkSchedule()
	require.Equal(t, electraMaxBlobs, uint64(cfg.MaxBlobsPerBlockAtEpoch(cfg.FuluForkEpoch-1)))
	require.Equal(t, electraMaxBlobs+1, uint64(cfg.MaxBlobsPerBlockAtEpoch(cfg.FuluForkEpoch)))
	require.Equal(t, electraMaxBlobs+2, uint64(cfg.MaxBlobsPerBlockAtEpoch(cfg.FuluForkEpoch+2)))
}

func TestMaxBlobsNoSchedule(t *testing.T) {
	params.SetActiveTestCleanup(t, params.MainnetBeaconConfig)
	cfg := params.MainnetConfig()
	electraMaxBlobs := uint64(cfg.DeprecatedMaxBlobsPerBlockElectra)
	cfg.BlobSchedule = nil
	cfg.InitializeForkSchedule()
	require.Equal(t, electraMaxBlobs, uint64(cfg.MaxBlobsPerBlockAtEpoch(cfg.FuluForkEpoch-1)))
	require.Equal(t, electraMaxBlobs, uint64(cfg.MaxBlobsPerBlockAtEpoch(cfg.ElectraForkEpoch)))
	require.Equal(t, cfg.DeprecatedMaxBlobsPerBlock, cfg.MaxBlobsPerBlockAtEpoch(cfg.ElectraForkEpoch-1))
	require.Equal(t, cfg.DeprecatedMaxBlobsPerBlock, cfg.MaxBlobsPerBlockAtEpoch(cfg.DenebForkEpoch))
	preBlobEpochs := []primitives.Epoch{cfg.DenebForkEpoch - 1, cfg.CapellaForkEpoch, cfg.BellatrixForkEpoch, cfg.AltairForkEpoch, 0}
	for _, epoch := range preBlobEpochs {
		require.Equal(t, 0, cfg.MaxBlobsPerBlockAtEpoch(epoch))
	}
}

func Test_TargetBlobCount(t *testing.T) {
	cfg := params.MainnetConfig()
	cfg.ElectraForkEpoch = 10
	require.Equal(t, cfg.TargetBlobsPerBlock(primitives.Slot(cfg.ElectraForkEpoch)*cfg.SlotsPerEpoch-1), 3)
	require.Equal(t, cfg.TargetBlobsPerBlock(primitives.Slot(cfg.ElectraForkEpoch)*cfg.SlotsPerEpoch), 6)
	cfg.ElectraForkEpoch = math.MaxUint64
}

func fillGVR(value byte) [32]byte {
	var gvr [32]byte
	for i := 0; i < len(gvr); i++ {
		gvr[i] = value
	}
	return gvr
}

func TestEntryWithForkDigest(t *testing.T) {
	var zero [32]byte
	one := fillGVR(byte(1))
	two := fillGVR(byte(2))
	three := fillGVR(byte(3))
	configs := map[[32]byte]*params.BeaconChainConfig{
		zero:  testConfigForSchedule(zero),
		one:   testConfigForSchedule(one),
		two:   testConfigForSchedule(two),
		three: testConfigForSchedule(three),
	}
	for _, cfg := range configs {
		cfg.InitializeForkSchedule()
	}
	cases := []struct {
		epoch    primitives.Epoch
		gvr      [32]byte
		expected string
	}{
		{epoch: 9, expected: "0x97b2c268"},
		{epoch: 10, expected: "0x97b2c268"},
		{epoch: 11, expected: "0x97b2c268"},
		{epoch: 99, expected: "0x97b2c268"},
		{epoch: 100, expected: "0x44a571e8"},
		{epoch: 101, expected: "0x44a571e8"},
		{epoch: 150, expected: "0x1171afca"},
		{epoch: 199, expected: "0x1171afca"},
		{epoch: 200, expected: "0x427a30ab"},
		{epoch: 201, expected: "0x427a30ab"},
		{epoch: 250, expected: "0xd5310ef1"},
		{epoch: 299, expected: "0xd5310ef1"},
		{epoch: 300, expected: "0x51d229f7"},
		{epoch: 301, expected: "0x51d229f7"},
		{epoch: 9, gvr: fillGVR(byte(1)), expected: "0x4a5c3011"},
		{epoch: 9, gvr: fillGVR(byte(2)), expected: "0xe8332b52"},
		{epoch: 9, gvr: fillGVR(byte(3)), expected: "0x0e38e75e"},
		{epoch: 100, gvr: fillGVR(byte(1)), expected: "0xbfe98545"},
		{epoch: 100, gvr: fillGVR(byte(2)), expected: "0x9b7e4788"},
		{epoch: 100, gvr: fillGVR(byte(3)), expected: "0x8b5ce4af"},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_%s", c.epoch, c.expected), func(t *testing.T) {
			var expected [4]byte
			err := hexutil.UnmarshalFixedText("ForkDigest", []byte(c.expected), expected[:])
			require.NoError(t, err)
			cfg := configs[c.gvr]
			digest := params.ForkDigestUsingConfig(c.epoch, cfg)
			require.Equal(t, expected, digest)
		})
	}
}

func testConfigForSchedule(gvr [32]byte) *params.BeaconChainConfig {
	cfg := params.MinimalSpecConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.BellatrixForkEpoch = 0
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 9
	cfg.FuluForkEpoch = 100
	cfg.GenesisValidatorsRoot = gvr
	cfg.BlobSchedule = []params.BlobScheduleEntry{
		{Epoch: 100, MaxBlobsPerBlock: 100},
		{Epoch: 150, MaxBlobsPerBlock: 175},
		{Epoch: 200, MaxBlobsPerBlock: 200},
		{Epoch: 250, MaxBlobsPerBlock: 275},
		{Epoch: 300, MaxBlobsPerBlock: 300},
	}
	return cfg
}

func TestFilterFarFuture(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().FuluForkEpoch = params.BeaconConfig().FarFutureEpoch
	params.BeaconConfig().InitializeForkSchedule()
	last := params.LastNetworkScheduleEntry()
	require.Equal(t, [4]byte(params.BeaconConfig().ElectraForkVersion), last.ForkVersion)
}

func TestFarFuturePrepareFilter(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	oldElectra := cfg.ElectraForkEpoch
	// This should cause electra to be filtered from the schedule, so looking up the entry for electra's epoch
	// should return the previous entry (deneb).
	cfg.ElectraForkEpoch = params.BeaconConfig().FarFutureEpoch
	cfg.FuluForkEpoch = params.BeaconConfig().FarFutureEpoch
	params.OverrideBeaconConfig(cfg)
	entry := params.GetNetworkScheduleEntry(oldElectra)
	require.Equal(t, [4]byte(params.BeaconConfig().DenebForkVersion), entry.ForkVersion)
}

func TestMaxBlobsOverrideEpoch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	require.Equal(t, 0, cfg.MaxBlobsPerBlockAtEpoch(0))
	params.SetGenesisFork(t, cfg, version.Deneb)
	require.Equal(t, cfg.DeprecatedMaxBlobsPerBlock, cfg.MaxBlobsPerBlockAtEpoch(0))
	params.SetGenesisFork(t, cfg, version.Electra)
	require.Equal(t, cfg.DeprecatedMaxBlobsPerBlockElectra, cfg.MaxBlobsPerBlockAtEpoch(0))
	params.SetGenesisFork(t, cfg, version.Fulu)
	require.Equal(t, cfg.DeprecatedMaxBlobsPerBlockElectra, cfg.MaxBlobsPerBlockAtEpoch(0))
}
