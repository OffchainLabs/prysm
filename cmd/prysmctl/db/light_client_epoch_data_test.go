package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	light_client "github.com/OffchainLabs/prysm/v7/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestEraEpochs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig().Copy()) // Altair fork epoch 74240 = 290*256

	tests := []struct {
		era         uint64
		first, last primitives.Epoch
		ok          bool
	}{
		{era: 0},
		{era: 290}, // ends at the Altair fork epoch
		{era: 291, first: 74241, last: 74496, ok: true}, // first era with epoch data
		{era: 1886, first: 482561, last: 482816, ok: true},
	}
	for _, tt := range tests {
		first, last, ok := eraEpochs(tt.era)
		require.Equal(t, tt.ok, ok, "era %d", tt.era)
		if ok {
			require.Equal(t, tt.first, first, "era %d", tt.era)
			require.Equal(t, tt.last, last, "era %d", tt.era)
		}
	}
	require.Equal(t, uint64(291), firstEra())

	// A fork epoch inside an era cuts the era's first epochs.
	cfg := params.MainnetConfig().Copy()
	cfg.AltairForkEpoch = 300
	params.OverrideBeaconConfig(cfg)
	first, last, ok := eraEpochs(2)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.Epoch(301), first)
	require.Equal(t, primitives.Epoch(512), last)
	require.Equal(t, uint64(2), firstEra())
}

type fakeEpochData map[primitives.Epoch]bool

func (f fakeEpochData) LightClientEpochData(_ context.Context, epoch primitives.Epoch) (light_client.EpochData, error) {
	if f[epoch] {
		return &ethpb.LightClientEpochDataElectra{Epoch: epoch}, nil
	}
	return nil, nil
}

func TestErasToReplay(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig().Copy())

	db := fakeEpochData{}
	for _, k := range []uint64{1000, 1001} {
		first, last, _ := eraEpochs(k)
		for e := first; e <= last; e++ {
			db[e] = true
		}
	}
	// Era 1001 misses its last epoch, era 1002 has none.
	_, last, _ := eraEpochs(1001)
	delete(db, last)

	todo, err := erasToReplay(t.Context(), db, 1000, 1002)
	require.NoError(t, err)
	require.DeepEqual(t, []uint64{1001, 1002}, todo)

	// Eras up to the Altair fork epoch have nothing to replay.
	todo, err = erasToReplay(t.Context(), fakeEpochData{}, 289, 291)
	require.NoError(t, err)
	require.DeepEqual(t, []uint64{291}, todo)
}

func TestSplitEras(t *testing.T) {
	eras := []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	require.DeepEqual(t, [][]uint64{{1, 2, 3}, {4, 5, 6}, {7, 8, 9, 10}}, splitEras(eras, 3))
	require.DeepEqual(t, [][]uint64{{1}, {2}}, splitEras([]uint64{1, 2}, 4))
}

type savedCall struct {
	period uint64
	epochs int
}

type fakeSaver struct {
	calls        []savedCall
	committeeErr error
}

func (f *fakeSaver) SaveLightClientEpochData(_ context.Context, data []light_client.EpochData) error {
	f.calls = append(f.calls, savedCall{epochs: len(data)})
	return nil
}

func (f *fakeSaver) SaveLightClientPeriodSyncCommittee(_ context.Context, period uint64, _ *ethpb.SyncCommittee) error {
	f.calls = append(f.calls, savedCall{period: period})
	return f.committeeErr
}

func TestSaveEpochData(t *testing.T) {
	data := []light_client.EpochData{&ethpb.LightClientEpochDataElectra{}, &ethpb.LightClientEpochDataElectra{}}
	committees := map[uint64]*ethpb.SyncCommittee{8: {}, 7: {}}

	db := &fakeSaver{}
	require.NoError(t, saveEpochData(t.Context(), db, data, committees))
	require.DeepEqual(t, []savedCall{{period: 7}, {period: 8}, {epochs: 2}}, db.calls)

	// Without the committees, the epoch data is not saved, so the era stays incomplete.
	db = &fakeSaver{committeeErr: errors.New("disk full")}
	require.ErrorContains(t, "disk full", saveEpochData(t.Context(), db, data, committees))
	require.DeepEqual(t, []savedCall{{period: 7}}, db.calls)
}

func TestRemoveUnneeded(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.era", "b.era.part", "c.era", "c.era.part"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}
	require.NoError(t, removeUnneeded(dir, map[string]bool{"a.era": true, "b.era": true}))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	slices.Sort(left)
	require.DeepEqual(t, []string{"a.era", "b.era.part"}, left)
}
