package sync

import (
	"context"
	"fmt"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/wrapper"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
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

	s := &Service{badPayloadCache: lruwrpr.New(10)}

	blk := util.NewBeaconBlockGloas()
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	res, err := s.validateExecutionPayloadBidParentValid(ctx, wsb.Block())
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)
}

func TestValidateExecutionPayloadBidParentValid_RejectWhenBuildingOnInvalidPayload(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	s := &Service{
		badPayloadCache: lruwrpr.New(10),
		cfg:             &config{chain: &mock.ChainService{BuiltOnFullParentVal: true}},
	}

	blk := util.NewBeaconBlockGloas()
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	parentRoot := wsb.Block().ParentRoot()
	s.badPayloadCache.Add(string(parentRoot[:]), true)

	res, err := s.validateExecutionPayloadBidParentValid(ctx, wsb.Block())
	require.Error(t, err)
	require.Equal(t, pubsub.ValidationReject, res)
}

func TestValidateExecutionPayloadBidParentValid_AcceptWhenBuildingOnEmptyParent(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()

	s := &Service{
		badPayloadCache: lruwrpr.New(10),
		cfg:             &config{chain: &mock.ChainService{BuiltOnFullParentVal: false}},
	}

	blk := util.NewBeaconBlockGloas()
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	parentRoot := wsb.Block().ParentRoot()
	s.badPayloadCache.Add(string(parentRoot[:]), true)

	res, err := s.validateExecutionPayloadBidParentValid(ctx, wsb.Block())
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res)
}

func TestRequestPayloadEnvelope_SkipsWhenAlreadyResolved(t *testing.T) {
	root := [32]byte{0x42}

	tests := []struct {
		name  string
		setup func(*Service)
	}{
		{
			name: "already have full node",
			setup: func(s *Service) {
				s.cfg.chain = &mock.ChainService{ForkchoiceRoots: map[[32]byte]bool{root: true}}
			},
		},
		{
			name: "payload marked bad",
			setup: func(s *Service) {
				s.cfg.chain = &mock.ChainService{}
				s.badPayloadCache.Add(string(root[:]), true)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// p2p is nil — getBestPeers would panic if the guards don't short-circuit.
			s := &Service{
				cfg:             &config{},
				badPayloadCache: lruwrpr.New(10),
			}
			tt.setup(s)
			require.NotPanics(t, func() { s.requestPayloadEnvelope(root) })
		})
	}
}

func TestFetchPayloadEnvelope_ReceiveHasDeadline(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	ctxMap, err := ContextByteVersionsForValRoot(params.BeaconConfig().GenesisValidatorsRoot)
	require.NoError(t, err)

	p1, p2 := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	p1.Peers().SetConnectionState(p2.PeerID(), peers.Connected)
	p1.Peers().SetChainState(p2.PeerID(), &ethpb.StatusV2{})

	root := [32]byte{0x42}
	envelope := &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    make([]byte, fieldparams.RootLength),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, fieldparams.RootLength),
				ReceiptsRoot:  make([]byte, fieldparams.RootLength),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, fieldparams.RootLength),
				BaseFeePerGas: make([]byte, fieldparams.RootLength),
				BlockHash:     make([]byte, fieldparams.RootLength),
				SlotNumber:    1,
			},
			BeaconBlockRoot:       root[:],
			ParentBeaconBlockRoot: make([]byte, fieldparams.RootLength),
		},
		Signature: make([]byte, fieldparams.BLSSignatureLength),
	}

	protocol := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1)
	p2.SetStreamHandler(protocol, func(stream network.Stream) {
		req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
		require.NoError(t, p2.Encoding().DecodeWithMaxLength(stream, req))
		require.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2.Encoding(), envelope))
		require.NoError(t, stream.CloseWrite())
	})

	chain := &mock.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{}}
	s := &Service{
		ctx: context.Background(),
		cfg: &config{
			chain: chain,
			p2p:   p1,
			clock: startup.NewClock(time.Now(), [fieldparams.RootLength]byte{}),
		},
		ctxMap:          ctxMap,
		badPayloadCache: lruwrpr.New(10),
	}

	s.fetchPayloadEnvelope(root)
	require.True(t, chain.ReceivePayloadEnvelopeCtxHadDeadline)
}

type payloadRecoveryChain struct {
	*mock.ChainService
	receive func(context.Context, interfaces.ROSignedExecutionPayloadEnvelope) error
}

func (*payloadRecoveryChain) HasNode([32]byte) bool     { return true }
func (*payloadRecoveryChain) HasFullNode([32]byte) bool { return false }
func (c *payloadRecoveryChain) ReceiveExecutionPayloadEnvelope(ctx context.Context, envelope interfaces.ROSignedExecutionPayloadEnvelope) error {
	return c.receive(ctx, envelope)
}

type payloadRecoveryDB struct {
	db.NoHeadAccessDatabase
	deadlines chan time.Time
}

func (d *payloadRecoveryDB) Block(ctx context.Context, root [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	deadline, _ := ctx.Deadline()
	d.deadlines <- deadline
	return d.NoHeadAccessDatabase.Block(ctx, root)
}

type payloadRecoveryColumnRequest struct {
	stream      network.Stream
	identifiers p2ptypes.DataColumnsByRootIdentifiers
}

type payloadRecoveryImport struct {
	err      error
	deadline time.Time
	columns  uint64
}

type payloadRecoveryTest struct {
	service          *Service
	cancel           context.CancelFunc
	root             [32]byte
	remote           *p2ptest.TestP2P
	envelope         *ethpb.SignedExecutionPayloadEnvelope
	envelopeRequests chan network.Stream
	columnRequests   chan payloadRecoveryColumnRequest
	imports          chan payloadRecoveryImport
	fetchDeadlines   chan time.Time
}

func newPayloadRecoveryTest(t *testing.T, withBlobs bool) *payloadRecoveryTest {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	cfg.SlotDurationMilliseconds = 1000
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()
	ctxMap, err := ContextByteVersionsForValRoot(cfg.GenesisValidatorsRoot)
	require.NoError(t, err)

	privateKeyBytes := [32]byte{1}
	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKeyBytes[:])
	require.NoError(t, err)
	local, remote := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t, libp2p.Identity(privateKey))
	t.Cleanup(func() {
		require.NoError(t, local.BHost.Close())
		require.NoError(t, remote.BHost.Close())
	})
	local.Connect(remote)
	local.Peers().SetConnectionState(remote.PeerID(), peers.Connected)
	local.Peers().SetChainState(remote.PeerID(), &ethpb.StatusV2{HeadSlot: 128})
	local.Peers().SetMetadata(remote.PeerID(), wrapper.WrappedMetadataV2(&ethpb.MetaDataV2{CustodyGroupCount: 128}))

	block := util.NewBeaconBlockGloas()
	block.Block.Slot = 1
	block.Block.Body.SignedExecutionPayloadBid.Message.BlobKzgCommitments = nil
	if withBlobs {
		block.Block.Body.SignedExecutionPayloadBid.Message.BlobKzgCommitments = [][]byte{make([]byte, 48)}
	}
	signed, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	ro, err := blocks.NewROBlock(signed)
	require.NoError(t, err)
	database := testDB.SetupDB(t)
	require.NoError(t, database.SaveBlock(t.Context(), signed))
	storage := filesystem.NewEphemeralDataColumnStorage(t)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	h := &payloadRecoveryTest{
		cancel: cancel, root: ro.Root(), remote: remote,
		envelopeRequests: make(chan network.Stream, 4),
		columnRequests:   make(chan payloadRecoveryColumnRequest, 4),
		imports:          make(chan payloadRecoveryImport, 4),
		fetchDeadlines:   make(chan time.Time, 4),
	}
	chain := &payloadRecoveryChain{ChainService: &mock.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{}}}
	chain.receive = func(ctx context.Context, _ interfaces.ROSignedExecutionPayloadEnvelope) error {
		deadline, _ := ctx.Deadline()
		h.imports <- payloadRecoveryImport{err: ctx.Err(), deadline: deadline, columns: storage.Summary(h.root).Count()}
		return nil
	}
	h.service = &Service{
		ctx: ctx,
		cfg: &config{
			chain: chain, p2p: local,
			clock:             startup.NewClock(time.Now().Add(-128*cfg.SlotDuration()), [32]byte{}),
			beaconDB:          &payloadRecoveryDB{NoHeadAccessDatabase: database, deadlines: h.fetchDeadlines},
			dataColumnStorage: storage,
		},
		ctxMap:          ctxMap,
		badPayloadCache: lruwrpr.New(10),
		newColumnsVerifier: func(columns []blocks.RODataColumn, _ []verification.Requirement) verification.DataColumnsVerifier {
			verifier := &verification.MockDataColumnsVerifier{}
			verifier.AppendRODataColumns(columns...)
			return verifier
		},
	}
	h.envelope = &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			BeaconBlockRoot: h.root[:], ParentBeaconBlockRoot: make([]byte, 32),
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash: make([]byte, 32), FeeRecipient: make([]byte, 20),
				StateRoot: make([]byte, 32), ReceiptsRoot: make([]byte, 32),
				LogsBloom: make([]byte, 256), PrevRandao: make([]byte, 32),
				BaseFeePerGas: make([]byte, 32), BlockHash: make([]byte, 32), SlotNumber: 1,
			},
		},
		Signature: make([]byte, 96),
	}
	remote.SetStreamHandler(fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1), func(stream network.Stream) {
		request := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
		if err := remote.Encoding().DecodeWithMaxLength(stream, request); err != nil {
			t.Errorf("decode envelope request: %v", err)
			return
		}
		select {
		case h.envelopeRequests <- stream:
		case <-ctx.Done():
			_ = stream.Reset()
		}
	})
	remote.SetStreamHandler(fmt.Sprintf("%s/ssz_snappy", p2p.RPCDataColumnSidecarsByRootTopicV1), func(stream network.Stream) {
		request := new(p2ptypes.DataColumnsByRootIdentifiers)
		if err := remote.Encoding().DecodeWithMaxLength(stream, request); err != nil {
			t.Errorf("decode column request: %v", err)
			return
		}
		select {
		case h.columnRequests <- payloadRecoveryColumnRequest{stream: stream, identifiers: *request}:
		case <-ctx.Done():
			_ = stream.Reset()
		}
	})
	return h
}

func waitPayloadRecoveryEvent[T any](t *testing.T, events <-chan T) T {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for payload recovery")
		var zero T
		return zero
	}
}

func (h *payloadRecoveryTest) start() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.service.requestPayloadEnvelope(h.root)
	}()
	return done
}

func (h *payloadRecoveryTest) serveEnvelope(t *testing.T, stream network.Stream) {
	t.Helper()
	require.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, h.remote.Encoding(), h.envelope))
	require.NoError(t, stream.CloseWrite())
}

func (h *payloadRecoveryTest) serveColumns(t *testing.T, request payloadRecoveryColumnRequest) uint64 {
	t.Helper()
	var count uint64
	for _, identifier := range request.identifiers {
		require.Equal(t, h.root[:], identifier.BlockRoot)
		for _, index := range identifier.Columns {
			column, err := blocks.NewRODataColumnGloas(&ethpb.DataColumnSidecarGloas{
				Index: index, Slot: 1, BeaconBlockRoot: h.root[:],
				Column: [][]byte{make([]byte, 2048)}, KzgProofs: [][]byte{make([]byte, 48)},
			})
			require.NoError(t, err)
			require.NoError(t, WriteDataColumnSidecarChunk(request.stream, h.service.cfg.clock, h.remote.Encoding(), column))
			count++
		}
	}
	require.NoError(t, request.stream.CloseWrite())
	return count
}

func TestFetchPayloadEnvelope_RecoveryLifecycle(t *testing.T) {
	t.Run("downloads overlap and columns are saved before a fresh import deadline", func(t *testing.T) {
		h := newPayloadRecoveryTest(t, true)
		done := h.start()
		// Both requests must reach the peer while neither response has been released.
		envelope := waitPayloadRecoveryEvent(t, h.envelopeRequests)
		columns := waitPayloadRecoveryEvent(t, h.columnRequests)
		fetchDeadline := waitPayloadRecoveryEvent(t, h.fetchDeadlines)
		h.serveEnvelope(t, envelope)
		count := h.serveColumns(t, columns)
		imported := waitPayloadRecoveryEvent(t, h.imports)
		waitPayloadRecoveryEvent(t, done)
		require.Positive(t, count)
		require.Equal(t, count, imported.columns)
		require.NoError(t, imported.err)
		require.False(t, fetchDeadline.IsZero())
		require.True(t, imported.deadline.After(fetchDeadline), "import must receive its own deadline after downloads finish")
	})

	t.Run("shutdown cancels outstanding downloads without importing", func(t *testing.T) {
		h := newPayloadRecoveryTest(t, true)
		done := h.start()
		envelope := waitPayloadRecoveryEvent(t, h.envelopeRequests)
		waitPayloadRecoveryEvent(t, h.columnRequests)
		h.serveEnvelope(t, envelope)
		h.cancel()
		waitPayloadRecoveryEvent(t, done)
		require.Empty(t, h.imports)
	})

	t.Run("timed out columns release the same root for a new request", func(t *testing.T) {
		h := newPayloadRecoveryTest(t, true)
		done := h.start()
		envelope := waitPayloadRecoveryEvent(t, h.envelopeRequests)
		waitPayloadRecoveryEvent(t, h.columnRequests)
		h.serveEnvelope(t, envelope)
		waitPayloadRecoveryEvent(t, done)
		require.Empty(t, h.imports)

		done = h.start()
		envelope = waitPayloadRecoveryEvent(t, h.envelopeRequests)
		columns := waitPayloadRecoveryEvent(t, h.columnRequests)
		h.serveEnvelope(t, envelope)
		count := h.serveColumns(t, columns)
		imported := waitPayloadRecoveryEvent(t, h.imports)
		waitPayloadRecoveryEvent(t, done)
		require.Equal(t, count, imported.columns)
		require.NoError(t, imported.err)
	})
	t.Run("timed out import releases the same root and reuses stored columns", func(t *testing.T) {
		h := newPayloadRecoveryTest(t, true)
		chain := h.service.cfg.chain.(*payloadRecoveryChain)
		receive := chain.receive
		importStarted := make(chan struct{})
		importResult := make(chan error, 1)
		chain.receive = func(ctx context.Context, _ interfaces.ROSignedExecutionPayloadEnvelope) error {
			close(importStarted)
			<-ctx.Done()
			importResult <- ctx.Err()
			return ctx.Err()
		}
		done := h.start()
		envelope := waitPayloadRecoveryEvent(t, h.envelopeRequests)
		columns := waitPayloadRecoveryEvent(t, h.columnRequests)
		h.serveEnvelope(t, envelope)
		count := h.serveColumns(t, columns)
		waitPayloadRecoveryEvent(t, importStarted)
		waitPayloadRecoveryEvent(t, done)
		require.ErrorIs(t, waitPayloadRecoveryEvent(t, importResult), context.DeadlineExceeded)

		chain.receive = receive
		done = h.start()
		envelope = waitPayloadRecoveryEvent(t, h.envelopeRequests)
		h.serveEnvelope(t, envelope)
		imported := waitPayloadRecoveryEvent(t, h.imports)
		waitPayloadRecoveryEvent(t, done)
		require.NoError(t, imported.err)
		require.Equal(t, count, imported.columns)
		require.Empty(t, h.columnRequests)
	})
}

func TestFetchPayloadEnvelope_NoRequiredColumns(t *testing.T) {
	for _, tc := range []struct {
		name             string
		withBlobs        bool
		outsideRetention bool
	}{
		{name: "payload has no blobs"},
		{name: "payload is outside column retention", withBlobs: true, outsideRetention: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newPayloadRecoveryTest(t, tc.withBlobs)
			if tc.outsideRetention {
				cfg := params.BeaconConfig()
				elapsed := time.Duration(uint64(cfg.MinEpochsForDataColumnSidecarsRequest+1)*uint64(cfg.SlotsPerEpoch)) * cfg.SlotDuration()
				h.service.cfg.clock = startup.NewClock(time.Now().Add(-elapsed), [32]byte{})
			}
			done := h.start()
			envelope := waitPayloadRecoveryEvent(t, h.envelopeRequests)
			h.serveEnvelope(t, envelope)
			imported := waitPayloadRecoveryEvent(t, h.imports)
			waitPayloadRecoveryEvent(t, done)
			require.NoError(t, imported.err)
			require.Zero(t, imported.columns)
			require.Empty(t, h.columnRequests)
		})
	}
}
