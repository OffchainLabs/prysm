package blockchain

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
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

// setupBuilderFailureTest inserts a Gloas parent block whose bid names builderIndex, gives the
// parent enough attestation weight to clear the threshold, and returns the service plus the
// parent root.
func setupBuilderFailureTest(
	t *testing.T,
	builderIndex primitives.BuilderIndex,
	balances []uint64,
) (*Service, [32]byte, [32]byte) {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	require.NoError(t, params.SetActive(cfg))

	cb := cache.NewBuilderCircuitBreaker(nil)
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

	return service, parentRoot, parentHash
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
	service, parentRoot, _ := setupBuilderFailureTest(t, 5, []uint64{100})

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1))
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

	service, parentRoot, _ := setupBuilderFailureTest(t, 5, []uint64{100})

	// Two children extending the same empty parent must only count as one failure, so the
	// builder gets the short ban rather than the critical one.
	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1))
	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 2))
	require.Equal(t, true, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 1))
}

func TestCheckBuilderPayloadFailure_NotBlacklistedWhenPayloadPresent(t *testing.T) {
	service, parentRoot, parentHash := setupBuilderFailureTest(t, 5, []uint64{100})

	// The payload did arrive, so a child on empty is a payload reorg attempt, not a failure.
	env, err := blocks.WrappedROExecutionPayloadEnvelope(
		testSignedEnvelope(t, parentRoot, 1, parentHash[:]).Message)
	require.NoError(t, err)
	require.NoError(t, service.InsertPayload(env))

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1))
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))
}

func TestCheckBuilderPayloadFailure_NotBlacklistedBelowWeightThreshold(t *testing.T) {
	// 100 validators of 100 each: committee weight is 312, a single attestation is far below 60%.
	balances := make([]uint64, 100)
	for i := range balances {
		balances[i] = 100
	}
	service, parentRoot, _ := setupBuilderFailureTest(t, 5, balances)

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1))
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))
}

func TestCheckBuilderPayloadFailure_NotBlacklistedAcrossSkippedSlot(t *testing.T) {
	service, parentRoot, _ := setupBuilderFailureTest(t, 5, []uint64{100})

	// Parent is at slot 1, so a child at slot 3 leaves a skipped slot in between.
	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 3, 1))
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(5, 0))
}

func TestCheckBuilderPayloadFailure_NotBlacklistedForSelfBuild(t *testing.T) {
	selfBuild := params.BeaconConfig().BuilderIndexSelfBuild
	service, parentRoot, _ := setupBuilderFailureTest(t, selfBuild, []uint64{100})

	service.checkBuilderPayloadFailure(childOnEmpty(t, parentRoot, 2, 1))
	require.Equal(t, false, service.cfg.BuilderCircuitBreaker.Blacklisted(selfBuild, 0))
}

// checkBuilderPayloadFailure has no builds-on-full check because forkchoice cannot hold such a
// block while the parent's payload is missing: insert resolves the child to the parent's full node
// and rejects it when that node is absent. If this ever stops holding, the circuit breaker would
// start blacklisting builders whose payload was actually delivered.
func TestInsertRejectsBuildsOnFullChildWithoutPayload(t *testing.T) {
	service, parentRoot, parentHash := setupBuilderFailureTest(t, 5, []uint64{100})
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

func TestCheckBuilderPayloadFailure_NoBreakerConfigured(t *testing.T) {
	service, _ := setupGloasService(t, &mockExecution.EngineClient{})
	require.Equal(t, true, service.cfg.BuilderCircuitBreaker == nil)
	// Must not panic.
	service.checkBuilderPayloadFailure(childOnEmpty(t, [32]byte{1}, 2, 1))
}
