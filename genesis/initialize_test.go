package genesis_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/genesis"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestInitialize(t *testing.T) {
	require.NoError(t, genesis.Initialize(t.Context(), "testdata"))
	require.Equal(t, params.MainnetName, params.BeaconConfig().ConfigName)

}

func TestEmbeddedMainnetHardcodedValues(t *testing.T) {
	// Initialize genesis with mainnet config to load embedded state
	require.NoError(t, genesis.Initialize(t.Context(), t.TempDir()))

	// Get the initialized genesis state
	state, err := genesis.State()
	require.NoError(t, err)
	require.NotNil(t, state)

	// Verify hardcoded validators root matches the computed value from the state
	expectedValidatorsRoot := [32]byte{75, 54, 61, 185, 78, 40, 97, 32, 215, 110, 185, 5, 52, 15, 221, 78, 84, 191, 233, 240, 107, 243, 63, 246, 207, 90, 210, 127, 81, 27, 254, 149}
	actualValidatorsRoot := state.GenesisValidatorsRoot()
	require.Equal(t, expectedValidatorsRoot, [32]byte(actualValidatorsRoot), "hardcoded validators root does not match embedded state")

	// Verify hardcoded genesis time matches the computed value from the state
	expectedTime := time.Unix(1606824023, 0)
	actualTime := state.GenesisTime()
	require.Equal(t, expectedTime, actualTime, "hardcoded genesis time does not match embedded state")
}

// mockProvider is a test provider for genesis state
type mockProvider struct {
	name  string
	err   error
	state state.BeaconState
}

func (m *mockProvider) Genesis(context.Context) (state.BeaconState, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.state, nil
}

// createTestGenesisState creates a deterministic genesis state for testing.
// This avoids using the embedded mainnet state which could cause false positives.
func createTestGenesisState(t *testing.T, numValidators uint64, slot primitives.Slot) state.BeaconState {
	// Create a deterministic genesis state using test utilities
	deposits, _, err := util.DeterministicDepositsAndKeys(numValidators)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)

	// Create a minimal beacon state directly
	pb := &ethpb.BeaconState{
		Slot:                  slot,
		GenesisTime:           uint64(time.Unix(2000000000, 0).Unix()), // Use a different time than mainnet
		GenesisValidatorsRoot: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		Eth1Data:              eth1Data,
		Validators:            make([]*ethpb.Validator, numValidators),
		Balances:              make([]uint64, numValidators),
		Fork: &ethpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			Epoch:           0,
		},
		LatestBlockHeader: &ethpb.BeaconBlockHeader{
			Slot:       0,
			ParentRoot: make([]byte, 32),
			StateRoot:  make([]byte, 32),
			BodyRoot:   make([]byte, 32),
		},
		BlockRoots:                  make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot),
		StateRoots:                  make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot),
		RandaoMixes:                 make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		Slashings:                   make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector),
		JustificationBits:           []byte{0},
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, 32)},
		CurrentJustifiedCheckpoint:  &ethpb.Checkpoint{Root: make([]byte, 32)},
		FinalizedCheckpoint:         &ethpb.Checkpoint{Root: make([]byte, 32)},
	}

	// Initialize validators and balances
	for i := uint64(0); i < numValidators; i++ {
		pb.Validators[i] = &ethpb.Validator{
			PublicKey:                  deposits[i].Data.PublicKey,
			WithdrawalCredentials:      deposits[i].Data.WithdrawalCredentials,
			EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			Slashed:                    false,
			ActivationEligibilityEpoch: 0,
			ActivationEpoch:            0,
			ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
		}
		pb.Balances[i] = params.BeaconConfig().MaxEffectiveBalance
	}

	// Initialize arrays with proper sizes
	for i := 0; i < len(pb.BlockRoots); i++ {
		pb.BlockRoots[i] = make([]byte, 32)
	}
	for i := 0; i < len(pb.StateRoots); i++ {
		pb.StateRoots[i] = make([]byte, 32)
	}
	for i := 0; i < len(pb.RandaoMixes); i++ {
		pb.RandaoMixes[i] = make([]byte, 32)
	}

	st, err := state_native.InitializeFromProtoUnsafePhase0(pb)
	require.NoError(t, err)
	return st
}

func TestInitializeWithProviders(t *testing.T) {
	originalConfig := params.BeaconConfig().Copy()
	defer params.OverrideBeaconConfig(originalConfig)

	t.Run("providers_used_when_no_embedded_or_file", func(t *testing.T) {
		// Use a custom config that won't have embedded data
		customConfig := params.MainnetConfig().Copy()
		customConfig.ConfigName = "test-config-no-embedded"
		params.OverrideBeaconConfig(customConfig)

		// Use a deterministic test state instead of mainnet to avoid false positives
		testState := createTestGenesisState(t, 64, 0)

		provider := &mockProvider{
			state: testState,
			name:  "test-provider",
		}

		err := genesis.Initialize(t.Context(), t.TempDir(), provider)
		require.NoError(t, err)

		// Verify the state was stored
		storedState, err := genesis.State()
		require.NoError(t, err)
		require.NotNil(t, storedState)
		require.DeepEqual(t, testState.GenesisValidatorsRoot(), storedState.GenesisValidatorsRoot())
		// Verify it's not the mainnet state
		require.NotEqual(t, uint64(1606824023), storedState.GenesisTime().Unix())
	})

	t.Run("multiple_providers_first_success_wins", func(t *testing.T) {
		// Use a custom config that won't have embedded data
		customConfig := params.MainnetConfig().Copy()
		customConfig.ConfigName = "test-config-multiple-providers"
		params.OverrideBeaconConfig(customConfig)

		// Use deterministic test states with different slots
		state1 := createTestGenesisState(t, 64, 50)
		state2 := createTestGenesisState(t, 32, 100)

		// Create providers - first fails, second succeeds
		failingProvider := &mockProvider{
			err:  errors.New("provider failed"),
			name: "failing-provider",
		}
		successProvider1 := &mockProvider{
			state: state1,
			name:  "success-provider-1",
		}
		successProvider2 := &mockProvider{
			state: state2,
			name:  "success-provider-2",
		}

		// Initialize with multiple providers
		err := genesis.Initialize(t.Context(), t.TempDir(), failingProvider, successProvider1, successProvider2)
		require.NoError(t, err)

		// Verify first successful provider's state was used
		storedState, err := genesis.State()
		require.NoError(t, err)
		require.NotNil(t, storedState)
		// state1 has slot 50, state2 has slot 100
		require.Equal(t, primitives.Slot(50), storedState.Slot())
		// Verify it's state1 by checking validator count
		require.Equal(t, 64, storedState.NumValidators())
	})

	t.Run("all_providers_fail_returns_error", func(t *testing.T) {
		// Use a custom config that won't have embedded data
		customConfig := params.MinimalSpecConfig().Copy()
		customConfig.ConfigName = "test-config-all-fail"
		params.OverrideBeaconConfig(customConfig)

		// Create failing providers
		provider1 := &mockProvider{
			err:  errors.New("provider 1 failed"),
			name: "failing-provider-1",
		}
		provider2 := &mockProvider{
			err:  errors.New("provider 2 failed"),
			name: "failing-provider-2",
		}

		// Initialize should fail when all providers fail
		err := genesis.Initialize(t.Context(), t.TempDir(), provider1, provider2)
		require.ErrorIs(t, err, genesis.ErrGenesisStateNotInitialized)
	})

	t.Run("no_providers_and_no_data_returns_error", func(t *testing.T) {
		// Use a custom config that won't have embedded data
		customConfig := params.MinimalSpecConfig().Copy()
		customConfig.ConfigName = "test-config-no-providers"
		params.OverrideBeaconConfig(customConfig)

		// Initialize with no providers should fail
		err := genesis.Initialize(t.Context(), t.TempDir())
		require.ErrorIs(t, err, genesis.ErrGenesisStateNotInitialized)
	})

	t.Run("provider_returns_nil_state", func(t *testing.T) {
		// Use a custom config that won't have embedded data
		customConfig := params.MinimalSpecConfig().Copy()
		customConfig.ConfigName = "test-config-nil-state"
		params.OverrideBeaconConfig(customConfig)

		// Create provider that returns nil state
		provider := &mockProvider{
			state: nil,
			name:  "nil-state-provider",
		}

		// Initialize should fail
		err := genesis.Initialize(t.Context(), t.TempDir(), provider)
		require.ErrorIs(t, err, genesis.ErrGenesisStateNotInitialized)
	})

	t.Run("empty_dir_path_with_providers", func(t *testing.T) {
		// Use a custom config that won't have embedded data
		customConfig := params.MainnetConfig().Copy()
		customConfig.ConfigName = "test-config-empty-dir"
		params.OverrideBeaconConfig(customConfig)

		// Use a deterministic test state
		testState := createTestGenesisState(t, 16, 0)

		// Create successful provider
		provider := &mockProvider{
			state: testState,
			name:  "test-provider",
		}

		// Initialize with empty dir should fail
		err := genesis.Initialize(t.Context(), "", provider)
		require.ErrorIs(t, err, genesis.ErrFilePathUnset)
	})

	t.Run("genesis_file_takes_precedence_over_providers", func(t *testing.T) {
		// Create temp directory with genesis file
		tmpDir := t.TempDir()

		// Use a custom config that won't have embedded data
		customConfig := params.MainnetConfig().Copy()
		customConfig.ConfigName = "test-config-file-precedence"
		params.OverrideBeaconConfig(customConfig)

		// Create deterministic test states for file and provider
		fileState := createTestGenesisState(t, 128, 75)
		fileTime := time.Unix(1234567890, 0)
		require.NoError(t, fileState.SetGenesisTime(fileTime))

		// Save state to file with proper naming convention
		marshaled, err := fileState.MarshalSSZ()
		require.NoError(t, err)

		gvr := fileState.GenesisValidatorsRoot()
		gvrHex := hexutil.Encode(gvr)
		filename := filepath.Join(tmpDir, "genesis-1234567890-"+gvrHex+".ssz")
		err = os.WriteFile(filename, marshaled, 0644)
		require.NoError(t, err)

		// Create a provider with different state
		providerState := createTestGenesisState(t, 256, 200)
		provider := &mockProvider{
			state: providerState,
			name:  "test-provider",
		}

		// Initialize should use file, not provider
		err = genesis.Initialize(t.Context(), tmpDir, provider)
		require.NoError(t, err)

		// Verify file state was used, not provider state
		storedState, err := genesis.State()
		require.NoError(t, err)
		require.NotNil(t, storedState)
		// fileState has slot 75 and 128 validators, providerState has slot 200 and 256 validators
		require.Equal(t, primitives.Slot(75), storedState.Slot())
		require.Equal(t, 128, storedState.NumValidators())
		require.Equal(t, fileTime, storedState.GenesisTime())
	})
}
