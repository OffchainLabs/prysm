package blockchain

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// gloasBlockWithBid builds a Gloas block committing to blockHash and extending parentBlockHash.
func gloasBlockWithBid(
	t *testing.T,
	slot primitives.Slot,
	parentRoot [32]byte,
	blockHash, parentBlockHash [32]byte,
	builderIndex primitives.BuilderIndex,
) *ethpb.SignedBeaconBlockGloas {
	t.Helper()
	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash[:],
			ParentBlockHash: parentBlockHash[:],
			BuilderIndex:    builderIndex,
		},
	})
	return util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body:       &ethpb.BeaconBlockBodyGloas{SignedExecutionPayloadBid: bid},
		},
	})
}

// builderRegistryState returns a Gloas state holding count active builders. IsActiveBuilder wants a
// finalized deposit, hence the finalized epoch ahead of DepositEpoch.
func builderRegistryState(t *testing.T, count int) state.BeaconState {
	t.Helper()
	base, _ := testGloasState(t, 1, [32]byte{}, [32]byte{})
	base.FinalizedCheckpoint.Epoch = 1
	builders := make([]*ethpb.Builder, count)
	for i := range builders {
		pubkey := make([]byte, fieldparams.BLSPubkeyLength)
		pubkey[0] = byte(i + 1)
		builders[i] = &ethpb.Builder{
			Pubkey:            pubkey,
			Version:           []byte{0},
			ExecutionAddress:  make([]byte, 20),
			DepositEpoch:      0,
			WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
		}
	}
	base.Builders = builders
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)
	return st
}

// setupBuilderFailureTest inserts a Gloas parent block whose bid names builderIndex and gives the
// parent enough attestation weight to clear the threshold.
func setupBuilderFailureTest(
	t *testing.T,
	builderIndex primitives.BuilderIndex,
	balances []uint64,
) (*Service, [32]byte, [32]byte, state.BeaconState) {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	require.NoError(t, params.SetActive(cfg))

	cb := cache.NewBuilderCircuitBreaker()
	service, tr := setupGloasService(t, &mockExecution.EngineClient{})
	require.NoError(t, WithBuilderCircuitBreaker(cb)(service))
	service.cfg.ForkChoiceStore.SetBalancesByRooter(
		func(context.Context, [32]byte) ([]uint64, error) { return balances, nil })

	parentHash := bytesutil.ToBytes32([]byte("parenthash"))
	parentRoot := bytesutil.ToBytes32([]byte("parentroot"))
	base, _ := testGloasState(t, 1, params.BeaconConfig().ZeroHash, parentHash)
	parentBlk := gloasBlockWithBid(t, 1, params.BeaconConfig().ZeroHash, parentHash, [32]byte{}, builderIndex)
	insertGloasBlock(t, service, base, parentBlk, parentRoot)

	// One attestation for the parent. UpdateJustifiedCheckpoint pulls the balances in, then Head
	// folds them into the node weights.
	service.cfg.ForkChoiceStore.ProcessAttestation(tr.ctx, []uint64{0}, parentRoot, 1, false)
	require.NoError(t, service.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(
		tr.ctx, &forkchoicetypes.Checkpoint{Epoch: 0, Root: parentRoot}))
	_, err := service.cfg.ForkChoiceStore.Head(tr.ctx)
	require.NoError(t, err)

	// The self build sentinel is not a registry index, so it gets no entry.
	count := 0
	if builderIndex != params.BeaconConfig().BuilderIndexSelfBuild {
		count = int(builderIndex) + 1
	}
	return service, parentRoot, parentHash, builderRegistryState(t, count)
}

// childOnEmpty returns a block extending the parent's empty payload.
func childOnEmpty(t *testing.T, parentRoot [32]byte, slot primitives.Slot, hash byte) interfaces.ReadOnlyBeaconBlock {
	t.Helper()
	blk := gloasBlockWithBid(t, slot, parentRoot, bytesutil.ToBytes32([]byte{hash}), [32]byte{}, 3)
	signed, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	return signed.Block()
}

func TestCheckBuilderPayloadFailure_BlacklistsBuilder(t *testing.T) {
	service, parentRoot, _, st := setupBuilderFailureTest(t, 5, []uint64{100})

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1), st)
	require.Equal(t, true, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))
}

func TestCheckBuilderPayloadFailure_IdempotentPerParent(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderCriticalFailures = 2
	cfg.BuilderBlacklistPeriod = 1
	cfg.BuilderCriticalBlacklistPeriod = 256
	require.NoError(t, params.SetActive(cfg))

	service, parentRoot, _, st := setupBuilderFailureTest(t, 5, []uint64{100})

	// Two children extending the same empty parent must only count as one failure, so the
	// builder gets the short ban rather than the critical one.
	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1), st)
	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 2), st)
	require.Equal(t, true, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 1))
}

// An exit is terminal, so the record goes with it.
func TestCheckBuilderPayloadFailure_ExitedBuilderIsUnbanned(t *testing.T) {
	service, parentRoot, _, st := setupBuilderFailureTest(t, 5, []uint64{100})

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1), st)
	require.Equal(t, true, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))

	builder, err := st.Builder(5)
	require.NoError(t, err)
	builder.WithdrawableEpoch = 8
	require.NoError(t, st.UpdateBuilderAtIndex(5, builder))

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 2), st)
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))
}

func TestCheckBuilderPayloadFailure_NotBlacklistedWhenPayloadPresent(t *testing.T) {
	service, parentRoot, parentHash, st := setupBuilderFailureTest(t, 5, []uint64{100})

	// The payload did arrive, so a child on empty is a payload reorg attempt, not a failure.
	env, err := blocks.WrappedROExecutionPayloadEnvelope(
		testSignedEnvelope(t, parentRoot, 1, parentHash[:]).Message)
	require.NoError(t, err)
	require.NoError(t, service.InsertPayload(env))

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1), st)
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))
}

func TestCheckBuilderPayloadFailure_NotBlacklistedBelowWeightThreshold(t *testing.T) {
	// 100 validators of 100 each: committee weight is 312, a single attestation is far below 60%.
	balances := make([]uint64, 100)
	for i := range balances {
		balances[i] = 100
	}
	service, parentRoot, _, st := setupBuilderFailureTest(t, 5, balances)

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1), st)
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))
}

func TestCheckBuilderPayloadFailure_NotBlacklistedAcrossSkippedSlot(t *testing.T) {
	service, parentRoot, _, st := setupBuilderFailureTest(t, 5, []uint64{100})

	// Parent is at slot 1, so a child at slot 3 leaves a skipped slot in between.
	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 3, 1), st)
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))
}

func TestCheckBuilderPayloadFailure_NotBlacklistedForSelfBuild(t *testing.T) {
	selfBuild := params.BeaconConfig().BuilderIndexSelfBuild
	service, parentRoot, _, st := setupBuilderFailureTest(t, selfBuild, []uint64{100})

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1), st)
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(selfBuild, 0))
}

// checkBuilderPayloadFailure has no builds-on-full check because forkchoice cannot hold such a
// block while the parent's payload is missing: insert resolves the child to the parent's full node
// and rejects it when that node is absent. If this ever stops holding, the circuit breaker would
// start blacklisting builders whose payload was actually delivered.
func TestInsertRejectsBuildsOnFullChildWithoutPayload(t *testing.T) {
	service, parentRoot, parentHash, _ := setupBuilderFailureTest(t, 5, []uint64{100})
	require.Equal(t, false, service.cfg.ForkChoiceStore.HasFullNode(parentRoot))

	childHash := bytesutil.ToBytes32([]byte("childhash"))
	base, _ := testGloasState(t, 2, parentRoot, childHash)
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)
	// The child commits to the parent's own block hash, so it claims to build on full.
	blk := gloasBlockWithBid(t, 2, parentRoot, childHash, parentHash, 3)
	signed, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlockWithRoot(signed, bytesutil.ToBytes32([]byte("child")))
	require.NoError(t, err)

	require.ErrorContains(t, "invalid parent root", service.InsertNode(t.Context(), st, roblock))
}

// gaugeValue reads the current value of a prometheus gauge.
func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, g.Write(&m))
	return m.GetGauge().GetValue()
}

// The gauges must track the breaker's current state, not the state at the last detected failure,
// otherwise they keep reporting a tripped breaker long after the bans have expired.
func TestCheckBuilderPayloadFailure_GaugesFollowExpiry(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BuilderAllowedFailures = 0
	cfg.BuilderBlacklistPeriod = 1
	cfg.BuilderCriticalFailedBuilders = 1
	require.NoError(t, params.SetActive(cfg))

	service, parentRoot, parentHash, st := setupBuilderFailureTest(t, 5, []uint64{100})

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1), st)
	require.Equal(t, float64(1), gaugeValue(t, builderBlacklistedCount))
	require.Equal(t, float64(1), gaugeValue(t, builderSelfBuildOnly))

	// A later block that is not a failure candidate: the parent's payload is present, so every
	// early return in recordBuilderPayloadFailure fires. The gauges must still be refreshed.
	env, err := blocks.WrappedROExecutionPayloadEnvelope(
		testSignedEnvelope(t, parentRoot, 1, parentHash[:]).Message)
	require.NoError(t, err)
	require.NoError(t, service.InsertPayload(env))

	healthy := childOnEmpty(t, parentRoot, params.BeaconConfig().SlotsPerEpoch+2, 3)
	service.checkBuilderPayloadFailure(healthy, st)
	require.Equal(t, float64(0), gaugeValue(t, builderBlacklistedCount))
	require.Equal(t, float64(0), gaugeValue(t, builderSelfBuildOnly))
}

func TestCheckBuilderPayloadFailure_NoBreakerConfigured(t *testing.T) {
	service, _ := setupGloasService(t, &mockExecution.EngineClient{})
	require.Equal(t, true, service.cfg.BuilderCircuitBreaker == nil)
	// Must not panic.
	service.checkBuilderPayloadFailure(childOnEmpty(t, [32]byte{1}, 2, 1), builderRegistryState(t, 1))
}
