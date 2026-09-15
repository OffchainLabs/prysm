package sync

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
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
	recentBlockSlot func([32]byte) (primitives.Slot, error)
	receivedRoots   [][32]byte
}

func (c *payloadRecoveryChain) RecentBlockSlot(root [32]byte) (primitives.Slot, error) {
	return c.recentBlockSlot(root)
}

func (c *payloadRecoveryChain) ReceiveExecutionPayloadEnvelope(ctx context.Context, signed interfaces.ROSignedExecutionPayloadEnvelope) error {
	envelope, err := signed.Envelope()
	if err != nil {
		return err
	}
	c.receivedRoots = append(c.receivedRoots, envelope.BeaconBlockRoot())
	return c.ChainService.ReceiveExecutionPayloadEnvelope(ctx, signed)
}

func TestFetchPayloadEnvelope_RangeFallback(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	root := [32]byte{0x42}
	currentSlot := 3 * cfg.SlotsPerEpoch
	ctxMap, err := ContextByteVersionsForValRoot(cfg.GenesisValidatorsRoot)
	require.NoError(t, err)
	rootTopic := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRootTopicV1)
	rangeTopic := fmt.Sprintf("%s/ssz_snappy", p2p.RPCExecutionPayloadEnvelopesByRangeTopicV1)

	tests := []struct {
		name                 string
		rangeResponse        string
		blockSlot            primitives.Slot
		peerCount            int
		wantBadResponses     int
		wantRootRequests     int32
		wantRangeRequests    int32
		firstRootError       bool
		unknownSlot          bool
		cancelBeforeFetch    bool
		cancelBeforeFallback bool
		wantReceived         bool
	}{
		{
			name: "payload predates peer finalization while local finalization lags", peerCount: 1,
			wantRootRequests: 1, wantRangeRequests: 1, wantReceived: true,
		},
		{
			name: "slot immediately before peer finalized epoch", blockSlot: 2*cfg.SlotsPerEpoch - 1, peerCount: 1,
			wantRootRequests: 1, wantRangeRequests: 1, wantReceived: true,
		},
		{
			name: "peer finalized epoch boundary skips fallback", blockSlot: 2 * cfg.SlotsPerEpoch, peerCount: 3,
			wantRootRequests: 3,
		},
		{
			name: "withheld current head skips fallback", blockSlot: currentSlot, peerCount: 3,
			wantRootRequests: 3,
		},
		{
			name: "recent parent skips fallback", blockSlot: currentSlot - 1, peerCount: 3,
			wantRootRequests: 3,
		},
		{
			name: "alternate fork is skipped without penalty", peerCount: 2, rangeResponse: "alternate fork",
			wantRootRequests: 2, wantRangeRequests: 2, wantReceived: true,
		},
		{
			name: "wrong range slot is rejected and penalized", peerCount: 2, rangeResponse: "wrong slot",
			wantRootRequests: 2, wantRangeRequests: 2, wantBadResponses: 1, wantReceived: true,
		},
		{
			name: "excess range count is rejected and penalized", peerCount: 2, rangeResponse: "excess count",
			wantRootRequests: 2, wantRangeRequests: 2, wantBadResponses: 1, wantReceived: true,
		},
		{
			name: "root error skips fallback and tries next peer", peerCount: 2, firstRootError: true,
			wantRootRequests: 2, wantRangeRequests: 1, wantReceived: true,
		},
		{
			name: "range error tries next peer", peerCount: 2, rangeResponse: "error",
			wantRootRequests: 2, wantRangeRequests: 2, wantReceived: true,
		},
		{
			name: "unknown block slot skips fallback", peerCount: 2, unknownSlot: true,
			wantRootRequests: 2,
		},
		{
			name: "empty responses are bounded to three peers and six requests", peerCount: 4, rangeResponse: "empty",
			wantRootRequests: 3, wantRangeRequests: 3,
		},
		{
			name: "cancellation before fetch", peerCount: 1, cancelBeforeFetch: true,
		},
		{
			name: "cancellation before fallback", peerCount: 2, cancelBeforeFallback: true,
			wantRootRequests: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockSlot := tt.blockSlot
			if blockSlot == 0 {
				blockSlot = cfg.SlotsPerEpoch - 1
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			p1 := p2ptest.NewTestP2P(t)
			t.Cleanup(func() { require.NoError(t, p1.BHost.Close()) })
			var rootRequests, rangeRequests atomic.Int32
			var invalidRangePeer atomic.Int32
			invalidRangePeer.Store(-1)
			rootCalls := make([]atomic.Int32, tt.peerCount)
			rangeCalls := make([]atomic.Int32, tt.peerCount)
			remotePeers := make([]*p2ptest.TestP2P, tt.peerCount)
			for i := range tt.peerCount {
				p2 := p2ptest.NewTestP2P(t)
				t.Cleanup(func() { require.NoError(t, p2.BHost.Close()) })
				remotePeers[i] = p2
				p1.Connect(p2)
				p1.Peers().SetConnectionState(p2.PeerID(), peers.Connected)
				p1.Peers().SetChainState(p2.PeerID(), &ethpb.StatusV2{FinalizedEpoch: 2, HeadSlot: currentSlot})
				p2.SetStreamHandler(rootTopic, func(stream network.Stream) {
					defer func() { _ = stream.Close() }()
					req := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
					if err := p2.Encoding().DecodeWithMaxLength(stream, req); err != nil {
						assert.NoError(t, err)
						return
					}
					assert.Equal(t, true, slices.Equal(p2ptypes.ExecutionPayloadEnvelopesByRootReq{root}, *req), "unexpected requested roots: %v", *req)
					rootCalls[i].Add(1)
					if rootRequests.Add(1) == 1 && tt.firstRootError {
						writeErrorResponseToStream(responseCodeServerError, "temporary failure", stream, p2)
					}
				})
				p2.SetStreamHandler(rangeTopic, func(stream network.Stream) {
					defer func() { _ = stream.Close() }()
					req := new(ethpb.ExecutionPayloadEnvelopesByRangeRequest)
					if err := p2.Encoding().DecodeWithMaxLength(stream, req); err != nil {
						assert.NoError(t, err)
						return
					}
					assert.Equal(t, blockSlot, req.StartSlot)
					assert.Equal(t, uint64(1), req.Count)
					assert.Equal(t, int32(1), rootCalls[i].Load())
					rangeCalls[i].Add(1)
					attempt := rangeRequests.Add(1)
					if tt.rangeResponse == "empty" {
						return
					}
					envelope := testSignedEnvelope(blockSlot, root[:])
					if attempt == 1 {
						switch tt.rangeResponse {
						case "alternate fork":
							envelope.Message.BeaconBlockRoot[0]++
						case "wrong slot":
							invalidRangePeer.Store(int32(i))
							envelope.Message.Payload.SlotNumber++
						case "excess count":
							invalidRangePeer.Store(int32(i))
							assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2.Encoding(), envelope))
						case "error":
							writeErrorResponseToStream(responseCodeServerError, "temporary failure", stream, p2)
							return
						}
					}
					assert.NoError(t, WriteExecutionPayloadEnvelopeChunk(stream, p2.Encoding(), envelope))
				})
			}
			chain := &payloadRecoveryChain{
				ChainService: &mock.ChainService{FinalizedCheckPoint: &ethpb.Checkpoint{}},
				recentBlockSlot: func(got [32]byte) (primitives.Slot, error) {
					require.Equal(t, root, got)
					if tt.cancelBeforeFallback {
						cancel()
					}
					if tt.unknownSlot {
						return 0, fmt.Errorf("unknown block")
					}
					return blockSlot, nil
				},
			}
			s := &Service{
				ctx: ctx,
				cfg: &config{
					chain: chain,
					p2p:   p1,
					clock: startup.NewClock(time.Now().Add(-time.Duration(currentSlot)*cfg.SlotDuration()), cfg.GenesisValidatorsRoot),
				},
				ctxMap:          ctxMap,
				badPayloadCache: lruwrpr.New(10),
			}
			if tt.cancelBeforeFetch {
				cancel()
			}

			s.fetchPayloadEnvelope(root)

			require.Equal(t, tt.wantRootRequests, rootRequests.Load())
			require.Equal(t, tt.wantRangeRequests, rangeRequests.Load())
			if tt.wantReceived {
				require.Equal(t, [][32]byte{root}, chain.receivedRoots)
				require.True(t, chain.ReceivePayloadEnvelopeCtxHadDeadline)
			} else {
				require.Empty(t, chain.receivedRoots)
			}
			badResponses := 0
			for i, p2 := range remotePeers {
				require.LessOrEqual(t, rootCalls[i].Load(), int32(1))
				require.LessOrEqual(t, rangeCalls[i].Load(), int32(1))
				count, err := p1.Peers().Scorers().BadResponsesScorer().Count(p2.PeerID())
				require.NoError(t, err)
				wantPeerBadResponses := 0
				if int32(i) == invalidRangePeer.Load() {
					wantPeerBadResponses = tt.wantBadResponses
				}
				require.Equal(t, wantPeerBadResponses, count)
				badResponses += count
			}
			require.Equal(t, tt.wantBadResponses, badResponses)
			require.False(t, s.hasBadPayload(root))
		})
	}
}
