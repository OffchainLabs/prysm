package sync

import (
	"context"
	"testing"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/stretchr/testify/require"
)

func TestValidateExecutionPayloadBid_Accept(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	parentRoot := bytesutil.PadTo([]byte{0x01}, fieldparams.RootLength)
	block := util.NewBeaconBlockGloas()
	block.Block.ParentRoot = parentRoot
	block.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockRoot = parentRoot
	block.Block.Body.SignedExecutionPayloadBid.Message.BlobKzgCommitments = nil

	wsb, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	s := &Service{}
	res, err := s.validateExecutionPayloadBid(ctx, wsb.Block())
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)
}

func TestValidateExecutionPayloadBid_RejectParentRootMismatch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	block := util.NewBeaconBlockGloas()
	block.Block.ParentRoot = bytesutil.PadTo([]byte{0x01}, fieldparams.RootLength)
	block.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockRoot = bytesutil.PadTo([]byte{0x02}, fieldparams.RootLength)

	wsb, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	s := &Service{}
	res, err := s.validateExecutionPayloadBid(ctx, wsb.Block())
	require.Error(t, err)
	require.Equal(t, pubsub.ValidationReject, res)
}

func TestValidateExecutionPayloadBid_RejectTooManyCommitments(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	parentRoot := bytesutil.PadTo([]byte{0x01}, fieldparams.RootLength)
	block := util.NewBeaconBlockGloas()
	block.Block.ParentRoot = parentRoot
	block.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockRoot = parentRoot

	maxBlobs := params.BeaconConfig().MaxBlobsPerBlockAtEpoch(0)
	commitments := make([][]byte, maxBlobs+1)
	for i := range commitments {
		commitments[i] = bytesutil.PadTo([]byte{0x02}, fieldparams.BLSPubkeyLength)
	}
	block.Block.Body.SignedExecutionPayloadBid.Message.BlobKzgCommitments = commitments

	wsb, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	s := &Service{}
	res, err := s.validateExecutionPayloadBid(ctx, wsb.Block())
	require.Error(t, err)
	require.Equal(t, pubsub.ValidationReject, res)
}

func TestValidateExecutionPayloadBidParentSeen_PreGloas(t *testing.T) {
	ctx := context.Background()
	blk := util.HydrateSignedBeaconBlockDeneb(nil)
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	s := &Service{}
	res, err := s.validateExecutionPayloadBidParentSeen(ctx, wsb.Block())
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)
}

func TestValidateExecutionPayloadBidParentSeen_Accept(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	ready := true
	s := &Service{cfg: &config{chain: &mock.ChainService{ParentPayloadReadyVal: &ready}}}

	blk := util.NewBeaconBlockGloas()
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	res, err := s.validateExecutionPayloadBidParentSeen(ctx, wsb.Block())
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)
}

func TestValidateExecutionPayloadBidParentSeen_Ignore(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	notReady := false
	s := &Service{cfg: &config{chain: &mock.ChainService{ParentPayloadReadyVal: &notReady}}}

	blk := util.NewBeaconBlockGloas()
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	res, err := s.validateExecutionPayloadBidParentSeen(ctx, wsb.Block())
	require.Error(t, err)
	require.Equal(t, pubsub.ValidationIgnore, res)
}

func TestValidateExecutionPayloadBidParentValid_PreGloas(t *testing.T) {
	ctx := context.Background()
	blk := util.HydrateSignedBeaconBlockDeneb(nil)
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	s := &Service{}
	res, err := s.validateExecutionPayloadBidParentValid(ctx, wsb.Block())
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)
}

func TestValidateExecutionPayloadBidParentValid_Accept(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	s := &Service{
		badPayloadRootCache:  lruwrpr.New(10),
		goodPayloadRootCache: lruwrpr.New(10),
	}

	blk := util.NewBeaconBlockGloas()
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	res, err := s.validateExecutionPayloadBidParentValid(ctx, wsb.Block())
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)
}

func TestValidateExecutionPayloadBidParentValid_Reject(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	s := &Service{
		badPayloadRootCache:  lruwrpr.New(10),
		goodPayloadRootCache: lruwrpr.New(10),
	}

	blk := util.NewBeaconBlockGloas()
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	parentRoot := wsb.Block().ParentRoot()
	s.setBadPayloadRoot(ctx, parentRoot)

	res, err := s.validateExecutionPayloadBidParentValid(ctx, wsb.Block())
	require.Error(t, err)
	require.Equal(t, pubsub.ValidationReject, res)
}

// TestBadPayloadCache_EquivocationSafe verifies that the bad payload cache,
// keyed by envelope HTR, does not suffer from builder equivocation poisoning.
func TestBadPayloadCache_EquivocationSafe(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	s := &Service{badPayloadCache: lruwrpr.New(10)}

	badEnvelopeHTR := [32]byte{0xBB}
	goodEnvelopeHTR := [32]byte{0xCC}

	s.setBadPayload(ctx, badEnvelopeHTR)

	require.True(t, s.hasBadPayload(badEnvelopeHTR))
	require.False(t, s.hasBadPayload(goodEnvelopeHTR))
}

// TestBadPayloadRoot_GoodOverridesBad verifies that a good payload root
// overrides a bad one, handling builder equivocation correctly.
func TestBadPayloadRoot_GoodOverridesBad(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	s := &Service{
		badPayloadRootCache:  lruwrpr.New(10),
		goodPayloadRootCache: lruwrpr.New(10),
	}

	blockRoot := [32]byte{0xAA}

	// Step 1: Bad envelope → mark root as bad.
	s.setBadPayloadRoot(ctx, blockRoot)
	require.True(t, s.hasBadPayloadRoot(blockRoot))

	// Step 2: Good envelope → mark root as good (heals).
	s.setGoodPayloadRoot(ctx, blockRoot)
	require.False(t, s.hasBadPayloadRoot(blockRoot))
}

// TestBadPayloadRoot_RejectThenAcceptAfterHeal verifies the full equivocation
// flow: bad payload causes REJECT, good payload heals, subsequent block accepted.
func TestBadPayloadRoot_RejectThenAcceptAfterHeal(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	s := &Service{
		badPayloadRootCache:  lruwrpr.New(10),
		goodPayloadRootCache: lruwrpr.New(10),
	}

	blockNRoot := bytesutil.PadTo([]byte{0xAA}, fieldparams.RootLength)

	// Step 1: Bad envelope for block N.
	s.setBadPayloadRoot(ctx, [32]byte(blockNRoot))

	// Step 2: Block N+1 arrives — parent is bad → REJECT.
	childBlock := util.NewBeaconBlockGloas()
	childBlock.Block.ParentRoot = blockNRoot
	childBlock.Block.Body.SignedExecutionPayloadBid.Message.ParentBlockRoot = blockNRoot
	wsb, err := blocks.NewSignedBeaconBlock(childBlock)
	require.NoError(t, err)

	res, err := s.validateExecutionPayloadBidParentValid(ctx, wsb.Block())
	require.Error(t, err)
	require.Equal(t, pubsub.ValidationReject, res)

	// Step 3: Good envelope for block N arrives → heals.
	s.setGoodPayloadRoot(ctx, [32]byte(blockNRoot))

	// Step 4: Block N+2 with same parent → now accepted.
	res, err = s.validateExecutionPayloadBidParentValid(ctx, wsb.Block())
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)
}
