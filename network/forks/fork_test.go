package forks

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
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
			name:        "no fork",
			targetEpoch: 0,
			want: &ethpb.Fork{
				Epoch:           0,
				CurrentVersion:  []byte{'A', 'B', 'C', 'D'},
				PreviousVersion: []byte{'A', 'B', 'C', 'D'},
			},
			wantErr: false,
			setConfg: func() {
				cfg = cfg.Copy()
				cfg.GenesisForkVersion = []byte{'A', 'B', 'C', 'D'}
				cfg.ForkVersionSchedule = map[[4]byte]primitives.Epoch{}
				params.OverrideBeaconConfig(cfg)
			},
		},
		{
			name:        "genesis fork",
			targetEpoch: 0,
			want: &ethpb.Fork{
				Epoch:           0,
				CurrentVersion:  []byte{'A', 'B', 'C', 'D'},
				PreviousVersion: []byte{'A', 'B', 'C', 'D'},
			},
			wantErr: false,
			setConfg: func() {
				cfg = cfg.Copy()
				cfg.GenesisForkVersion = []byte{'A', 'B', 'C', 'D'}
				cfg.ForkVersionSchedule = map[[4]byte]primitives.Epoch{
					{'A', 'B', 'C', 'D'}: 0,
				}
				params.OverrideBeaconConfig(cfg)
			},
		},
		{
			name:        "altair pre-fork",
			targetEpoch: 0,
			want: &ethpb.Fork{
				Epoch:           0,
				CurrentVersion:  []byte{'A', 'B', 'C', 'D'},
				PreviousVersion: []byte{'A', 'B', 'C', 'D'},
			},
			wantErr: false,
			setConfg: func() {
				cfg = cfg.Copy()
				cfg.GenesisForkVersion = []byte{'A', 'B', 'C', 'D'}
				cfg.AltairForkVersion = []byte{'A', 'B', 'C', 'F'}
				cfg.ForkVersionSchedule = map[[4]byte]primitives.Epoch{
					{'A', 'B', 'C', 'D'}: 0,
					{'A', 'B', 'C', 'F'}: 10,
				}
				params.OverrideBeaconConfig(cfg)
			},
		},
		{
			name:        "altair on fork",
			targetEpoch: 10,
			want: &ethpb.Fork{
				Epoch:           10,
				CurrentVersion:  []byte{'A', 'B', 'C', 'F'},
				PreviousVersion: []byte{'A', 'B', 'C', 'D'},
			},
			wantErr: false,
			setConfg: func() {
				cfg = cfg.Copy()
				cfg.GenesisForkVersion = []byte{'A', 'B', 'C', 'D'}
				cfg.AltairForkVersion = []byte{'A', 'B', 'C', 'F'}
				cfg.ForkVersionSchedule = map[[4]byte]primitives.Epoch{
					{'A', 'B', 'C', 'D'}: 0,
					{'A', 'B', 'C', 'F'}: 10,
				}
				params.OverrideBeaconConfig(cfg)
			},
		},

		{
			name:        "altair post fork",
			targetEpoch: 10,
			want: &ethpb.Fork{
				Epoch:           10,
				CurrentVersion:  []byte{'A', 'B', 'C', 'F'},
				PreviousVersion: []byte{'A', 'B', 'C', 'D'},
			},
			wantErr: false,
			setConfg: func() {
				cfg = cfg.Copy()
				cfg.GenesisForkVersion = []byte{'A', 'B', 'C', 'D'}
				cfg.AltairForkVersion = []byte{'A', 'B', 'C', 'F'}
				cfg.ForkVersionSchedule = map[[4]byte]primitives.Epoch{
					{'A', 'B', 'C', 'D'}: 0,
					{'A', 'B', 'C', 'F'}: 10,
				}
				params.OverrideBeaconConfig(cfg)
			},
		},

		{
			name:        "3 forks, pre-fork",
			targetEpoch: 20,
			want: &ethpb.Fork{
				Epoch:           10,
				CurrentVersion:  []byte{'A', 'B', 'C', 'F'},
				PreviousVersion: []byte{'A', 'B', 'C', 'D'},
			},
			wantErr: false,
			setConfg: func() {
				cfg = cfg.Copy()
				cfg.GenesisForkVersion = []byte{'A', 'B', 'C', 'D'}
				cfg.ForkVersionSchedule = map[[4]byte]primitives.Epoch{
					{'A', 'B', 'C', 'D'}: 0,
					{'A', 'B', 'C', 'F'}: 10,
					{'A', 'B', 'C', 'Z'}: 100,
				}
				params.OverrideBeaconConfig(cfg)
			},
		},
		{
			name:        "3 forks, on fork",
			targetEpoch: 100,
			want: &ethpb.Fork{
				Epoch:           100,
				CurrentVersion:  []byte{'A', 'B', 'C', 'Z'},
				PreviousVersion: []byte{'A', 'B', 'C', 'F'},
			},
			wantErr: false,
			setConfg: func() {
				cfg = cfg.Copy()
				cfg.GenesisForkVersion = []byte{'A', 'B', 'C', 'D'}
				cfg.ForkVersionSchedule = map[[4]byte]primitives.Epoch{
					{'A', 'B', 'C', 'D'}: 0,
					{'A', 'B', 'C', 'F'}: 10,
					{'A', 'B', 'C', 'Z'}: 100,
				}
				params.OverrideBeaconConfig(cfg)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setConfg()
			got, err := Fork(tt.targetEpoch)
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
