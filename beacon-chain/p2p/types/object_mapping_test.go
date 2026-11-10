package types

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestInitializeDataMaps_LightClientForks(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	InitializeDataMaps()
	cfg := params.BeaconConfig()
	for _, tt := range []struct {
		name                 string
		fork                 []byte
		optimistic, finality any
	}{
		{"altair", cfg.AltairForkVersion, &ethpb.LightClientOptimisticUpdateAltair{}, &ethpb.LightClientFinalityUpdateAltair{}},
		{"bellatrix", cfg.BellatrixForkVersion, &ethpb.LightClientOptimisticUpdateAltair{}, &ethpb.LightClientFinalityUpdateAltair{}},
		{"capella", cfg.CapellaForkVersion, &ethpb.LightClientOptimisticUpdateCapella{}, &ethpb.LightClientFinalityUpdateCapella{}},
		{"deneb", cfg.DenebForkVersion, &ethpb.LightClientOptimisticUpdateDeneb{}, &ethpb.LightClientFinalityUpdateDeneb{}},
		{"electra", cfg.ElectraForkVersion, &ethpb.LightClientOptimisticUpdateDeneb{}, &ethpb.LightClientFinalityUpdateElectra{}},
		{"fulu", cfg.FuluForkVersion, &ethpb.LightClientOptimisticUpdateDeneb{}, &ethpb.LightClientFinalityUpdateElectra{}},
		{"gloas", cfg.GloasForkVersion, &ethpb.LightClientOptimisticUpdateGloas{}, &ethpb.LightClientFinalityUpdateGloas{}},
		{"heze", cfg.HezeForkVersion, &ethpb.LightClientOptimisticUpdateGloas{}, &ethpb.LightClientFinalityUpdateGloas{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			optimisticFactory, ok := LightClientOptimisticUpdateMap[bytesutil.ToBytes4(tt.fork)]
			require.Equal(t, true, ok)
			optimistic, err := optimisticFactory()
			require.NoError(t, err)
			require.Equal(t, reflect.TypeOf(tt.optimistic), reflect.TypeOf(optimistic.Proto()))
			finalityFactory, ok := LightClientFinalityUpdateMap[bytesutil.ToBytes4(tt.fork)]
			require.Equal(t, true, ok)
			finality, err := finalityFactory()
			require.NoError(t, err)
			require.Equal(t, reflect.TypeOf(tt.finality), reflect.TypeOf(finality.Proto()))
		})
	}
}

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

func TestInitializeDataMaps_Gloas(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	InitializeDataMaps()

	gloasVersion := bytesutil.ToBytes4(params.BeaconConfig().GloasForkVersion)
	bFunc, ok := BlockMap[gloasVersion]
	require.Equal(t, true, ok)

	b, err := bFunc()
	require.NoError(t, err)
	assert.Equal(t, version.Gloas, b.Version())

	mdFunc, ok := MetaDataMap[gloasVersion]
	require.Equal(t, true, ok)
	md, err := mdFunc()
	require.NoError(t, err)
	assert.NotNil(t, md.MetadataObjV2())

	attFunc, ok := AttestationMap[gloasVersion]
	require.Equal(t, true, ok)
	att, err := attFunc()
	require.NoError(t, err)
	_, ok = att.(*ethpb.SingleAttestation)
	assert.Equal(t, true, ok)

	aggFunc, ok := AggregateAttestationMap[gloasVersion]
	require.Equal(t, true, ok)
	agg, err := aggFunc()
	require.NoError(t, err)
	_, ok = agg.(*ethpb.SignedAggregateAttestationAndProofGloas)
	assert.Equal(t, true, ok)
	assert.Equal(t, version.Gloas, agg.Version())
}
