package blockchain

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	mockExecution "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	forkchoicetypes "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/testing/util/random"
)

func Test_getPayloadEnvelopePrestate(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, fcs := tr.ctx, tr.fcs

	gs, _ := util.DeterministicGenesisStateEpbs(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	require.NoError(t, fcs.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Root: service.originBlockRoot}))

	p := random.ExecutionPayloadEnvelope(t)
	p.BeaconBlockRoot = service.originBlockRoot[:]
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)

	_, err = service.getPayloadEnvelopePrestate(ctx, e)
	require.NoError(t, err)
}

func Test_notifyNewEnvelope(t *testing.T) {
	service, tr := minimalTestService(t, WithPayloadIDCache(cache.NewPayloadIDCache()))
	ctx, fcs := tr.ctx, tr.fcs
	gs, _ := util.DeterministicGenesisStateEpbs(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	require.NoError(t, fcs.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Root: service.originBlockRoot}))
	p := random.ExecutionPayloadEnvelope(t)
	p.BeaconBlockRoot = service.originBlockRoot[:]
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	engine := &mockExecution.EngineClient{}
	service.cfg.ExecutionEngineCaller = engine
	isValidPayload, err := service.notifyNewEnvelope(ctx, e)
	require.NoError(t, err)
	require.Equal(t, true, isValidPayload)
}

func Test_validateExecutionOnEnvelope(t *testing.T) {
	service, tr := minimalTestService(t, WithPayloadIDCache(cache.NewPayloadIDCache()))
	ctx, fcs := tr.ctx, tr.fcs
	gs, _ := util.DeterministicGenesisStateEpbs(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	require.NoError(t, fcs.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Root: service.originBlockRoot}))
	p := random.ExecutionPayloadEnvelope(t)
	p.BeaconBlockRoot = service.originBlockRoot[:]
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	engine := &mockExecution.EngineClient{}
	service.cfg.ExecutionEngineCaller = engine
	isValidPayload, err := service.validateExecutionOnEnvelope(ctx, e)
	require.NoError(t, err)
	require.Equal(t, true, isValidPayload)
}

func Test_ReceiveExecutionPayloadEnvelope(t *testing.T) {
	service, tr := minimalTestService(t, WithPayloadIDCache(cache.NewPayloadIDCache()))
	ctx, fcs := tr.ctx, tr.fcs
	gs, _ := util.DeterministicGenesisStateEpbs(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	require.NoError(t, fcs.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Root: service.originBlockRoot}))
	post := gs.Copy()
	p := &enginev1.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadDeneb{
			ParentHash: make([]byte, 32),
			BlockHash:  make([]byte, 32),
		},
		BeaconBlockRoot:    service.originBlockRoot[:],
		BlobKzgCommitments: make([][]byte, 0),
		BeaconStateRoot:    make([]byte, 32),
		ExecutionRequests:  &enginev1.ExecutionRequests{},
	}
	sp := &enginev1.SignedExecutionPayloadEnvelope{
		Message: p,
	}
	e, err := blocks.WrappedROSignedExecutionPayloadEnvelope(sp)
	require.NoError(t, err)
	das := &das.MockAvailabilityStore{}

	blockHeader := post.LatestBlockHeader()
	prevStateRoot, err := post.HashTreeRoot(ctx)
	require.NoError(t, err)
	blockHeader.StateRoot = prevStateRoot[:]
	require.NoError(t, post.SetLatestBlockHeader(blockHeader))
	stRoot, err := post.HashTreeRoot(ctx)
	require.NoError(t, err)
	p.BeaconStateRoot = stRoot[:]
	engine := &mockExecution.EngineClient{}
	service.cfg.ExecutionEngineCaller = engine
	require.NoError(t, service.ReceiveExecutionPayloadEnvelope(ctx, e, das))
}
