//go:build minimal

package validator

import (
	"bytes"
	"context"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func testGloasBlock(t *testing.T) (*consensusblocks.GetPayloadResponse, interfaces.SignedBeaconBlock) {
	t.Helper()

	payload := &enginev1.ExecutionPayloadGloas{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     make([]byte, 0),
	}
	ed, err := consensusblocks.WrappedExecutionPayloadGloas(payload)
	require.NoError(t, err)

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData:          ed,
		Bid:                    big.NewInt(0),
		ExecutionRequestsGloas: &enginev1.ExecutionRequestsGloas{},
	}

	sBlk, err := consensusblocks.NewSignedBeaconBlock(util.NewBeaconBlockGloas())
	require.NoError(t, err)

	return local, sBlk
}

func TestStoreExecutionPayloadEnvelope(t *testing.T) {
	local, sBlk := testGloasBlock(t)

	vs := &Server{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
	envelope, err := vs.storeExecutionPayloadEnvelope(sBlk, local)
	require.NoError(t, err)
	require.Equal(t, sBlk.Block().Slot(), envelope.Payload.SlotNumber)

	contents, ok := vs.ExecutionPayloadEnvelopeCache.Contents()
	require.Equal(t, true, ok)
	require.Equal(t, sBlk.Block().Slot(), contents.Envelope.Payload.SlotNumber)
}

func TestExtractExecutionPayloadGloas(t *testing.T) {
	payload := &enginev1.ExecutionPayloadGloas{
		ParentHash:    make([]byte, 32),
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     make([]byte, 32),
		ExtraData:     make([]byte, 0),
	}
	ed, err := consensusblocks.WrappedExecutionPayloadGloas(payload)
	require.NoError(t, err)

	local := &consensusblocks.GetPayloadResponse{
		ExecutionData: ed,
		Bid:           big.NewInt(0),
	}

	result := extractExecutionPayloadGloas(local)
	require.NotNil(t, result)
	require.DeepEqual(t, payload, result)
}

func TestExtractExecutionPayloadGloas_Nil(t *testing.T) {
	require.Equal(t, true, extractExecutionPayloadGloas(nil) == nil)
	require.Equal(t, true, extractExecutionPayloadGloas(&consensusblocks.GetPayloadResponse{}) == nil)
}

func TestGetExecutionPayloadEnvelopeRPC_NilRequest(t *testing.T) {
	vs := &Server{}
	_, err := vs.GetExecutionPayloadEnvelope(t.Context(), nil)
	require.ErrorContains(t, "request cannot be nil", err)
}

func TestGetExecutionPayloadEnvelopeRPC_PreFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 10
	params.OverrideBeaconConfig(cfg)

	vs := &Server{}
	_, err := vs.GetExecutionPayloadEnvelope(t.Context(), &ethpb.ExecutionPayloadEnvelopeRequest{
		Slot: 0, // epoch 0, before GloasForkEpoch 10
	})
	require.ErrorContains(t, "not supported before Gloas fork", err)
}

func TestPublishExecutionPayloadEnvelope_NilRequest(t *testing.T) {
	vs := &Server{}
	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), nil)
	require.ErrorContains(t, "must set contents or signed_envelope", err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{SignedExecutionPayloadEnvelope: &ethpb.SignedExecutionPayloadEnvelope{}},
		},
	})
	require.ErrorContains(t, "signed envelope or payload cannot be nil", err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPublishExecutionPayloadEnvelope_PreFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 10
	params.OverrideBeaconConfig(cfg)

	vs := &Server{}
	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{
				SignedExecutionPayloadEnvelope: &ethpb.SignedExecutionPayloadEnvelope{
					Message: &ethpb.ExecutionPayloadEnvelope{
						Payload: &enginev1.ExecutionPayloadGloas{SlotNumber: 0}, // epoch 0, before GloasForkEpoch 10
					},
				},
			},
		},
	})
	require.ErrorContains(t, "not supported before Gloas fork", err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func statelessEnvelopeRequest(t *testing.T) *ethpb.GenericSignedExecutionPayloadEnvelope {
	t.Helper()
	require.NoError(t, kzg.Start())

	blobCount := 1
	rawBlobs := make([]kzg.Blob, blobCount)
	for i := range rawBlobs {
		rawBlobs[i] = kzg.Blob{uint8(i + 1)}
	}
	_, proofsPerBlob := util.GenerateCellsAndProofs(t, rawBlobs)

	flatBlobs := make([][]byte, blobCount)
	for i, b := range rawBlobs {
		flatBlobs[i] = b[:]
	}
	flatProofs := make([][]byte, 0, blobCount*fieldparams.NumberOfColumns)
	for _, proofs := range proofsPerBlob {
		for _, p := range proofs {
			flatProofs = append(flatProofs, p[:])
		}
	}
	signed := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     make([]byte, 32),
				ExtraData:     make([]byte, 0),
				SlotNumber:    1,
			},
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			BeaconBlockRoot:       make([]byte, 32),
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}

	return &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{
				SignedExecutionPayloadEnvelope: signed,
				Blobs:                          flatBlobs,
				KzgProofs:                      flatProofs,
			},
		},
	}
}

func TestPublishExecutionPayloadEnvelope_StatelessContents_RejectsBadProofs(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	req := statelessEnvelopeRequest(t)
	contents := req.Envelope.(*ethpb.GenericSignedExecutionPayloadEnvelope_Contents).Contents
	// Corrupt the first proof — verifyCellProofs must reject before any P2P/cache/receiver is touched.
	contents.KzgProofs[0] = bytes.Repeat([]byte{0xff}, 48)

	vs := &Server{}
	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), req)
	require.ErrorContains(t, "kzg verification failed", err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetExecutionPayloadEnvelopeRPC_Success(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
			SlotNumber:    1,
		},
		ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
		BuilderIndex:          primitives.BuilderIndex(0),
		BeaconBlockRoot:       make([]byte, 32),
		ParentBeaconBlockRoot: make([]byte, 32),
	}

	vs := &Server{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
	vs.ExecutionPayloadEnvelopeCache.Set(&cache.ExecutionPayloadContents{Envelope: envelope})

	resp, err := vs.GetExecutionPayloadEnvelope(t.Context(), &ethpb.ExecutionPayloadEnvelopeRequest{
		Slot: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Envelope)
	wantHTR, err := envelope.HashTreeRoot()
	require.NoError(t, err)
	gotHTR, err := resp.Envelope.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, wantHTR, gotHTR)
}

// Stateful publish: bare signed_envelope arm must match the cached envelope by HTR.
func TestPublishExecutionPayloadEnvelope_SignedEnvelopeArm(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
			ExtraData:     make([]byte, 0),
			SlotNumber:    1,
		},
		ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
		BuilderIndex:          0,
		BeaconBlockRoot:       make([]byte, 32),
		ParentBeaconBlockRoot: make([]byte, 32),
	}
	signed := &ethpb.SignedExecutionPayloadEnvelope{Message: envelope, Signature: make([]byte, 96)}
	statefulReq := &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_SignedEnvelope{SignedEnvelope: signed},
	}

	t.Run("cache miss", func(t *testing.T) {
		vs := &Server{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
		_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), statefulReq)
		require.ErrorContains(t, "no cached blobs and KZG proofs", err)
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
	})

	t.Run("cached envelope mismatch", func(t *testing.T) {
		tampered := proto.Clone(envelope).(*ethpb.ExecutionPayloadEnvelope)
		tampered.BuilderIndex = envelope.BuilderIndex + 1
		vs := &Server{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
		vs.ExecutionPayloadEnvelopeCache.Set(&cache.ExecutionPayloadContents{Envelope: tampered})
		_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), statefulReq)
		require.ErrorContains(t, "does not match submitted envelope", err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("success", func(t *testing.T) {
		broadcaster := &mockp2p.MockBroadcaster{}
		receiver := &mockExecutionPayloadEnvelopeReceiver{done: make(chan struct{})}
		vs := &Server{
			Ctx:                              t.Context(),
			P2P:                              broadcaster,
			ExecutionPayloadEnvelopeReceiver: receiver,
			ExecutionPayloadEnvelopeCache:    cache.NewExecutionPayloadEnvelopeCache(),
		}
		vs.ExecutionPayloadEnvelopeCache.Set(&cache.ExecutionPayloadContents{Envelope: envelope})
		resp, err := vs.PublishExecutionPayloadEnvelope(t.Context(), statefulReq)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, true, broadcaster.BroadcastCalled.Load())
		waitForEnvelopeImport(t, receiver)
		require.Equal(t, int32(1), receiver.calls.Load())
	})
}

func TestPublishExecutionPayloadEnvelope_Success(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	broadcaster := &mockp2p.MockBroadcaster{}
	receiver := &mockExecutionPayloadEnvelopeReceiver{done: make(chan struct{})}
	vs := &Server{
		Ctx:                              t.Context(),
		P2P:                              broadcaster,
		ExecutionPayloadEnvelopeReceiver: receiver,
	}

	req := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     make([]byte, 32),
				ExtraData:     make([]byte, 0),
				SlotNumber:    1,
			},
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			BuilderIndex:          0,
			BeaconBlockRoot:       make([]byte, 32),
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}

	resp, err := vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{SignedExecutionPayloadEnvelope: req},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, true, broadcaster.BroadcastCalled.Load())
	require.Equal(t, 1, len(broadcaster.BroadcastMessages))
	waitForEnvelopeImport(t, receiver)
	require.Equal(t, int32(1), receiver.calls.Load())
}

func TestPublishExecutionPayloadEnvelope_SideEffectOrder(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	events := make(chan string, 4)
	releaseImport := make(chan struct{})
	contexts := make(chan context.Context, 1)
	receiver := &mockExecutionPayloadEnvelopeReceiver{
		done:     make(chan struct{}),
		events:   events,
		contexts: contexts,
		release:  releaseImport,
	}
	dataReceiver := &mockDataColumnReceiver{events: events}
	broadcaster := &mockPublishEnvelopeBroadcaster{
		MockBroadcaster: &mockp2p.MockBroadcaster{},
		events:          events,
		dataColumnErr:   errors.New("data column broadcast failed"),
	}
	serviceCtx, cancelService := context.WithCancel(t.Context())
	defer cancelService()
	requestCtx, cancelRequest := context.WithCancel(t.Context())
	vs := &Server{
		Ctx:                              serviceCtx,
		P2P:                              broadcaster,
		DataColumnReceiver:               dataReceiver,
		ExecutionPayloadEnvelopeReceiver: receiver,
	}
	req := statelessEnvelopeRequest(t)

	result := make(chan error, 1)
	go func() {
		_, err := vs.PublishExecutionPayloadEnvelope(requestCtx, req)
		result <- err
	}()

	// Publishing returns even while the background import is blocked.
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("publish waited for background import")
	}

	var importCtx context.Context
	select {
	case importCtx = <-contexts:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for background import")
	}
	cancelRequest()
	require.NoError(t, importCtx.Err())

	for _, want := range []string{"broadcast data columns", "broadcast envelope", "import data columns", "import envelope"} {
		select {
		case got := <-events:
			require.Equal(t, want, got)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %q", want)
		}
	}

	close(releaseImport)
	waitForEnvelopeImport(t, receiver)
	require.Equal(t, int32(1), dataReceiver.calls.Load())
}

func TestPublishExecutionPayloadEnvelope_EnvelopeBroadcastFailure(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	events := make(chan string, 2)
	receiver := &mockExecutionPayloadEnvelopeReceiver{}
	dataReceiver := &mockDataColumnReceiver{}
	broadcaster := &mockPublishEnvelopeBroadcaster{
		MockBroadcaster: &mockp2p.MockBroadcaster{},
		events:          events,
		envelopeErr:     errors.New("envelope broadcast failed"),
	}
	vs := &Server{
		Ctx:                              t.Context(),
		P2P:                              broadcaster,
		DataColumnReceiver:               dataReceiver,
		ExecutionPayloadEnvelopeReceiver: receiver,
	}

	_, err := vs.PublishExecutionPayloadEnvelope(t.Context(), statelessEnvelopeRequest(t))
	require.ErrorContains(t, "failed to broadcast execution payload envelope", err)
	require.Equal(t, codes.Internal, status.Code(err))
	require.Equal(t, int32(0), dataReceiver.calls.Load())
	require.Equal(t, int32(0), receiver.calls.Load())
	for _, want := range []string{"broadcast data columns", "broadcast envelope"} {
		require.Equal(t, want, <-events)
	}
}

func TestPublishExecutionPayloadEnvelope_ImportFailureDoesNotFailPublish(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	broadcaster := &mockp2p.MockBroadcaster{}
	receiver := &mockExecutionPayloadEnvelopeReceiver{err: errors.New("import failed"), done: make(chan struct{})}
	vs := &Server{
		Ctx:                              t.Context(),
		P2P:                              broadcaster,
		ExecutionPayloadEnvelopeReceiver: receiver,
	}

	req := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     make([]byte, 32),
				ExtraData:     make([]byte, 0),
				SlotNumber:    1,
			},
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			BeaconBlockRoot:       make([]byte, 32),
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}

	resp, err := vs.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{SignedExecutionPayloadEnvelope: req},
		},
	})
	// Background import failure must not fail the publish.
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, true, broadcaster.BroadcastCalled.Load())
	waitForEnvelopeImport(t, receiver)
}

type mockExecutionPayloadEnvelopeReceiver struct {
	calls    atomic.Int32
	err      error
	done     chan struct{}
	events   chan<- string
	contexts chan<- context.Context
	release  <-chan struct{}
}

func (m *mockExecutionPayloadEnvelopeReceiver) ReceiveExecutionPayloadEnvelope(ctx context.Context, _ interfaces.ROSignedExecutionPayloadEnvelope) error {
	m.calls.Add(1)
	if m.events != nil {
		m.events <- "import envelope"
	}
	if m.contexts != nil {
		m.contexts <- ctx
	}
	if m.release != nil {
		<-m.release
	}
	if m.done != nil {
		close(m.done)
	}
	return m.err
}

type mockDataColumnReceiver struct {
	calls  atomic.Int32
	events chan<- string
}

func (m *mockDataColumnReceiver) ReceiveDataColumn(_ consensusblocks.VerifiedRODataColumn) error {
	m.calls.Add(1)
	return nil
}

func (m *mockDataColumnReceiver) ReceiveDataColumns(_ []consensusblocks.VerifiedRODataColumn) error {
	m.calls.Add(1)
	if m.events != nil {
		m.events <- "import data columns"
	}
	return nil
}

type mockPublishEnvelopeBroadcaster struct {
	*mockp2p.MockBroadcaster
	events        chan<- string
	dataColumnErr error
	envelopeErr   error
}

func (m *mockPublishEnvelopeBroadcaster) Broadcast(ctx context.Context, msg proto.Message) error {
	m.events <- "broadcast envelope"
	if m.envelopeErr != nil {
		return m.envelopeErr
	}
	return m.MockBroadcaster.Broadcast(ctx, msg)
}

func (m *mockPublishEnvelopeBroadcaster) BroadcastDataColumnSidecars(
	ctx context.Context,
	verified []consensusblocks.VerifiedRODataColumn,
	partial []consensusblocks.PartialDataColumn,
) error {
	m.events <- "broadcast data columns"
	if m.dataColumnErr != nil {
		return m.dataColumnErr
	}
	return m.MockBroadcaster.BroadcastDataColumnSidecars(ctx, verified, partial)
}

func waitForEnvelopeImport(t *testing.T, m *mockExecutionPayloadEnvelopeReceiver) {
	t.Helper()
	select {
	case <-m.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for background envelope import")
	}
}
