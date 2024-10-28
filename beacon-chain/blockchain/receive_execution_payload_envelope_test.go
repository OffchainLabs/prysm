package blockchain

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/das"
	mockExecution "github.com/prysmaticlabs/prysm/v5/beacon-chain/execution/testing"
	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/epbs"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func Test_getPayloadEnvelopePrestate(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, fcs := tr.ctx, tr.fcs

	gs, _ := util.DeterministicGenesisStateEpbs(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	require.NoError(t, fcs.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Root: service.originBlockRoot}))

	p := &enginev1.ExecutionPayloadEnvelope{
		Payload:         &enginev1.ExecutionPayloadElectra{},
		BeaconBlockRoot: service.originBlockRoot[:],
	}
	e, err := epbs.WrappedROExecutionPayloadEnvelope(p)
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
	p := &enginev1.ExecutionPayloadEnvelope{
		Payload:         &enginev1.ExecutionPayloadElectra{},
		BeaconBlockRoot: service.originBlockRoot[:],
	}
	e, err := epbs.WrappedROExecutionPayloadEnvelope(p)
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
	p := &enginev1.ExecutionPayloadEnvelope{
		Payload:         &enginev1.ExecutionPayloadElectra{},
		BeaconBlockRoot: service.originBlockRoot[:],
	}
	e, err := epbs.WrappedROExecutionPayloadEnvelope(p)
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
		Payload: &enginev1.ExecutionPayloadElectra{
			ParentHash: make([]byte, 32),
			BlockHash:  make([]byte, 32),
		},
		BeaconBlockRoot:    service.originBlockRoot[:],
		BlobKzgCommitments: make([][]byte, 0),
		StateRoot:          make([]byte, 32),
	}
	e, err := epbs.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	das := &das.MockAvailabilityStore{}

	blockHeader := post.LatestBlockHeader()
	prevStateRoot, err := post.HashTreeRoot(ctx)
	require.NoError(t, err)
	blockHeader.StateRoot = prevStateRoot[:]
	require.NoError(t, post.SetLatestBlockHeader(blockHeader))
	stRoot, err := post.HashTreeRoot(ctx)
	require.NoError(t, err)
	p.StateRoot = stRoot[:]
	engine := &mockExecution.EngineClient{}
	service.cfg.ExecutionEngineCaller = engine
	require.NoError(t, service.ReceiveExecutionPayloadEnvelope(ctx, e, das))
}
