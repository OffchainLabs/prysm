package core

import (
	"bytes"
	"context"
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
	"google.golang.org/protobuf/proto"
)

func TestGetExecutionPayloadEnvelope(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		envelope, rpcErr := (&Service{}).GetExecutionPayloadEnvelope(t.Context(), nil)
		require.Equal(t, true, envelope == nil)
		requireRPCError(t, rpcErr, BadRequest, "request cannot be nil")
	})

	t.Run("pre-fork", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 10
		params.OverrideBeaconConfig(cfg)

		envelope, rpcErr := (&Service{}).GetExecutionPayloadEnvelope(t.Context(), &ethpb.ExecutionPayloadEnvelopeRequest{Slot: 0})
		require.Equal(t, true, envelope == nil)
		requireRPCError(t, rpcErr, BadRequest, "not supported before Gloas fork")
	})

	t.Run("not found", func(t *testing.T) {
		configureGloasFork(t)
		service := &Service{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}

		envelope, rpcErr := service.GetExecutionPayloadEnvelope(t.Context(), &ethpb.ExecutionPayloadEnvelopeRequest{Slot: 1})
		require.Equal(t, true, envelope == nil)
		requireRPCError(t, rpcErr, NotFound, "not found for slot 1")
	})

	t.Run("success", func(t *testing.T) {
		configureGloasFork(t)
		want := testSignedEnvelope(1).Message
		envelopeCache := cache.NewExecutionPayloadEnvelopeCache()
		envelopeCache.Set(&cache.ExecutionPayloadContents{Envelope: want})
		service := &Service{ExecutionPayloadEnvelopeCache: envelopeCache}

		got, rpcErr := service.GetExecutionPayloadEnvelope(t.Context(), &ethpb.ExecutionPayloadEnvelopeRequest{Slot: 1})
		requireNoRPCError(t, rpcErr)
		wantRoot, err := want.HashTreeRoot()
		require.NoError(t, err)
		gotRoot, err := got.HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, wantRoot, gotRoot)
	})
}

func TestPublishExecutionPayloadEnvelope(t *testing.T) {
	t.Run("validation", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		service := &Service{}

		rpcErr := service.PublishExecutionPayloadEnvelope(t.Context(), nil)
		requireRPCError(t, rpcErr, BadRequest, "must set contents or signed_envelope")

		rpcErr = service.PublishExecutionPayloadEnvelope(t.Context(), &ethpb.GenericSignedExecutionPayloadEnvelope{
			Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
				Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{SignedExecutionPayloadEnvelope: &ethpb.SignedExecutionPayloadEnvelope{}},
			},
		})
		requireRPCError(t, rpcErr, BadRequest, "signed envelope or payload cannot be nil")

		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 10
		params.OverrideBeaconConfig(cfg)
		rpcErr = service.PublishExecutionPayloadEnvelope(t.Context(), envelopeContentsRequest(testSignedEnvelope(0)))
		requireRPCError(t, rpcErr, BadRequest, "not supported before Gloas fork")
	})

	t.Run("stateless contents reject bad proofs", func(t *testing.T) {
		configureGloasFork(t)
		req := statelessEnvelopeRequest(t)
		contents := req.Envelope.(*ethpb.GenericSignedExecutionPayloadEnvelope_Contents).Contents
		contents.KzgProofs[0] = bytes.Repeat([]byte{0xff}, 48)

		rpcErr := (&Service{}).PublishExecutionPayloadEnvelope(t.Context(), req)
		requireRPCError(t, rpcErr, BadRequest, "kzg verification failed")
	})

	t.Run("signed envelope", func(t *testing.T) {
		configureGloasFork(t)
		signed := testSignedEnvelope(1)
		req := &ethpb.GenericSignedExecutionPayloadEnvelope{
			Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_SignedEnvelope{SignedEnvelope: signed},
		}

		t.Run("cache miss", func(t *testing.T) {
			service := &Service{ExecutionPayloadEnvelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
			requireRPCError(t, service.PublishExecutionPayloadEnvelope(t.Context(), req), FailedPrecondition, "no cached blobs and KZG proofs")
		})

		t.Run("cached envelope mismatch", func(t *testing.T) {
			tampered := proto.Clone(signed.Message).(*ethpb.ExecutionPayloadEnvelope)
			tampered.BuilderIndex++
			envelopeCache := cache.NewExecutionPayloadEnvelopeCache()
			envelopeCache.Set(&cache.ExecutionPayloadContents{Envelope: tampered})
			service := &Service{ExecutionPayloadEnvelopeCache: envelopeCache}

			requireRPCError(t, service.PublishExecutionPayloadEnvelope(t.Context(), req), BadRequest, "does not match submitted envelope")
		})

		t.Run("success", func(t *testing.T) {
			broadcaster := &mockp2p.MockBroadcaster{}
			receiver := &mockExecutionPayloadEnvelopeReceiver{done: make(chan struct{})}
			envelopeCache := cache.NewExecutionPayloadEnvelopeCache()
			envelopeCache.Set(&cache.ExecutionPayloadContents{Envelope: signed.Message})
			service := &Service{
				Ctx:                              t.Context(),
				P2P:                              broadcaster,
				ExecutionPayloadEnvelopeCache:    envelopeCache,
				ExecutionPayloadEnvelopeReceiver: receiver,
			}

			requireNoRPCError(t, service.PublishExecutionPayloadEnvelope(t.Context(), req))
			require.Equal(t, true, broadcaster.BroadcastCalled.Load())
			waitForEnvelopeImport(t, receiver)
			require.Equal(t, int32(1), receiver.calls.Load())
		})
	})

	t.Run("success", func(t *testing.T) {
		configureGloasFork(t)
		broadcaster := &mockp2p.MockBroadcaster{}
		receiver := &mockExecutionPayloadEnvelopeReceiver{done: make(chan struct{})}
		service := &Service{
			Ctx:                              t.Context(),
			P2P:                              broadcaster,
			ExecutionPayloadEnvelopeReceiver: receiver,
		}

		requireNoRPCError(t, service.PublishExecutionPayloadEnvelope(t.Context(), envelopeContentsRequest(testSignedEnvelope(1))))
		require.Equal(t, true, broadcaster.BroadcastCalled.Load())
		require.Equal(t, 1, len(broadcaster.BroadcastMessages))
		waitForEnvelopeImport(t, receiver)
		require.Equal(t, int32(1), receiver.calls.Load())
	})

	t.Run("side effect order", func(t *testing.T) {
		configureGloasFork(t)
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
		service := &Service{
			Ctx:                              serviceCtx,
			P2P:                              broadcaster,
			DataColumnReceiver:               dataReceiver,
			ExecutionPayloadEnvelopeReceiver: receiver,
		}
		req := statelessEnvelopeRequest(t)

		result := make(chan *RpcError, 1)
		go func() {
			result <- service.PublishExecutionPayloadEnvelope(requestCtx, req)
		}()

		select {
		case rpcErr := <-result:
			requireNoRPCError(t, rpcErr)
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

		cancelService()
		require.Equal(t, context.Canceled, importCtx.Err())
		close(releaseImport)
		waitForEnvelopeImport(t, receiver)
		require.Equal(t, int32(1), dataReceiver.calls.Load())
	})

	t.Run("envelope broadcast failure", func(t *testing.T) {
		configureGloasFork(t)
		events := make(chan string, 2)
		receiver := &mockExecutionPayloadEnvelopeReceiver{}
		dataReceiver := &mockDataColumnReceiver{}
		broadcaster := &mockPublishEnvelopeBroadcaster{
			MockBroadcaster: &mockp2p.MockBroadcaster{},
			events:          events,
			envelopeErr:     errors.New("envelope broadcast failed"),
		}
		service := &Service{
			Ctx:                              t.Context(),
			P2P:                              broadcaster,
			DataColumnReceiver:               dataReceiver,
			ExecutionPayloadEnvelopeReceiver: receiver,
		}

		requireRPCError(t, service.PublishExecutionPayloadEnvelope(t.Context(), statelessEnvelopeRequest(t)), Internal, "failed to broadcast execution payload envelope")
		require.Equal(t, int32(0), dataReceiver.calls.Load())
		require.Equal(t, int32(0), receiver.calls.Load())
		for _, want := range []string{"broadcast data columns", "broadcast envelope"} {
			require.Equal(t, want, <-events)
		}
	})

	t.Run("import failure does not fail publish", func(t *testing.T) {
		configureGloasFork(t)
		broadcaster := &mockp2p.MockBroadcaster{}
		receiver := &mockExecutionPayloadEnvelopeReceiver{err: errors.New("import failed"), done: make(chan struct{})}
		service := &Service{
			Ctx:                              t.Context(),
			P2P:                              broadcaster,
			ExecutionPayloadEnvelopeReceiver: receiver,
		}

		requireNoRPCError(t, service.PublishExecutionPayloadEnvelope(t.Context(), envelopeContentsRequest(testSignedEnvelope(1))))
		require.Equal(t, true, broadcaster.BroadcastCalled.Load())
		waitForEnvelopeImport(t, receiver)
	})
}

func configureGloasFork(t *testing.T) {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
}

func testSignedEnvelope(slot primitives.Slot) *ethpb.SignedExecutionPayloadEnvelope {
	return &ethpb.SignedExecutionPayloadEnvelope{
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
				SlotNumber:    slot,
			},
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			BeaconBlockRoot:       make([]byte, 32),
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}
}

func envelopeContentsRequest(signed *ethpb.SignedExecutionPayloadEnvelope) *ethpb.GenericSignedExecutionPayloadEnvelope {
	return &ethpb.GenericSignedExecutionPayloadEnvelope{
		Envelope: &ethpb.GenericSignedExecutionPayloadEnvelope_Contents{
			Contents: &ethpb.SignedExecutionPayloadEnvelopeContents{SignedExecutionPayloadEnvelope: signed},
		},
	}
}

func statelessEnvelopeRequest(t *testing.T) *ethpb.GenericSignedExecutionPayloadEnvelope {
	t.Helper()
	require.NoError(t, kzg.Start())

	rawBlobs := []kzg.Blob{{1}}
	_, proofsPerBlob := util.GenerateCellsAndProofs(t, rawBlobs)
	flatProofs := make([][]byte, 0, fieldparams.NumberOfColumns)
	for _, proof := range proofsPerBlob[0] {
		flatProofs = append(flatProofs, proof[:])
	}

	req := envelopeContentsRequest(testSignedEnvelope(1))
	contents := req.Envelope.(*ethpb.GenericSignedExecutionPayloadEnvelope_Contents).Contents
	contents.Blobs = [][]byte{rawBlobs[0][:]}
	contents.KzgProofs = flatProofs
	return req
}

func requireNoRPCError(t *testing.T, rpcErr *RpcError) {
	t.Helper()
	if rpcErr != nil {
		t.Fatal(rpcErr.Err)
	}
}

func requireRPCError(t *testing.T, rpcErr *RpcError, reason ErrorReason, message string) {
	t.Helper()
	require.NotNil(t, rpcErr)
	require.Equal(t, reason, rpcErr.Reason)
	require.ErrorContains(t, message, rpcErr.Err)
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

func waitForEnvelopeImport(t *testing.T, receiver *mockExecutionPayloadEnvelopeReceiver) {
	t.Helper()
	select {
	case <-receiver.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for background envelope import")
	}
}
