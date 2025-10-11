package params_test

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()

	tests := []struct {
		name        string
		targetEpoch primitives.Epoch
		want        *ethpb.Fork
		wantErr     bool
		setConfg    func()
	}{
		{
			name:        "genesis fork",
			targetEpoch: 0,
			want: &ethpb.Fork{
				Epoch:           cfg.GenesisEpoch,
				CurrentVersion:  cfg.GenesisForkVersion,
				PreviousVersion: cfg.GenesisForkVersion,
			},
		},
		{
			name:        "altair on fork",
			targetEpoch: cfg.AltairForkEpoch,
			want: &ethpb.Fork{
				Epoch:           cfg.AltairForkEpoch,
				CurrentVersion:  cfg.AltairForkVersion,
				PreviousVersion: cfg.GenesisForkVersion,
			},
		},
		{
			name:        "altair post fork",
			targetEpoch: cfg.CapellaForkEpoch + 1,
			want: &ethpb.Fork{
				Epoch:           cfg.CapellaForkEpoch,
				CurrentVersion:  cfg.CapellaForkVersion,
				PreviousVersion: cfg.BellatrixForkVersion,
			},
		},
		{
			name:        "3 forks, pre-fork",
			targetEpoch: cfg.ElectraForkEpoch - 1,
			want: &ethpb.Fork{
				Epoch:           cfg.DenebForkEpoch,
				CurrentVersion:  cfg.DenebForkVersion,
				PreviousVersion: cfg.CapellaForkVersion,
			},
		},
		{
			name:        "3 forks, on fork",
			targetEpoch: cfg.ElectraForkEpoch,
			want: &ethpb.Fork{
				Epoch:           cfg.ElectraForkEpoch,
				CurrentVersion:  cfg.ElectraForkVersion,
				PreviousVersion: cfg.DenebForkVersion,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := cfg.Copy()
			params.OverrideBeaconConfig(copied)
			got, err := params.Fork(tt.targetEpoch)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fork() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Fork() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetrieveForkDataFromDigest(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	digest := params.ForkDigest(params.BeaconConfig().AltairForkEpoch)
	version, epoch, err := params.ForkDataFromDigest(digest)
	require.NoError(t, err)
	require.Equal(t, [4]byte(params.BeaconConfig().AltairForkVersion), version)
	require.Equal(t, params.BeaconConfig().AltairForkEpoch, epoch)
}

func TestNextForkData(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	cfg := params.BeaconConfig()
	require.Equal(t, true, params.LastForkEpoch() < cfg.FarFutureEpoch)
	tests := []struct {
		name              string
		setConfg          func()
		currEpoch         primitives.Epoch
		wantedForkVersion [4]byte
		wantedEpoch       primitives.Epoch
	}{
		{
			name:              "genesis",
			currEpoch:         0,
			wantedForkVersion: [4]byte(cfg.AltairForkVersion),
			wantedEpoch:       cfg.AltairForkEpoch,
		},
		{
			name:              "altair pre-fork",
			currEpoch:         cfg.AltairForkEpoch - 1,
			wantedForkVersion: [4]byte(cfg.AltairForkVersion),
			wantedEpoch:       cfg.AltairForkEpoch,
		},
		{
			name:              "altair on fork",
			currEpoch:         cfg.AltairForkEpoch,
			wantedForkVersion: [4]byte(cfg.BellatrixForkVersion),
			wantedEpoch:       cfg.BellatrixForkEpoch,
		},

		{
			name:              "altair post fork",
			currEpoch:         cfg.AltairForkEpoch + 1,
			wantedForkVersion: [4]byte(cfg.BellatrixForkVersion),
			wantedEpoch:       cfg.BellatrixForkEpoch,
		},
		{
			name:              "after last bpo - should be far future epoch and 0x00000000",
			currEpoch:         params.LastForkEpoch() + 1,
			wantedForkVersion: [4]byte(cfg.ElectraForkVersion),
			wantedEpoch:       cfg.ElectraForkEpoch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params.OverrideBeaconConfig(cfg.Copy())
			fVersion, fEpoch := params.NextForkData(tt.currEpoch)
			if fVersion != tt.wantedForkVersion {
				t.Errorf("NextForkData() fork version = %v, want %v", fVersion, tt.wantedForkVersion)
			}
			if fEpoch != tt.wantedEpoch {
				t.Errorf("NextForkData() fork epoch = %v, want %v", fEpoch, tt.wantedEpoch)
			}
		})
	}
}

func TestLastForkEpoch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	if cfg.FuluForkEpoch == cfg.FarFutureEpoch {
		require.Equal(t, cfg.ElectraForkEpoch, params.LastForkEpoch())
	}
}

func TestForkFromConfig_UsesPassedConfig(t *testing.T) {
	testCfg := params.BeaconConfig().Copy()
	testCfg.AltairForkVersion = []byte{0x02, 0x00, 0x00, 0x00}
	testCfg.GenesisForkVersion = []byte{0x03, 0x00, 0x00, 0x00}
	testCfg.AltairForkEpoch = 100
	testCfg.InitializeForkSchedule()

	// Test at Altair fork epoch - should use the passed config's versions
	fork := params.ForkFromConfig(testCfg, testCfg.AltairForkEpoch)

	want := &ethpb.Fork{
		Epoch:           testCfg.AltairForkEpoch,
		CurrentVersion:  testCfg.AltairForkVersion,
		PreviousVersion: testCfg.GenesisForkVersion,
	}

	if !reflect.DeepEqual(fork, want) {
		t.Errorf("ForkFromConfig() got = %v, want %v", fork, want)
	}
}
