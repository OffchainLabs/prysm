package light_client

import (
	"fmt"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	lightclient "github.com/OffchainLabs/prysm/v6/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	lightclienttypes "github.com/OffchainLabs/prysm/v6/consensus-types/light-client"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/golang/snappy"
)

// RunLightClientUpdateRankingTests executes "light_client/update_ranking/pyspec_tests/update_ranking" tests.
func RunLightClientUpdateRankingTests(t *testing.T, config string, v int) {
	require.NoError(t, utils.SetConfig(t, config))
	if v >= version.Altair {
		params.BeaconConfig().AltairForkEpoch = 0
	}
	if v >= version.Bellatrix {
		params.BeaconConfig().BellatrixForkEpoch = 0
	}
	if v >= version.Capella {
		params.BeaconConfig().CapellaForkEpoch = 0
	}
	if v >= version.Deneb {
		params.BeaconConfig().DenebForkEpoch = 0
	}
	if v >= version.Electra {
		params.BeaconConfig().ElectraForkEpoch = 0
	}
	if v >= version.Fulu {
		params.BeaconConfig().FuluForkEpoch = 0
	}

	_, testsFolderPath := utils.TestFolders(t, config, version.String(v), "light_client/update_ranking/pyspec_tests/")
	testTypes, err := util.BazelListDirectories(testsFolderPath)
	require.NoError(t, err)

	if len(testTypes) == 0 {
		t.Fatalf("No test types found for %s", testsFolderPath)
	}
	if testTypes[0] != "update_ranking" {
		t.Fatalf("Expected test type 'update_ranking', got %s", testTypes[0])
	}

	_, testsFolderPath = utils.TestFolders(t, config, version.String(v), "light_client/update_ranking/pyspec_tests/update_ranking")
	helpers.ClearCache()
	t.Run("update ranking", func(t *testing.T) {
		runLightClientUpdateRankingTest(t, testsFolderPath, v)
	})
}

func runLightClientUpdateRankingTest(t *testing.T, testFolderPath string, v int) {
	metaFile, err := util.BazelFileBytes(path.Join(testFolderPath, "meta.yaml"))
	require.NoError(t, err)
	var meta struct {
		Count int `json:"updates_count"`
	}
	require.NoError(t, utils.UnmarshalYaml(metaFile, &meta))

	for i := 0; i < meta.Count-1; i++ {
		oldUpdateFile, err := util.BazelFileBytes(path.Join(testFolderPath, fmt.Sprintf("updates_%d.ssz_snappy", i)))
		require.NoError(t, err)
		oldUpdateSSZ, err := snappy.Decode(nil, oldUpdateFile)
		require.NoError(t, err, "Failed to decompress")
		oldUpdate := createUpdate(t, oldUpdateSSZ, v)

		newUpdateFile, err := util.BazelFileBytes(path.Join(testFolderPath, fmt.Sprintf("updates_%d.ssz_snappy", i+1)))
		require.NoError(t, err)
		newUpdateSSZ, err := snappy.Decode(nil, newUpdateFile)
		require.NoError(t, err, "Failed to decompress")
		newUpdate := createUpdate(t, newUpdateSSZ, v)

		result, err := lightclient.IsBetterUpdate(newUpdate, oldUpdate)
		require.NoError(t, err)
		require.Equal(t, false, result, "Update %d is not better than update %d", i, i+1)
	}
}

func createUpdate(t *testing.T, ssz []byte, v int) interfaces.LightClientUpdate {
	switch v {
	case version.Altair:
		updateBase := &ethpb.LightClientUpdateAltair{}
		require.NoError(t, updateBase.UnmarshalSSZ(ssz), "Failed to unmarshal")
		update, err := lightclienttypes.NewWrappedUpdateAltair(updateBase)
		require.NoError(t, err)
		return update
	case version.Bellatrix:
		updateBase := &ethpb.LightClientUpdateAltair{}
		require.NoError(t, updateBase.UnmarshalSSZ(ssz), "Failed to unmarshal")
		update, err := lightclienttypes.NewWrappedUpdateAltair(updateBase)
		require.NoError(t, err)
		return update
	case version.Capella:
		updateBase := &ethpb.LightClientUpdateCapella{}
		require.NoError(t, updateBase.UnmarshalSSZ(ssz), "Failed to unmarshal")
		update, err := lightclienttypes.NewWrappedUpdateCapella(updateBase)
		require.NoError(t, err)
		return update
	case version.Deneb:
		updateBase := &ethpb.LightClientUpdateDeneb{}
		require.NoError(t, updateBase.UnmarshalSSZ(ssz), "Failed to unmarshal")
		update, err := lightclienttypes.NewWrappedUpdateDeneb(updateBase)
		require.NoError(t, err)
		return update
	case version.Electra:
		updateBase := &ethpb.LightClientUpdateElectra{}
		require.NoError(t, updateBase.UnmarshalSSZ(ssz), "Failed to unmarshal")
		update, err := lightclienttypes.NewWrappedUpdateElectra(updateBase)
		require.NoError(t, err)
		return update
	default:
		t.Fatalf("Unsupported version %d", v)
		return nil
	}
}
