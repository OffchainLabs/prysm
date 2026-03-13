package blockchain

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func prepareGloasForkchoiceState(
	_ context.Context,
	slot primitives.Slot,
	blockRoot [32]byte,
	parentRoot [32]byte,
	blockHash [32]byte,
	parentBlockHash [32]byte,
	justifiedEpoch primitives.Epoch,
	finalizedEpoch primitives.Epoch,
) (state.BeaconState, blocks.ROBlock, error) {
	blockHeader := &ethpb.BeaconBlockHeader{
		ParentRoot: parentRoot[:],
	}

	justifiedCheckpoint := &ethpb.Checkpoint{
		Epoch: justifiedEpoch,
	}

	finalizedCheckpoint := &ethpb.Checkpoint{
		Epoch: finalizedEpoch,
	}

	builderPendingPayments := make([]*ethpb.BuilderPendingPayment, 64)
	for i := range builderPendingPayments {
		builderPendingPayments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, 20),
			},
		}
	}

	base := &ethpb.BeaconStateGloas{
		Slot:                       slot,
		RandaoMixes:                make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		CurrentJustifiedCheckpoint: justifiedCheckpoint,
		FinalizedCheckpoint:        finalizedCheckpoint,
		LatestBlockHeader:          blockHeader,
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
			BlockHash:          blockHash[:],
			ParentBlockHash:    parentBlockHash[:],
			ParentBlockRoot:    make([]byte, 32),
			PrevRandao:         make([]byte, 32),
			FeeRecipient:       make([]byte, 20),
			BlobKzgCommitments: [][]byte{make([]byte, 48)},
		},
		Builders:                     make([]*ethpb.Builder, 0),
		BuilderPendingPayments:       builderPendingPayments,
		ExecutionPayloadAvailability: make([]byte, 1024),
		LatestBlockHash:              make([]byte, 32),
		PayloadExpectedWithdrawals:   make([]*enginev1.Withdrawal, 0),
		ProposerLookahead:            make([]uint64, 64),
	}

	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	if err != nil {
		return nil, blocks.ROBlock{}, err
	}

	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash[:],
			ParentBlockHash: parentBlockHash[:],
		},
	})

	blk := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body: &ethpb.BeaconBlockBodyGloas{
				SignedExecutionPayloadBid: bid,
			},
		},
	})

	signed, err := blocks.NewSignedBeaconBlock(blk)
	if err != nil {
		return nil, blocks.ROBlock{}, err
	}
	roblock, err := blocks.NewROBlockWithRoot(signed, blockRoot)
	return st, roblock, err
}

func testGloasState(t *testing.T, slot primitives.Slot, parentRoot [32]byte, blockHash [32]byte) (*ethpb.BeaconStateGloas, *ethpb.SignedBeaconBlockGloas) {
	t.Helper()
	builderPendingPayments := make([]*ethpb.BuilderPendingPayment, 64)
	for i := range builderPendingPayments {
		builderPendingPayments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{FeeRecipient: make([]byte, 20)},
		}
	}
	base := &ethpb.BeaconStateGloas{
		Slot:                       slot,
		RandaoMixes:                make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		BlockRoots:                 make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot),
		StateRoots:                 make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot),
		Slashings:                  make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector),
		CurrentJustifiedCheckpoint: &ethpb.Checkpoint{Root: make([]byte, 32)},
		FinalizedCheckpoint:        &ethpb.Checkpoint{Root: make([]byte, 32)},
		LatestBlockHeader: &ethpb.BeaconBlockHeader{
			ParentRoot: parentRoot[:],
			StateRoot:  make([]byte, 32),
			BodyRoot:   make([]byte, 32),
		},
		Eth1Data: &ethpb.Eth1Data{
			DepositRoot: make([]byte, 32),
			BlockHash:   make([]byte, 32),
		},
		LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
			BlockHash:          blockHash[:],
			ParentBlockHash:    make([]byte, 32),
			ParentBlockRoot:    make([]byte, 32),
			PrevRandao:         make([]byte, 32),
			FeeRecipient:       make([]byte, 20),
			BlobKzgCommitments: [][]byte{make([]byte, 48)},
		},
		Builders:                     make([]*ethpb.Builder, 0),
		BuilderPendingPayments:       builderPendingPayments,
		ExecutionPayloadAvailability: make([]byte, 1024),
		LatestBlockHash:              make([]byte, 32),
		PayloadExpectedWithdrawals:   make([]*enginev1.Withdrawal, 0),
		ProposerLookahead:            make([]uint64, 64),
	}

	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash[:],
			ParentBlockHash: make([]byte, 32),
		},
	})

	blk := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body:       &ethpb.BeaconBlockBodyGloas{SignedExecutionPayloadBid: bid},
		},
	})
	return base, blk
}

func testSignedEnvelope(t *testing.T, blockRoot [32]byte, slot primitives.Slot, blockHash []byte) *ethpb.SignedExecutionPayloadEnvelope {
	t.Helper()
	return &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadDeneb{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     blockHash,
				Transactions:  [][]byte{},
				Withdrawals:   []*enginev1.Withdrawal{},
			},
			ExecutionRequests: &enginev1.ExecutionRequests{},
			BuilderIndex:      0,
			BeaconBlockRoot:   blockRoot[:],
			Slot:              slot,
			StateRoot:         make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}
}

func setupGloasService(t *testing.T, engineClient *mockExecution.EngineClient) (*Service, *testServiceRequirements) {
	t.Helper()
	return minimalTestService(t,
		WithPayloadIDCache(cache.NewPayloadIDCache()),
		WithExecutionEngineCaller(engineClient),
	)
}

func insertGloasBlock(t *testing.T, s *Service, base *ethpb.BeaconStateGloas, blk *ethpb.SignedBeaconBlockGloas, blockRoot [32]byte) {
	t.Helper()
	ctx := t.Context()
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)
	signed, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlockWithRoot(signed, blockRoot)
	require.NoError(t, err)
	require.NoError(t, s.cfg.BeaconDB.SaveBlock(ctx, signed))
	require.NoError(t, s.cfg.BeaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{Root: blockRoot[:], Slot: blk.Block.Slot}))
	require.NoError(t, s.cfg.StateGen.SaveState(ctx, blockRoot, st))
	require.NoError(t, s.InsertNode(ctx, st, roblock))
}

func TestGetPayloadEnvelopePrestate_UnknownRoot(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()
	unknownRoot := bytesutil.ToBytes32([]byte("unknown"))
	env := &ethpb.ExecutionPayloadEnvelope{
		BeaconBlockRoot: unknownRoot[:],
		Payload:         &enginev1.ExecutionPayloadDeneb{},
	}
	envelope, err := blocks.WrappedROExecutionPayloadEnvelope(env)
	require.NoError(t, err)
	_, err = s.getPayloadEnvelopePrestate(ctx, envelope)
	require.ErrorContains(t, "not found in forkchoice", err)
}

func TestGetPayloadEnvelopePrestate_OK(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()

	blockRoot := bytesutil.ToBytes32([]byte("root1"))
	parentRoot := params.BeaconConfig().ZeroHash
	blockHash := bytesutil.ToBytes32([]byte("hash1"))

	base, blk := testGloasState(t, 1, parentRoot, blockHash)
	insertGloasBlock(t, s, base, blk, blockRoot)

	env := &ethpb.ExecutionPayloadEnvelope{
		BeaconBlockRoot: blockRoot[:],
		Payload:         &enginev1.ExecutionPayloadDeneb{},
	}
	envelope, err := blocks.WrappedROExecutionPayloadEnvelope(env)
	require.NoError(t, err)
	st, err := s.getPayloadEnvelopePrestate(ctx, envelope)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(1), st.Slot())
}

func TestNotifyNewEnvelope_Valid(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()

	blockRoot := bytesutil.ToBytes32([]byte("root1"))
	parentRoot := params.BeaconConfig().ZeroHash
	blockHash := bytesutil.ToBytes32([]byte("hash1"))

	base, _ := testGloasState(t, 1, parentRoot, blockHash)
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)

	env := &ethpb.ExecutionPayloadEnvelope{
		BeaconBlockRoot:   blockRoot[:],
		Payload:           &enginev1.ExecutionPayloadDeneb{BlockHash: blockHash[:]},
		ExecutionRequests: &enginev1.ExecutionRequests{},
		Slot:              1,
	}
	envelope, err := blocks.WrappedROExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	isValid, err := s.notifyNewEnvelope(ctx, st, envelope)
	require.NoError(t, err)
	require.Equal(t, true, isValid)
}

func TestNotifyNewEnvelope_Syncing(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{
		ErrNewPayload: execution.ErrAcceptedSyncingPayloadStatus,
	})
	ctx := t.Context()

	blockRoot := bytesutil.ToBytes32([]byte("root1"))
	parentRoot := params.BeaconConfig().ZeroHash
	blockHash := bytesutil.ToBytes32([]byte("hash1"))

	base, _ := testGloasState(t, 1, parentRoot, blockHash)
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)

	env := &ethpb.ExecutionPayloadEnvelope{
		BeaconBlockRoot:   blockRoot[:],
		Payload:           &enginev1.ExecutionPayloadDeneb{BlockHash: blockHash[:]},
		ExecutionRequests: &enginev1.ExecutionRequests{},
		Slot:              1,
	}
	envelope, err := blocks.WrappedROExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	isValid, err := s.notifyNewEnvelope(ctx, st, envelope)
	require.NoError(t, err)
	require.Equal(t, false, isValid)
}

func TestNotifyNewEnvelope_Invalid(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{
		ErrNewPayload: execution.ErrInvalidPayloadStatus,
	})
	ctx := t.Context()

	blockRoot := bytesutil.ToBytes32([]byte("root1"))
	parentRoot := params.BeaconConfig().ZeroHash
	blockHash := bytesutil.ToBytes32([]byte("hash1"))

	base, _ := testGloasState(t, 1, parentRoot, blockHash)
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)

	env := &ethpb.ExecutionPayloadEnvelope{
		BeaconBlockRoot:   blockRoot[:],
		Payload:           &enginev1.ExecutionPayloadDeneb{BlockHash: blockHash[:]},
		ExecutionRequests: &enginev1.ExecutionRequests{},
		Slot:              1,
	}
	envelope, err := blocks.WrappedROExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	_, err = s.notifyNewEnvelope(ctx, st, envelope)
	require.Equal(t, true, IsInvalidBlock(err))
}

func TestNotifyForkchoiceUpdateGloas_Valid(t *testing.T) {
	pid := &enginev1.PayloadIDBytes{1, 2, 3, 4, 5, 6, 7, 8}
	s, _ := setupGloasService(t, &mockExecution.EngineClient{PayloadIDBytes: pid})
	ctx := t.Context()

	blockHash := bytesutil.ToBytes32([]byte("hash1"))
	attr := payloadattribute.EmptyWithVersion(version.Gloas)

	retPid, err := s.notifyForkchoiceUpdateGloas(ctx, blockHash, attr)
	require.NoError(t, err)
	require.DeepEqual(t, pid, retPid)
}

func TestNotifyForkchoiceUpdateGloas_Syncing(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{
		ErrForkchoiceUpdated: execution.ErrAcceptedSyncingPayloadStatus,
	})
	ctx := t.Context()

	blockHash := bytesutil.ToBytes32([]byte("hash1"))
	_, err := s.notifyForkchoiceUpdateGloas(ctx, blockHash, nil)
	require.NoError(t, err)
}

func TestNotifyForkchoiceUpdateGloas_Invalid(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{
		ErrForkchoiceUpdated: execution.ErrInvalidPayloadStatus,
	})
	ctx := t.Context()

	blockHash := bytesutil.ToBytes32([]byte("hash1"))
	_, err := s.notifyForkchoiceUpdateGloas(ctx, blockHash, nil)
	require.Equal(t, true, IsInvalidBlock(err))
}

func TestNotifyForkchoiceUpdateGloas_NilAttributes(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()

	blockHash := bytesutil.ToBytes32([]byte("hash1"))
	_, err := s.notifyForkchoiceUpdateGloas(ctx, blockHash, nil)
	require.NoError(t, err)
}

func TestSavePostPayload(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()

	blockRoot := bytesutil.ToBytes32([]byte("root1"))
	blockHash := bytesutil.ToBytes32([]byte("hash1"))

	base, _ := testGloasState(t, 1, params.BeaconConfig().ZeroHash, blockHash)
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)

	protoEnv := testSignedEnvelope(t, blockRoot, 1, blockHash[:])
	signed, err := blocks.WrappedROSignedExecutionPayloadEnvelope(protoEnv)
	require.NoError(t, err)

	require.NoError(t, s.savePostPayload(ctx, signed, st))

	// Verify the envelope was saved in the DB.
	require.Equal(t, true, s.cfg.BeaconDB.HasExecutionPayloadEnvelope(ctx, blockRoot))
}

func TestValidateExecutionOnEnvelope_Valid(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()

	blockRoot := bytesutil.ToBytes32([]byte("root1"))
	parentRoot := params.BeaconConfig().ZeroHash
	blockHash := bytesutil.ToBytes32([]byte("hash1"))

	base, _ := testGloasState(t, 1, parentRoot, blockHash)
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)

	env := &ethpb.ExecutionPayloadEnvelope{
		BeaconBlockRoot:   blockRoot[:],
		Payload:           &enginev1.ExecutionPayloadDeneb{BlockHash: blockHash[:], ParentHash: make([]byte, 32)},
		ExecutionRequests: &enginev1.ExecutionRequests{},
		Slot:              1,
	}
	envelope, err := blocks.WrappedROExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	isValid, err := s.validateExecutionOnEnvelope(ctx, st, envelope)
	require.NoError(t, err)
	require.Equal(t, true, isValid)
}

func TestPostPayloadHeadUpdate_NotHead(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()

	root := bytesutil.ToBytes32([]byte("root1"))
	headRoot := bytesutil.ToBytes32([]byte("different"))
	blockHash := bytesutil.ToBytes32([]byte("hash1"))

	base, _ := testGloasState(t, 1, params.BeaconConfig().ZeroHash, blockHash)
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)

	env := &ethpb.ExecutionPayloadEnvelope{
		BeaconBlockRoot: root[:],
		Payload:         &enginev1.ExecutionPayloadDeneb{BlockHash: blockHash[:]},
		Slot:            1,
	}
	envelope, err := blocks.WrappedROExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	require.NoError(t, s.postPayloadHeadUpdate(ctx, envelope, st, root, headRoot[:]))
}

func TestPostPayloadHeadUpdate_SetsHeadFull(t *testing.T) {
	s, _ := setupGloasService(t, &mockExecution.EngineClient{})
	ctx := t.Context()

	root := bytesutil.ToBytes32([]byte("root1"))
	blockHash := bytesutil.ToBytes32([]byte("hash1"))

	base, blk := testGloasState(t, 1, params.BeaconConfig().ZeroHash, blockHash)
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)
	signed, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	s.head = &head{root: root, block: signed, state: st, slot: 1}
	require.Equal(t, false, s.head.full)

	env := &ethpb.ExecutionPayloadEnvelope{
		BeaconBlockRoot: root[:],
		Payload:         &enginev1.ExecutionPayloadDeneb{BlockHash: blockHash[:], ParentHash: make([]byte, 32)},
		Slot:            1,
	}
	envelope, err := blocks.WrappedROExecutionPayloadEnvelope(env)
	require.NoError(t, err)

	require.NoError(t, s.postPayloadHeadUpdate(ctx, envelope, st, root, root[:]))

	s.headLock.RLock()
	require.Equal(t, true, s.head.full)
	s.headLock.RUnlock()
}

func TestGetLookupParentRoot_PreGloas(t *testing.T) {
	service, _ := minimalTestService(t)

	parentRoot := [32]byte{1}
	blk := util.HydrateSignedBeaconBlockDeneb(&ethpb.SignedBeaconBlockDeneb{
		Block: &ethpb.BeaconBlockDeneb{
			ParentRoot: parentRoot[:],
		},
	})
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlock(wsb)
	require.NoError(t, err)

	got, err := service.getLookupParentRoot(roblock)
	require.NoError(t, err)
	require.Equal(t, parentRoot, got)
}

func TestGetLookupParentRoot_GloasBuildsOnEmpty(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.InitializeForkSchedule()
	params.OverrideBeaconConfig(cfg)

	service, req := minimalTestService(t)
	ctx := t.Context()

	parentRoot := [32]byte{1}
	parentBlockHash := [32]byte{20}
	parentNodeBlockHash := [32]byte{99} // Different from parentBlockHash => builds on empty

	// Insert a Gloas node for the parent so BlockHash works.
	st, parentROBlock, err := prepareGloasForkchoiceState(ctx, 1, parentRoot, params.BeaconConfig().ZeroHash, parentNodeBlockHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, req.fcs.InsertNode(ctx, st, parentROBlock))

	blockHash := [32]byte{10}
	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash[:],
			ParentBlockHash: parentBlockHash[:],
		},
	})

	blk := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       2,
			ParentRoot: parentRoot[:],
			Body: &ethpb.BeaconBlockBodyGloas{
				SignedExecutionPayloadBid: bid,
			},
		},
	})
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlock(wsb)
	require.NoError(t, err)

	got, err := service.getLookupParentRoot(roblock)
	require.NoError(t, err)
	// parentBlockHash != parentNodeBlockHash, so it builds on empty => returns parentRoot
	require.Equal(t, parentRoot, got)
}

func TestGetLookupParentRoot_GloasBuildsOnFull(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.InitializeForkSchedule()
	params.OverrideBeaconConfig(cfg)

	service, req := minimalTestService(t)
	ctx := t.Context()

	parentRoot := [32]byte{1}
	parentNodeBlockHash := [32]byte{10}

	// Insert a Gloas node for the parent so BlockHash works.
	st, parentROBlock, err := prepareGloasForkchoiceState(ctx, 1, parentRoot, params.BeaconConfig().ZeroHash, parentNodeBlockHash, params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, req.fcs.InsertNode(ctx, st, parentROBlock))

	// Set parentBlockHash in the bid to match the parent's blockHash.
	blockHash := [32]byte{20}
	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash[:],
			ParentBlockHash: parentNodeBlockHash[:],
		},
	})

	blk := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       2,
			ParentRoot: parentRoot[:],
			Body: &ethpb.BeaconBlockBodyGloas{
				SignedExecutionPayloadBid: bid,
			},
		},
	})
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlock(wsb)
	require.NoError(t, err)

	got, err := service.getLookupParentRoot(roblock)
	require.NoError(t, err)
	// parentBlockHash == parentNodeBlockHash, so it builds on full => returns parentBlockHash
	require.Equal(t, parentNodeBlockHash, got)
}

func TestGetLookupParentRoot_GloasParentPreForkEpoch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 2
	cfg.InitializeForkSchedule()
	params.OverrideBeaconConfig(cfg)

	service, req := minimalTestService(t)
	ctx := t.Context()

	parentRoot := [32]byte{1}
	parentNodeBlockHash := [32]byte{10}
	parentSlot, err := slots.EpochStart(params.BeaconConfig().GloasForkEpoch)
	require.NoError(t, err)
	parentSlot = parentSlot - 1

	st, parentROBlock, err := prepareGloasForkchoiceState(
		ctx,
		parentSlot,
		parentRoot,
		params.BeaconConfig().ZeroHash,
		parentNodeBlockHash,
		params.BeaconConfig().ZeroHash,
		0,
		0,
	)
	require.NoError(t, err)
	require.NoError(t, req.fcs.InsertNode(ctx, st, parentROBlock))

	blockHash := [32]byte{20}
	bid := util.HydrateSignedExecutionPayloadBid(&ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			BlockHash:       blockHash[:],
			ParentBlockHash: parentNodeBlockHash[:],
		},
	})

	blk := util.HydrateSignedBeaconBlockGloas(&ethpb.SignedBeaconBlockGloas{
		Block: &ethpb.BeaconBlockGloas{
			Slot:       parentSlot + 1,
			ParentRoot: parentRoot[:],
			Body: &ethpb.BeaconBlockBodyGloas{
				SignedExecutionPayloadBid: bid,
			},
		},
	})
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlock(wsb)
	require.NoError(t, err)

	got, err := service.getLookupParentRoot(roblock)
	require.NoError(t, err)
	// Parent slot is pre-fork, so always return parentRoot.
	require.Equal(t, parentRoot, got)
}

func TestLatePayloadTasks_ReturnsEarlyWhenBlockLate(t *testing.T) {
	logHook := logTest.NewGlobal()
	service, tr := setupGloasService(t, &mockExecution.EngineClient{})

	blockHash := bytesutil.ToBytes32([]byte("hash1"))
	base, _ := testGloasState(t, 1, params.BeaconConfig().ZeroHash, blockHash)
	base.LatestBlockHash = blockHash[:]
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)

	headRoot := bytesutil.ToBytes32([]byte("headroot"))
	service.head = &head{
		root:  headRoot,
		state: st,
		slot:  1,
	}
	// Set genesis time so CurrentSlot > HeadSlot.
	service.SetGenesisTime(time.Now().Add(-2 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second))

	service.latePayloadTasks(tr.ctx)
	require.LogsDoNotContain(t, logHook, "Could not notify forkchoice update")
	// No payload ID should have been cached.
	_, has := service.cfg.PayloadIDCache.PayloadID(service.CurrentSlot()+1, headRoot)
	require.Equal(t, false, has)
}

func TestLatePayloadTasks_SendsFCU(t *testing.T) {
	logHook := logTest.NewGlobal()
	resetCfg := features.InitWithReset(&features.Flags{
		PrepareAllPayloads: true,
	})
	defer resetCfg()

	pid := &enginev1.PayloadIDBytes{1, 2, 3, 4, 5, 6, 7, 8}
	service, tr := setupGloasService(t, &mockExecution.EngineClient{PayloadIDBytes: pid})

	blockHash := bytesutil.ToBytes32([]byte("hash1"))
	base, blk := testGloasState(t, 1, params.BeaconConfig().ZeroHash, blockHash)
	base.LatestBlockHash = blockHash[:]
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)

	signed, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	headRoot := bytesutil.ToBytes32([]byte("headroot"))
	service.head = &head{
		root:  headRoot,
		block: signed,
		state: st,
		slot:  1,
	}
	// CurrentSlot == HeadSlot == 1: place genesis 1.5 slots ago so we're solidly in slot 1.
	service.SetGenesisTime(time.Now().Add(-3 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second / 2))
	service.SetForkChoiceGenesisTime(service.genesisTime)

	service.latePayloadTasks(tr.ctx)
	require.LogsDoNotContain(t, logHook, "Could not notify forkchoice update")
	require.LogsDoNotContain(t, logHook, "Could not get")
	// Payload ID should have been cached.
	cachedPid, has := service.cfg.PayloadIDCache.PayloadID(service.CurrentSlot()+1, headRoot)
	require.Equal(t, true, has)
	require.Equal(t, primitives.PayloadID(pid[:]), cachedPid)
}

func TestLateBlockTasks_GloasFCU(t *testing.T) {
	logHook := logTest.NewGlobal()
	resetCfg := features.InitWithReset(&features.Flags{
		PrepareAllPayloads: true,
	})
	defer resetCfg()

	pid := &enginev1.PayloadIDBytes{1, 2, 3, 4, 5, 6, 7, 8}
	service, tr := setupGloasService(t, &mockExecution.EngineClient{PayloadIDBytes: pid})

	blockHash := bytesutil.ToBytes32([]byte("hash1"))
	base, _ := testGloasState(t, 1, params.BeaconConfig().ZeroHash, blockHash)
	base.LatestBlockHash = blockHash[:]
	st, err := state_native.InitializeFromProtoUnsafeGloas(base)
	require.NoError(t, err)

	headRoot := bytesutil.ToBytes32([]byte("headroot"))
	service.head = &head{
		root:  headRoot,
		state: st,
		slot:  1,
	}

	// Set genesis time so CurrentSlot > HeadSlot, triggering late block logic.
	service.SetGenesisTime(time.Now().Add(-2 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second))
	service.SetForkChoiceGenesisTime(service.genesisTime)

	service.lateBlockTasks(tr.ctx)
	require.LogsDoNotContain(t, logHook, "could not perform late block tasks")
}
