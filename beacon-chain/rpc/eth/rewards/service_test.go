package rewards

import (
	"strconv"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockstategen "github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen/mock"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestGetStateForRewards_NextSlotCacheHit(t *testing.T) {
	ctx := t.Context()
	db := dbutil.SetupDB(t)

	st, err := util.NewBeaconStateDeneb()
	require.NoError(t, err)
	b := util.HydrateSignedBeaconBlockDeneb(util.NewBeaconBlockDeneb())
	parent, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, parent))

	r, err := parent.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, transition.UpdateNextSlotCache(ctx, r[:], st))

	s := &BlockRewardService{
		Replayer: nil, // setting to nil because replayer must not be invoked
		DB:       db,
	}
	b = util.HydrateSignedBeaconBlockDeneb(util.NewBeaconBlockDeneb())
	sbb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	sbb.SetSlot(parent.Block().Slot() + 1)
	result, err := s.GetStateForRewards(ctx, sbb.Block())
	require.NoError(t, err)
	_, lcs := transition.LastCachedState()
	expected, err := lcs.HashTreeRoot(ctx)
	require.NoError(t, err)
	actual, err := result.HashTreeRoot(ctx)
	require.NoError(t, err)
	assert.DeepEqual(t, expected, actual)
}

func TestGetBlockRewardsData_GloasParentPayload(t *testing.T) {
	helpers.ClearCache()
	ctx := t.Context()

	// Parent block at slot P (= 1) with a full payload,
	// slot P+1 (= 2) skipped,
	// block at N (= 3) includes an attestation from slot P+1 voting the parent payload present.
	const parentSlot, blockSlot = primitives.Slot(1), primitives.Slot(3)

	// Set up a state with a parent block at slot P and a block at slot N.
	st, _ := util.DeterministicGenesisStateGloas(t, 64)
	require.NoError(t, st.SetSlot(blockSlot))
	header := st.LatestBlockHeader()
	header.Slot = parentSlot
	require.NoError(t, st.SetLatestBlockHeader(header))
	parentRoot, err := header.HashTreeRoot()
	require.NoError(t, err)
	roots := st.BlockRoots()
	for i := range blockSlot {
		roots[i] = parentRoot[:]
	}
	require.NoError(t, st.SetBlockRoots(roots))

	parentBlockHash := [32]byte{0xaa}
	emptyRequestsRoot, err := enginev1.EmptyExecutionRequestsHashTreeRoot()
	require.NoError(t, err)
	parentBid, err := blocks.WrappedROExecutionPayloadBid(util.HydrateExecutionPayloadBid(&ethpb.ExecutionPayloadBid{
		Slot:                  parentSlot,
		BlockHash:             parentBlockHash[:],
		ExecutionRequestsRoot: emptyRequestsRoot[:],
	}))
	require.NoError(t, err)
	require.NoError(t, st.SetExecutionPayloadBid(parentBid))
	require.NoError(t, st.SetExecutionPayloadAvailability(parentSlot, false))

	proposerIdx, err := helpers.BeaconProposerIndex(ctx, st)
	require.NoError(t, err)

	// Keep the proposer out of the sync committee so its balance delta is attestation rewards only.
	other := st.PubkeyAtIndex((proposerIdx + 1) % 64)
	syncPubkeys := make([][]byte, params.BeaconConfig().SyncCommitteeSize)
	for i := range syncPubkeys {
		syncPubkeys[i] = other[:]
	}
	require.NoError(t, st.SetCurrentSyncCommittee(&ethpb.SyncCommittee{Pubkeys: syncPubkeys, AggregatePubkey: other[:]}))

	// Build a block at slot N with an attestation from slot P+1 voting the parent payload present.
	committee, err := helpers.BeaconCommitteeFromState(ctx, st, parentSlot+1, 0)
	require.NoError(t, err)
	aggBits := bitfield.NewBitlist(uint64(len(committee)))
	for i := range committee {
		aggBits.SetBitAt(uint64(i), true)
	}
	cb := primitives.NewAttestationCommitteeBits()
	cb.SetBitAt(0, true)
	randao, err := helpers.RandaoMix(st, 0)
	require.NoError(t, err)
	b := util.NewBeaconBlockGloas()
	b.Block.Slot = blockSlot
	b.Block.ProposerIndex = proposerIdx
	b.Block.ParentRoot = parentRoot[:]
	b.Block.Body.SyncAggregate.SyncCommitteeSignature = common.InfiniteSignature[:]
	b.Block.Body.SignedExecutionPayloadBid.Signature = common.InfiniteSignature[:]
	bid := b.Block.Body.SignedExecutionPayloadBid.Message
	bid.Slot = blockSlot
	bid.BuilderIndex = params.BeaconConfig().BuilderIndexSelfBuild
	bid.ParentBlockHash = parentBlockHash[:]
	bid.ParentBlockRoot = parentRoot[:]
	bid.PrevRandao = randao
	b.Block.Body.Attestations = []*ethpb.AttestationGloas{{
		AggregationBits: aggBits,
		CommitteeBits:   cb,
		Signature:       make([]byte, fieldparams.BLSSignatureLength),
		Data: &ethpb.AttestationData{
			Slot:            parentSlot + 1,
			CommitteeIndex:  1,
			BeaconBlockRoot: parentRoot[:],
			Source:          &ethpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
			Target:          &ethpb.Checkpoint{Root: parentRoot[:]},
		},
	}}
	sbb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)

	// Run the transition to get the post-state and check that the proposer balance increased due to attestation rewards.
	initBalance, err := st.BalanceAtIndex(proposerIdx)
	require.NoError(t, err)
	post, err := transition.ProcessBlockForStateRoot(ctx, st.Copy(), sbb)
	require.NoError(t, err)
	postBalance, err := post.BalanceAtIndex(proposerIdx)
	require.NoError(t, err)

	// Without the parent payload applied the attestation misses the head flag.
	stale, err := altair.ProcessAttestationsNoVerifySignature(ctx, st.Copy(), sbb.Block())
	require.NoError(t, err)
	staleBalance, err := stale.BalanceAtIndex(proposerIdx)
	require.NoError(t, err)
	require.Equal(t, true, staleBalance < postBalance)

	rb := mockstategen.NewReplayerBuilder()
	rb.SetMockStateForSlot(st.Copy(), blockSlot-1)
	s := &BlockRewardService{Replayer: rb, DB: dbutil.SetupDB(t)}
	rewards, httpErr := s.GetBlockRewardsData(ctx, sbb.Block())
	require.Equal(t, (*httputil.DefaultJsonError)(nil), httpErr)

	// Core assertions: the attestation rewards are equal to the proposer's balance delta,
	// and the total rewards are equal to the proposer's balance delta.
	assert.Equal(t, strconv.FormatUint(postBalance-initBalance, 10), rewards.Attestations)
	assert.Equal(t, strconv.FormatUint(postBalance-initBalance, 10), rewards.Total)
}
