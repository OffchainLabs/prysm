package sync

import (
	"context"
	"testing"

	p2ptest "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
)

// createMinimalService creates a minimal Service instance for testing
func createMinimalService(t *testing.T) *Service {
	p2pService := p2ptest.NewTestP2P(t)
	return &Service{
		cfg: &config{
			p2p: p2pService,
		},
		ctx: context.Background(),
	}
}

func TestTopicFamiliesForEpoch(t *testing.T) {
	// Define test epochs
	const (
		genesisEpoch   = primitives.Epoch(0)
		altairEpoch    = primitives.Epoch(100)
		bellatrixEpoch = primitives.Epoch(200)
		capellaEpoch   = primitives.Epoch(300)
		denebEpoch     = primitives.Epoch(400)
		electraEpoch   = primitives.Epoch(500)
		fuluEpoch      = primitives.Epoch(600)
	)

	// Define topic families for each fork
	// These names must match what's returned by the Name() method of each topic family
	genesisFamilies := []string{
		"BlockTopicFamily",
		"AggregateAndProofTopicFamily",
		"VoluntaryExitTopicFamily",
		"ProposerSlashingTopicFamily",
		"AttesterSlashingTopicFamily",
		"AttestationTopicFamily",
	}

	altairFamilies := []string{
		"SyncContributionAndProofTopicFamily",
		"SyncCommitteeTopicFamily",
	}

	altairLightClientFamilies := []string{
		"LightClientOptimisticUpdateTopicFamily",
		"LightClientFinalityUpdateTopicFamily",
	}

	capellaFamilies := []string{
		"BlsToExecutionChangeTopicFamily",
	}

	denebBlobFamilies := []string{
		"BlobTopicFamily-0",
		"BlobTopicFamily-1",
		"BlobTopicFamily-2",
		"BlobTopicFamily-3",
		"BlobTopicFamily-4",
		"BlobTopicFamily-5",
	}

	electraBlobFamilies := denebBlobFamilies
	electraBlobFamilies = append(denebBlobFamilies, "BlobTopicFamily-6", "BlobTopicFamily-7")

	fuluFamilies := []string{
		"DataColumnTopicFamily",
	}

	// Helper function to combine fork families
	combineForks := func(forkSets ...[]string) []string {
		var combined []string
		for _, forkSet := range forkSets {
			combined = append(combined, forkSet...)
		}
		return combined
	}

	tests := []struct {
		name              string
		epoch             primitives.Epoch
		setupConfig       func()
		enableLightClient bool
		expectedFamilies  []string
	}{
		{
			name:  "epoch before any fork activation should return empty",
			epoch: primitives.Epoch(0),
			setupConfig: func() {
				config := params.BeaconConfig().Copy()
				// Set all fork epochs to future epochs
				config.GenesisEpoch = primitives.Epoch(1000)
				config.AltairForkEpoch = primitives.Epoch(2000)
				config.BellatrixForkEpoch = primitives.Epoch(3000)
				config.CapellaForkEpoch = primitives.Epoch(4000)
				config.DenebForkEpoch = primitives.Epoch(5000)
				config.ElectraForkEpoch = primitives.Epoch(6000)
				config.FuluForkEpoch = primitives.Epoch(7000)
				params.OverrideBeaconConfig(config)
			},
			expectedFamilies: []string{},
		},
		{
			name:  "epoch at genesis should return genesis topic families",
			epoch: genesisEpoch,
			setupConfig: func() {
				config := params.BeaconConfig().Copy()
				config.GenesisEpoch = genesisEpoch
				config.AltairForkEpoch = altairEpoch
				config.BellatrixForkEpoch = bellatrixEpoch
				config.CapellaForkEpoch = capellaEpoch
				config.DenebForkEpoch = denebEpoch
				config.ElectraForkEpoch = electraEpoch
				config.FuluForkEpoch = fuluEpoch
				params.OverrideBeaconConfig(config)
			},
			expectedFamilies: genesisFamilies,
		},
		{
			name:              "epoch at Altair without light client should have genesis + Altair families",
			epoch:             altairEpoch,
			enableLightClient: false,
			setupConfig: func() {
				config := params.BeaconConfig().Copy()
				config.GenesisEpoch = genesisEpoch
				config.AltairForkEpoch = altairEpoch
				config.BellatrixForkEpoch = bellatrixEpoch
				config.CapellaForkEpoch = capellaEpoch
				config.DenebForkEpoch = denebEpoch
				config.ElectraForkEpoch = electraEpoch
				config.FuluForkEpoch = fuluEpoch
				params.OverrideBeaconConfig(config)
			},
			expectedFamilies: combineForks(genesisFamilies, altairFamilies),
		},
		{
			name:              "epoch at Altair with light client enabled should include light client families",
			epoch:             altairEpoch,
			enableLightClient: true,
			setupConfig: func() {
				config := params.BeaconConfig().Copy()
				config.GenesisEpoch = genesisEpoch
				config.AltairForkEpoch = altairEpoch
				config.BellatrixForkEpoch = bellatrixEpoch
				config.CapellaForkEpoch = capellaEpoch
				config.DenebForkEpoch = denebEpoch
				config.ElectraForkEpoch = electraEpoch
				config.FuluForkEpoch = fuluEpoch
				params.OverrideBeaconConfig(config)
			},
			expectedFamilies: combineForks(genesisFamilies, altairFamilies, altairLightClientFamilies),
		},
		{
			name:  "epoch at Capella should have genesis + Altair + Capella families",
			epoch: capellaEpoch,
			setupConfig: func() {
				config := params.BeaconConfig().Copy()
				config.GenesisEpoch = genesisEpoch
				config.AltairForkEpoch = altairEpoch
				config.BellatrixForkEpoch = bellatrixEpoch
				config.CapellaForkEpoch = capellaEpoch
				config.DenebForkEpoch = denebEpoch
				config.ElectraForkEpoch = electraEpoch
				config.FuluForkEpoch = fuluEpoch
				params.OverrideBeaconConfig(config)
			},
			expectedFamilies: combineForks(genesisFamilies, altairFamilies, capellaFamilies),
		},
		{
			name:  "epoch at Deneb should include blob sidecars",
			epoch: denebEpoch,
			setupConfig: func() {
				config := params.BeaconConfig().Copy()
				config.GenesisEpoch = genesisEpoch
				config.AltairForkEpoch = altairEpoch
				config.BellatrixForkEpoch = bellatrixEpoch
				config.CapellaForkEpoch = capellaEpoch
				config.DenebForkEpoch = denebEpoch
				config.ElectraForkEpoch = electraEpoch
				config.FuluForkEpoch = fuluEpoch
				config.BlobsidecarSubnetCount = 6 // Deneb has 6 blob subnets
				params.OverrideBeaconConfig(config)
			},
			expectedFamilies: combineForks(genesisFamilies, altairFamilies, capellaFamilies, denebBlobFamilies),
		},
		{
			name:  "epoch at Electra should have Electra blobs not Deneb blobs",
			epoch: electraEpoch,
			setupConfig: func() {
				config := params.BeaconConfig().Copy()
				config.GenesisEpoch = genesisEpoch
				config.AltairForkEpoch = altairEpoch
				config.BellatrixForkEpoch = bellatrixEpoch
				config.CapellaForkEpoch = capellaEpoch
				config.DenebForkEpoch = denebEpoch
				config.ElectraForkEpoch = electraEpoch
				config.FuluForkEpoch = fuluEpoch
				config.BlobsidecarSubnetCount = 6
				config.BlobsidecarSubnetCountElectra = 8 // Electra has 8 blob subnets
				params.OverrideBeaconConfig(config)
			},
			expectedFamilies: combineForks(genesisFamilies, altairFamilies, capellaFamilies, electraBlobFamilies),
		},
		{
			name:  "epoch at Fulu should have data columns not blobs",
			epoch: fuluEpoch,
			setupConfig: func() {
				config := params.BeaconConfig().Copy()
				config.GenesisEpoch = genesisEpoch
				config.AltairForkEpoch = altairEpoch
				config.BellatrixForkEpoch = bellatrixEpoch
				config.CapellaForkEpoch = capellaEpoch
				config.DenebForkEpoch = denebEpoch
				config.ElectraForkEpoch = electraEpoch
				config.FuluForkEpoch = fuluEpoch
				config.BlobsidecarSubnetCount = 6
				config.BlobsidecarSubnetCountElectra = 8
				params.OverrideBeaconConfig(config)
			},
			expectedFamilies: combineForks(genesisFamilies, altairFamilies, capellaFamilies, fuluFamilies),
		},
		{
			name:  "epoch after Fulu should maintain Fulu families",
			epoch: fuluEpoch + 100,
			setupConfig: func() {
				config := params.BeaconConfig().Copy()
				config.GenesisEpoch = genesisEpoch
				config.AltairForkEpoch = altairEpoch
				config.BellatrixForkEpoch = bellatrixEpoch
				config.CapellaForkEpoch = capellaEpoch
				config.DenebForkEpoch = denebEpoch
				config.ElectraForkEpoch = electraEpoch
				config.FuluForkEpoch = fuluEpoch
				config.BlobsidecarSubnetCount = 6
				config.BlobsidecarSubnetCountElectra = 8
				params.OverrideBeaconConfig(config)
			},
			expectedFamilies: combineForks(genesisFamilies, altairFamilies, capellaFamilies, fuluFamilies),
		},
		{
			name:  "edge case - epoch exactly at deactivation should not include deactivated family",
			epoch: electraEpoch, // This deactivates Deneb blobs
			setupConfig: func() {
				config := params.BeaconConfig().Copy()
				config.GenesisEpoch = genesisEpoch
				config.AltairForkEpoch = altairEpoch
				config.BellatrixForkEpoch = bellatrixEpoch
				config.CapellaForkEpoch = capellaEpoch
				config.DenebForkEpoch = denebEpoch
				config.ElectraForkEpoch = electraEpoch
				config.FuluForkEpoch = fuluEpoch
				config.BlobsidecarSubnetCount = 6
				config.BlobsidecarSubnetCountElectra = 8
				params.OverrideBeaconConfig(config)
			},
			expectedFamilies: combineForks(genesisFamilies, altairFamilies, capellaFamilies, electraBlobFamilies),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			if tt.enableLightClient {
				resetFlags := features.InitWithReset(&features.Flags{
					EnableLightClient: true,
				})
				defer resetFlags()
			}
			tt.setupConfig()
			service := createMinimalService(t)
			families := TopicFamiliesForEpoch(tt.epoch, service, params.NetworkScheduleEntry{})

			// Collect actual family names
			actualFamilies := make([]string, 0, len(families))
			for _, family := range families {
				actualFamilies = append(actualFamilies, family.Name())
			}

			// Assert exact match - families should have exactly the expected families and nothing more
			assert.Equal(t, len(tt.expectedFamilies), len(actualFamilies),
				"Expected %d families but got %d", len(tt.expectedFamilies), len(actualFamilies))

			// Create a map for efficient lookup
			expectedMap := make(map[string]bool)
			for _, expected := range tt.expectedFamilies {
				expectedMap[expected] = true
			}

			// Check each actual family is expected
			for _, actual := range actualFamilies {
				if !expectedMap[actual] {
					t.Errorf("Unexpected topic family found: %s", actual)
				}
				delete(expectedMap, actual) // Remove from map as we find it
			}

			// Check all expected families were found (anything left in map was missing)
			for missing := range expectedMap {
				t.Errorf("Expected topic family not found: %s", missing)
			}
		})
	}
}
