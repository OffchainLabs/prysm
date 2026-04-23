package blocks_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// setGloasTestConfig sets fork epochs so Gloas is active at epoch 5.
func setGloasTestConfig(t *testing.T) {
	t.Helper()
	cfg := params.BeaconConfig().Copy()
	cfg.CapellaForkEpoch = 1
	cfg.DenebForkEpoch = 2
	cfg.ElectraForkEpoch = 3
	cfg.FuluForkEpoch = 4
	cfg.GloasForkEpoch = 5
	params.SetActiveTestCleanup(t, cfg)
}

// newGloasStateWithBuilder creates a minimal Gloas beacon state with one active builder
// and returns the state along with the builder's BLS private key.
func newGloasStateWithBuilder(t *testing.T, builderIndex primitives.BuilderIndex, epoch primitives.Epoch) (state.BeaconState, bls.SecretKey) {
	t.Helper()

	priv, err := bls.RandKey()
	require.NoError(t, err)

	cfg := params.BeaconConfig()

	builder := &ethpb.Builder{
		Pubkey:            priv.PublicKey().Marshal(),
		WithdrawableEpoch: cfg.FarFutureEpoch,
		DepositEpoch:      0,
		Balance:           32_000_000_000,
		ExecutionAddress:  make([]byte, 20),
	}

	builders := make([]*ethpb.Builder, int(builderIndex)+1)
	for i := range builders {
		if primitives.BuilderIndex(i) == builderIndex {
			builders[i] = builder
		} else {
			builders[i] = &ethpb.Builder{
				Pubkey:            make([]byte, 48),
				WithdrawableEpoch: cfg.FarFutureEpoch,
				DepositEpoch:      0,
				ExecutionAddress:  make([]byte, 20),
			}
		}
	}

	stProto := &ethpb.BeaconStateGloas{
		Slot: cfg.SlotsPerEpoch * primitives.Slot(epoch),
		Fork: &ethpb.Fork{
			PreviousVersion: cfg.FuluForkVersion,
			CurrentVersion:  cfg.GloasForkVersion,
			Epoch:           cfg.GloasForkEpoch,
		},
		GenesisValidatorsRoot: make([]byte, 32),
		FinalizedCheckpoint: &ethpb.Checkpoint{
			Epoch: epoch - 1,
			Root:  make([]byte, 32),
		},
		CurrentJustifiedCheckpoint:  &ethpb.Checkpoint{Root: make([]byte, 32)},
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, 32)},
		Builders:                    builders,
		Validators: []*ethpb.Validator{
			{
				ExitEpoch:       cfg.FarFutureEpoch,
				ActivationEpoch: 0,
				PublicKey:       make([]byte, 48),
			},
		},
		Balances:                     []uint64{32_000_000_000},
		BlockRoots:                   make([][]byte, cfg.SlotsPerHistoricalRoot),
		StateRoots:                   make([][]byte, cfg.SlotsPerHistoricalRoot),
		RandaoMixes:                  make([][]byte, cfg.EpochsPerHistoricalVector),
		Slashings:                    make([]uint64, cfg.EpochsPerSlashingsVector),
		ExecutionPayloadAvailability: make([]byte, cfg.SlotsPerHistoricalRoot/8),
	}

	for i := range stProto.BlockRoots {
		stProto.BlockRoots[i] = make([]byte, 32)
	}
	for i := range stProto.StateRoots {
		stProto.StateRoots[i] = make([]byte, 32)
	}
	for i := range stProto.RandaoMixes {
		stProto.RandaoMixes[i] = make([]byte, 32)
	}

	st, err := state_native.InitializeFromProtoUnsafeGloas(stProto)
	require.NoError(t, err)
	return st, priv
}

func signBuilderExit(t *testing.T, st state.ReadOnlyBeaconState, exit *ethpb.VoluntaryExit, priv bls.SecretKey) *ethpb.SignedVoluntaryExit {
	t.Helper()

	sb, err := signing.ComputeDomainAndSign(st, exit.Epoch, exit, params.BeaconConfig().DomainVoluntaryExit, priv)
	require.NoError(t, err)
	sig, err := bls.SignatureFromBytes(sb)
	require.NoError(t, err)

	return &ethpb.SignedVoluntaryExit{
		Exit:      exit,
		Signature: sig.Marshal(),
	}
}

func TestVerifyExitAndSignature_BuilderExit_HappyPath(t *testing.T) {
	setGloasTestConfig(t)

	builderIndex := primitives.BuilderIndex(0)
	epoch := primitives.Epoch(10)
	st, priv := newGloasStateWithBuilder(t, builderIndex, epoch)

	exit := &ethpb.VoluntaryExit{
		ValidatorIndex: builderIndex.ToValidatorIndex(),
		Epoch:          epoch,
	}
	signed := signBuilderExit(t, st, exit, priv)

	err := blocks.VerifyExitAndSignature(nil, st, signed)
	require.NoError(t, err)
}

func TestVerifyExitAndSignature_BuilderNotActive(t *testing.T) {
	setGloasTestConfig(t)

	builderIndex := primitives.BuilderIndex(0)
	epoch := primitives.Epoch(10)
	st, priv := newGloasStateWithBuilder(t, builderIndex, epoch)

	// Make builder not active by setting withdrawable epoch (already initiated exit).
	builder, err := st.Builder(builderIndex)
	require.NoError(t, err)
	builder.WithdrawableEpoch = 5
	require.NoError(t, st.UpdateBuilderAtIndex(builderIndex, builder))

	exit := &ethpb.VoluntaryExit{
		ValidatorIndex: builderIndex.ToValidatorIndex(),
		Epoch:          epoch,
	}
	signed := signBuilderExit(t, st, exit, priv)

	err = blocks.VerifyExitAndSignature(nil, st, signed)
	assert.ErrorContains(t, "is not active", err)
}

func TestVerifyExitAndSignature_BuilderPendingWithdrawal(t *testing.T) {
	setGloasTestConfig(t)

	builderIndex := primitives.BuilderIndex(0)
	epoch := primitives.Epoch(10)
	st, priv := newGloasStateWithBuilder(t, builderIndex, epoch)

	// Give the builder a pending withdrawal.
	require.NoError(t, st.AppendBuilderPendingWithdrawals([]*ethpb.BuilderPendingWithdrawal{
		{
			BuilderIndex: builderIndex,
			Amount:       1000,
			FeeRecipient: make([]byte, 20),
		},
	}))

	exit := &ethpb.VoluntaryExit{
		ValidatorIndex: builderIndex.ToValidatorIndex(),
		Epoch:          epoch,
	}
	signed := signBuilderExit(t, st, exit, priv)

	err := blocks.VerifyExitAndSignature(nil, st, signed)
	assert.ErrorContains(t, "pending balance to withdraw", err)
}

func TestVerifyExitAndSignature_BuilderBadSignature(t *testing.T) {
	setGloasTestConfig(t)

	builderIndex := primitives.BuilderIndex(0)
	epoch := primitives.Epoch(10)
	st, _ := newGloasStateWithBuilder(t, builderIndex, epoch)

	wrongKey, err := bls.RandKey()
	require.NoError(t, err)

	exit := &ethpb.VoluntaryExit{
		ValidatorIndex: builderIndex.ToValidatorIndex(),
		Epoch:          epoch,
	}
	signed := signBuilderExit(t, st, exit, wrongKey)

	err = blocks.VerifyExitAndSignature(nil, st, signed)
	assert.ErrorContains(t, "signature did not verify", err)
}

func TestVerifyExitAndSignature_BuilderExitInFuture(t *testing.T) {
	setGloasTestConfig(t)

	builderIndex := primitives.BuilderIndex(0)
	epoch := primitives.Epoch(10)
	st, priv := newGloasStateWithBuilder(t, builderIndex, epoch)

	exit := &ethpb.VoluntaryExit{
		ValidatorIndex: builderIndex.ToValidatorIndex(),
		Epoch:          epoch + 1, // Future epoch.
	}
	signed := signBuilderExit(t, st, exit, priv)

	err := blocks.VerifyExitAndSignature(nil, st, signed)
	assert.ErrorContains(t, "expected current epoch >= exit epoch", err)
}

func TestProcessVoluntaryExits_BuilderExit(t *testing.T) {
	setGloasTestConfig(t)

	builderIndex := primitives.BuilderIndex(0)
	epoch := primitives.Epoch(10)
	st, priv := newGloasStateWithBuilder(t, builderIndex, epoch)

	exit := &ethpb.VoluntaryExit{
		ValidatorIndex: builderIndex.ToValidatorIndex(),
		Epoch:          epoch,
	}
	signed := signBuilderExit(t, st, exit, priv)

	newState, err := blocks.ProcessVoluntaryExits(t.Context(), st, []*ethpb.SignedVoluntaryExit{signed}, validators.ExitInformation(st))
	require.NoError(t, err)

	// Verify builder's withdrawable epoch was set.
	builder, err := newState.Builder(builderIndex)
	require.NoError(t, err)
	cfg := params.BeaconConfig()
	expectedWithdrawableEpoch := epoch + cfg.MinBuilderWithdrawabilityDelay
	assert.Equal(t, expectedWithdrawableEpoch, builder.WithdrawableEpoch)
}

func TestProcessVoluntaryExits_BuilderExitPreGloas(t *testing.T) {
	cfg := params.BeaconConfig().Copy()
	cfg.CapellaForkEpoch = 1
	cfg.DenebForkEpoch = 2
	cfg.ElectraForkEpoch = 3
	cfg.FuluForkEpoch = 4
	cfg.GloasForkEpoch = 100 // Gloas not yet active.
	params.SetActiveTestCleanup(t, cfg)

	epoch := primitives.Epoch(10)
	builderIndex := primitives.BuilderIndex(0)

	stProto := &ethpb.BeaconStateFulu{
		Slot: cfg.SlotsPerEpoch * primitives.Slot(epoch),
		Fork: &ethpb.Fork{
			PreviousVersion: cfg.DenebForkVersion,
			CurrentVersion:  cfg.FuluForkVersion,
			Epoch:           cfg.FuluForkEpoch,
		},
		GenesisValidatorsRoot:       make([]byte, 32),
		FinalizedCheckpoint:         &ethpb.Checkpoint{Root: make([]byte, 32)},
		CurrentJustifiedCheckpoint:  &ethpb.Checkpoint{Root: make([]byte, 32)},
		PreviousJustifiedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, 32)},
		Validators: []*ethpb.Validator{
			{ExitEpoch: cfg.FarFutureEpoch, ActivationEpoch: 0, PublicKey: make([]byte, 48)},
		},
		Balances:    []uint64{32_000_000_000},
		BlockRoots:  make([][]byte, cfg.SlotsPerHistoricalRoot),
		StateRoots:  make([][]byte, cfg.SlotsPerHistoricalRoot),
		RandaoMixes: make([][]byte, cfg.EpochsPerHistoricalVector),
		Slashings:   make([]uint64, cfg.EpochsPerSlashingsVector),
	}
	for i := range stProto.BlockRoots {
		stProto.BlockRoots[i] = make([]byte, 32)
	}
	for i := range stProto.StateRoots {
		stProto.StateRoots[i] = make([]byte, 32)
	}
	for i := range stProto.RandaoMixes {
		stProto.RandaoMixes[i] = make([]byte, 32)
	}

	st, err := state_native.InitializeFromProtoUnsafeFulu(stProto)
	require.NoError(t, err)

	signed := &ethpb.SignedVoluntaryExit{
		Exit: &ethpb.VoluntaryExit{
			ValidatorIndex: builderIndex.ToValidatorIndex(),
			Epoch:          epoch,
		},
		Signature: make([]byte, 96),
	}

	// On pre-Gloas state, builder-flagged exits are not routed to the builder path.
	// ProcessVoluntaryExits treats the builder-flagged index as a regular validator index,
	// which fails because no such validator exists.
	_, err = blocks.ProcessVoluntaryExits(t.Context(), st, []*ethpb.SignedVoluntaryExit{signed}, validators.ExitInformation(st))
	require.ErrorContains(t, "out of bounds", err)
}
