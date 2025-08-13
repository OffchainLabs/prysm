package validator

import (
	"context"
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed"
	blockfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/block"
	statefeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/state"
	dbTest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/mock"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServer_StreamAltairBlocksVerified_ContextCanceled(t *testing.T) {
	ctx := t.Context()

	chainService := &chainMock.ChainService{}
	ctx, cancel := context.WithCancel(ctx)
	server := &Server{
		Ctx:           ctx,
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
	}

	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidatorAltair_StreamBlocksServer(ctrl)
	mockStream.EXPECT().Context().Return(ctx)
	go func(tt *testing.T) {
		assert.ErrorContains(tt, "Context canceled", server.StreamBlocksAltair(&ethpb.StreamBlocksRequest{
			VerifiedOnly: true,
		}, mockStream))
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestServer_StreamAltairBlocks_ContextCanceled(t *testing.T) {
	ctx := t.Context()

	chainService := &chainMock.ChainService{}
	ctx, cancel := context.WithCancel(ctx)
	server := &Server{
		Ctx:           ctx,
		BlockNotifier: chainService.BlockNotifier(),
		HeadFetcher:   chainService,
	}

	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidatorAltair_StreamBlocksServer(ctrl)
	mockStream.EXPECT().Context().Return(ctx)
	go func(tt *testing.T) {
		assert.ErrorContains(tt, "Context canceled", server.StreamBlocksAltair(&ethpb.StreamBlocksRequest{}, mockStream))
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestServer_StreamAltairBlocks_OnHeadUpdated(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())
	ctx := t.Context()
	beaconState, privs := util.DeterministicGenesisStateAltair(t, 64)
	c, err := altair.NextSyncCommittee(ctx, beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(c))

	b, err := util.GenerateFullBlockAltair(beaconState, privs, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	chainService := &chainMock.ChainService{State: beaconState}
	server := &Server{
		Ctx:           ctx,
		BlockNotifier: chainService.BlockNotifier(),
		HeadFetcher:   chainService,
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidatorAltair_StreamBlocksServer(ctrl)

	mockStream.EXPECT().Send(&ethpb.StreamBlocksResponse{Block: &ethpb.StreamBlocksResponse_AltairBlock{AltairBlock: b}}).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		err := server.StreamBlocksAltair(&ethpb.StreamBlocksRequest{}, mockStream)
		if s, _ := status.FromError(err); s.Code() != codes.Canceled {
			assert.NoError(tt, err)
		}
	}(t)
	wrappedBlk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = server.BlockNotifier.BlockFeed().Send(&feed.Event{
			Type: blockfeed.ReceivedBlock,
			Data: &blockfeed.ReceivedBlockData{SignedBlock: wrappedBlk},
		})
	}
	<-exitRoutine
}

func TestServer_StreamCapellaBlocks_OnHeadUpdated(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())
	ctx := t.Context()
	beaconState, privs := util.DeterministicGenesisStateCapella(t, 64)
	c, err := altair.NextSyncCommittee(ctx, beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(c))

	b, err := util.GenerateFullBlockCapella(beaconState, privs, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	chainService := &chainMock.ChainService{State: beaconState}
	server := &Server{
		Ctx:           ctx,
		BlockNotifier: chainService.BlockNotifier(),
		HeadFetcher:   chainService,
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidatorAltair_StreamBlocksServer(ctrl)

	mockStream.EXPECT().Send(&ethpb.StreamBlocksResponse{Block: &ethpb.StreamBlocksResponse_CapellaBlock{CapellaBlock: b}}).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		err := server.StreamBlocksAltair(&ethpb.StreamBlocksRequest{}, mockStream)
		if s, _ := status.FromError(err); s.Code() != codes.Canceled {
			assert.NoError(tt, err)
		}
	}(t)
	wrappedBlk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = server.BlockNotifier.BlockFeed().Send(&feed.Event{
			Type: blockfeed.ReceivedBlock,
			Data: &blockfeed.ReceivedBlockData{SignedBlock: wrappedBlk},
		})
	}
	<-exitRoutine
}

func TestServer_StreamAltairBlocksVerified_OnHeadUpdated(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := t.Context()
	beaconState, privs := util.DeterministicGenesisStateAltair(t, 32)
	c, err := altair.NextSyncCommittee(ctx, beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(c))

	b, err := util.GenerateFullBlockAltair(beaconState, privs, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wrappedBlk := util.SaveBlock(t, ctx, db, b)
	chainService := &chainMock.ChainService{State: beaconState}
	server := &Server{
		Ctx:           ctx,
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidatorAltair_StreamBlocksServer(ctrl)
	mockStream.EXPECT().Send(&ethpb.StreamBlocksResponse{Block: &ethpb.StreamBlocksResponse_AltairBlock{AltairBlock: b}}).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		err := server.StreamBlocksAltair(&ethpb.StreamBlocksRequest{VerifiedOnly: true}, mockStream)
		if s, _ := status.FromError(err); s.Code() != codes.Canceled {
			assert.NoError(tt, err)
		}
	}(t)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = server.StateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.BlockProcessed,
			Data: &statefeed.BlockProcessedData{Slot: b.Block.Slot, BlockRoot: r, SignedBlock: wrappedBlk},
		})
	}
	<-exitRoutine
}

func TestServer_StreamCapellaBlocksVerified_OnHeadUpdated(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := t.Context()
	beaconState, privs := util.DeterministicGenesisStateCapella(t, 32)
	c, err := altair.NextSyncCommittee(ctx, beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(c))

	b, err := util.GenerateFullBlockCapella(beaconState, privs, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wrappedBlk := util.SaveBlock(t, ctx, db, b)
	chainService := &chainMock.ChainService{State: beaconState}
	server := &Server{
		Ctx:           ctx,
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidatorAltair_StreamBlocksServer(ctrl)
	mockStream.EXPECT().Send(&ethpb.StreamBlocksResponse{Block: &ethpb.StreamBlocksResponse_CapellaBlock{CapellaBlock: b}}).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		err := server.StreamBlocksAltair(&ethpb.StreamBlocksRequest{VerifiedOnly: true}, mockStream)
		if s, _ := status.FromError(err); s.Code() != codes.Canceled {
			assert.NoError(tt, err)
		}
	}(t)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = server.StateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.BlockProcessed,
			Data: &statefeed.BlockProcessedData{Slot: b.Block.Slot, BlockRoot: r, SignedBlock: wrappedBlk},
		})
	}
	<-exitRoutine
}

func TestServer_StreamSlotsVerified_ContextCanceled(t *testing.T) {
	ctx := t.Context()

	chainService := &chainMock.ChainService{}
	ctx, cancel := context.WithCancel(ctx)
	server := &Server{
		Ctx:           ctx,
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
	}

	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_StreamSlotsServer(ctrl)
	mockStream.EXPECT().Context().Return(ctx)
	go func(tt *testing.T) {
		assert.ErrorContains(tt, "Context canceled", server.StreamSlots(&ethpb.StreamSlotsRequest{
			VerifiedOnly: true,
		}, mockStream))
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestServer_StreamSlots_ContextCanceled(t *testing.T) {
	ctx := t.Context()

	chainService := &chainMock.ChainService{}
	ctx, cancel := context.WithCancel(ctx)
	server := &Server{
		Ctx:           ctx,
		BlockNotifier: chainService.BlockNotifier(),
		HeadFetcher:   chainService,
	}

	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_StreamSlotsServer(ctrl)
	mockStream.EXPECT().Context().Return(ctx)
	go func(tt *testing.T) {
		assert.ErrorContains(tt, "Context canceled", server.StreamSlots(&ethpb.StreamSlotsRequest{}, mockStream))
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestServer_StreamSlots_OnHeadUpdated(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())
	ctx := t.Context()

	chainService := &chainMock.ChainService{}
	server := &Server{
		Ctx:               ctx,
		ForkchoiceFetcher: chainService,
		BlockNotifier:     chainService.BlockNotifier(),
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_StreamSlotsServer(ctrl)

	mockStream.EXPECT().Send(&ethpb.StreamSlotsResponse{
		Slot:                      123,
		PreviousDutyDependentRoot: params.BeaconConfig().ZeroHash[:],
		CurrentDutyDependentRoot:  params.BeaconConfig().ZeroHash[:],
	}).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		err := server.StreamSlots(&ethpb.StreamSlotsRequest{}, mockStream)
		if s, _ := status.FromError(err); s.Code() != codes.Canceled {
			assert.NoError(tt, err)
		}
	}(t)
	wrappedBlk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlock{Block: &ethpb.BeaconBlock{Slot: 123, Body: &ethpb.BeaconBlockBody{}}})
	require.NoError(t, err)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = server.BlockNotifier.BlockFeed().Send(&feed.Event{
			Type: blockfeed.ReceivedBlock,
			Data: &blockfeed.ReceivedBlockData{SignedBlock: wrappedBlk},
		})
	}
	<-exitRoutine
}

func TestServer_StreamSlotsVerified_OnHeadUpdated(t *testing.T) {
	ctx := t.Context()
	chainService := &chainMock.ChainService{}
	server := &Server{
		Ctx:               ctx,
		ForkchoiceFetcher: chainService,
		StateNotifier:     chainService.StateNotifier(),
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_StreamSlotsServer(ctrl)
	mockStream.EXPECT().Send(&ethpb.StreamSlotsResponse{
		Slot:                      123,
		PreviousDutyDependentRoot: params.BeaconConfig().ZeroHash[:],
		CurrentDutyDependentRoot:  params.BeaconConfig().ZeroHash[:],
	}).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		err := server.StreamSlots(&ethpb.StreamSlotsRequest{VerifiedOnly: true}, mockStream)
		if s, _ := status.FromError(err); s.Code() != codes.Canceled {
			assert.NoError(tt, err)
		}
	}(t)
	wrappedBlk, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlock{Block: &ethpb.BeaconBlock{Slot: 123, Body: &ethpb.BeaconBlockBody{}}})
	require.NoError(t, err)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = server.StateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.BlockProcessed,
			Data: &statefeed.BlockProcessedData{Slot: 123, BlockRoot: [32]byte{}, SignedBlock: wrappedBlk},
		})
	}
	<-exitRoutine
}

func TestServer_StreamBlocksVerified_FuluBlock(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := t.Context()
	beaconState, privs := util.DeterministicGenesisStateFulu(t, 32)
	c, err := altair.NextSyncCommittee(ctx, beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(c))

	b, err := util.GenerateFullBlockFulu(beaconState, privs, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wrappedBlk := util.SaveBlock(t, ctx, db, b)
	chainService := &chainMock.ChainService{State: beaconState}
	server := &Server{
		Ctx:           ctx,
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidatorAltair_StreamBlocksServer(ctrl)
	mockStream.EXPECT().Send(&ethpb.StreamBlocksResponse{Block: &ethpb.StreamBlocksResponse_FuluBlock{FuluBlock: b}}).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		err := server.StreamBlocksAltair(&ethpb.StreamBlocksRequest{VerifiedOnly: true}, mockStream)
		if s, _ := status.FromError(err); s.Code() != codes.Canceled {
			assert.NoError(tt, err)
		}
	}(t)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = server.StateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.BlockProcessed,
			Data: &statefeed.BlockProcessedData{Slot: b.Block.Slot, BlockRoot: r, SignedBlock: wrappedBlk},
		})
	}
	<-exitRoutine
}

func TestServer_StreamBlocks_FuluBlock(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())
	ctx := t.Context()
	beaconState, privs := util.DeterministicGenesisStateFulu(t, 64)
	c, err := altair.NextSyncCommittee(ctx, beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetCurrentSyncCommittee(c))

	b, err := util.GenerateFullBlockFulu(beaconState, privs, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	chainService := &chainMock.ChainService{State: beaconState}
	server := &Server{
		Ctx:           ctx,
		BlockNotifier: chainService.BlockNotifier(),
		HeadFetcher:   chainService,
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidatorAltair_StreamBlocksServer(ctrl)

	mockStream.EXPECT().Send(&ethpb.StreamBlocksResponse{Block: &ethpb.StreamBlocksResponse_FuluBlock{FuluBlock: b}}).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		err := server.StreamBlocksAltair(&ethpb.StreamBlocksRequest{}, mockStream)
		if s, _ := status.FromError(err); s.Code() != codes.Canceled {
			assert.NoError(tt, err)
		}
	}(t)
	wrappedBlk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = server.BlockNotifier.BlockFeed().Send(&feed.Event{
			Type: blockfeed.ReceivedBlock,
			Data: &blockfeed.ReceivedBlockData{SignedBlock: wrappedBlk},
		})
	}
	<-exitRoutine
}
