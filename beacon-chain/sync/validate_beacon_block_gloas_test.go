package sync

import (
	"context"
	"testing"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
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

func TestCanonicalBlockFromPubsub_GloasUsesParentExecutionRequests(t *testing.T) {
	ctx := context.Background()
	db := dbtest.SetupDB(t)
	parentRoot := [32]byte{0x01}
	block := util.NewBeaconBlockGloas()
	block.Block.ParentRoot = parentRoot[:]
	block.Block.Body.ParentExecutionRequests.Deposits = append(block.Block.Body.ParentExecutionRequests.Deposits, &enginev1.DepositRequest{
		Pubkey:                bytesutil.PadTo([]byte{0x02}, fieldparams.BLSPubkeyLength),
		WithdrawalCredentials: bytesutil.PadTo([]byte{0x03}, fieldparams.RootLength),
		Amount:                4,
		Signature:             bytesutil.PadTo([]byte{0x05}, fieldparams.BLSSignatureLength),
		Index:                 6,
	})
	wrapped, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	gossip, err := blocks.SignedGossipBeaconBlockGloasFromBlock(wrapped)
	require.NoError(t, err)
	envelope := testSignedExecutionPayloadEnvelope(t, 1, 1, parentRoot, [32]byte{})
	envelope.Message.ExecutionRequests = block.Block.Body.ParentExecutionRequests
	require.NoError(t, db.SaveExecutionPayloadEnvelope(ctx, envelope))

	ready := true
	s := &Service{cfg: &config{beaconDB: db, chain: &mock.ChainService{ParentPayloadReadyVal: &ready}}}
	got, ok, err := s.canonicalBlockFromPubsub(ctx, gossip)
	require.NoError(t, err)
	require.True(t, ok)
	reqs, err := got.Block().Body().ParentExecutionRequests()
	require.NoError(t, err)
	require.Len(t, reqs.Deposits, 1)
	require.Equal(t, uint64(6), reqs.Deposits[0].Index)
}

func TestCanonicalBlockFromPubsub_GloasWaitsForParentPayload(t *testing.T) {
	block := util.NewBeaconBlockGloas()
	wrapped, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	gossip, err := blocks.SignedGossipBeaconBlockGloasFromBlock(wrapped)
	require.NoError(t, err)

	ready := false
	s := &Service{cfg: &config{chain: &mock.ChainService{ParentPayloadReadyVal: &ready}}}
	_, ok, err := s.canonicalBlockFromPubsub(context.Background(), gossip)
	require.NoError(t, err)
	require.False(t, ok)
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

	s := &Service{badPayloadCache: lruwrpr.New(10)}

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

	s := &Service{badPayloadCache: lruwrpr.New(10)}

	blk := util.NewBeaconBlockGloas()
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	parentRoot := wsb.Block().ParentRoot()
	s.badPayloadCache.Add(string(parentRoot[:]), true)

	res, err := s.validateExecutionPayloadBidParentValid(ctx, wsb.Block())
	require.Error(t, err)
	require.Equal(t, pubsub.ValidationReject, res)
}
