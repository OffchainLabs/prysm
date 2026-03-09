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

// TestBadPayloadCache_EquivocationSafe verifies that the bad payload cache,
// now keyed by envelope HTR, does not suffer from builder equivocation
// poisoning. A bad envelope for block N is cached by its own HTR. A different
// (good) envelope for the same block N has a different HTR and is not blocked.
func TestBadPayloadCache_EquivocationSafe(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	s := &Service{badPayloadCache: lruwrpr.New(10)}

	// Two different envelope HTRs for the same block root (builder equivocation).
	badEnvelopeHTR := [32]byte{0xBB}
	goodEnvelopeHTR := [32]byte{0xCC}

	// Bad envelope is marked in the cache.
	s.setBadPayload(ctx, badEnvelopeHTR)

	// Bad envelope is recognized.
	require.True(t, s.hasBadPayload(badEnvelopeHTR))
	// Good envelope is NOT blocked — different HTR, not in cache.
	require.False(t, s.hasBadPayload(goodEnvelopeHTR))
}
