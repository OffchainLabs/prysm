package types

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestInitializeDataMaps(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	tests := []struct {
		name   string
		action func()
		exists bool
	}{
		{
			name: "no change",
			action: func() {
			},
			exists: true,
		},
		{
			name: "fork version changes",
			action: func() {
				cfg := params.BeaconConfig().Copy()
				cfg.GenesisForkVersion = []byte{0x01, 0x02, 0x00, 0x00}
				params.OverrideBeaconConfig(cfg)
			},
			exists: false,
		},
		{
			name: "fork version changes with reset",
			action: func() {
				cfg := params.BeaconConfig().Copy()
				cfg.GenesisForkVersion = []byte{0x01, 0x02, 0x00, 0x00}
				params.OverrideBeaconConfig(cfg)
				InitializeDataMaps()
			},
			exists: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.action()
			bFunc, ok := BlockMap[bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)]
			assert.Equal(t, tt.exists, ok)
			if tt.exists {
				b, err := bFunc()
				require.NoError(t, err)
				generic, err := b.PbGenericBlock()
				require.NoError(t, err)
				assert.NotNil(t, generic.GetPhase0())
			}
			mdFunc, ok := MetaDataMap[bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)]
			if tt.exists {
				md, err := mdFunc()
				require.NoError(t, err)
				assert.NotNil(t, md.MetadataObjV0())
			}
			assert.Equal(t, tt.exists, ok)
			attFunc, ok := AttestationMap[bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)]
			if tt.exists {
				att, err := attFunc()
				require.NoError(t, err)
				assert.Equal(t, version.Phase0, att.Version())
			}
			assert.Equal(t, tt.exists, ok)
			aggFunc, ok := AggregateAttestationMap[bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)]
			assert.Equal(t, tt.exists, ok)
			if tt.exists {
				agg, err := aggFunc()
				require.NoError(t, err)
				assert.Equal(t, version.Phase0, agg.Version())
			}
			attSlashFunc, ok := AttesterSlashingMap[bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)]
			assert.Equal(t, tt.exists, ok)
			if tt.exists {
				attSlash, err := attSlashFunc()
				require.NoError(t, err)
				assert.Equal(t, version.Phase0, attSlash.Version())
			}
		})
	}
}
