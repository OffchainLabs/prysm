package initialsync

import (
	"sync/atomic"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	prysmsync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestService_ParentPayloadBeforeImport(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BlobSchedule = []params.BlobScheduleEntry{{Epoch: 0, MaxBlobsPerBlock: 10}}
	params.SetGenesisFork(t, cfg, version.Gloas)
	params.BeaconConfig().InitializeForkSchedule()
	require.NoError(t, kzg.Start())

	const parentSlot = primitives.Slot(32)
	parentHash, ancestorHash := [32]byte{0x91}, [32]byte{0x90}
	blob := kzg.Blob{}
	commitment, err := kzg.BlobToKZGCommitment(&blob)
	require.NoError(t, err)
	parentProto := util.NewBeaconBlockGloas()
	parentProto.Block.Slot = parentSlot
	bid := parentProto.Block.Body.SignedExecutionPayloadBid.Message
	bid.ParentBlockHash, bid.BlockHash = ancestorHash[:], parentHash[:]
	bid.BlobKzgCommitments = [][]byte{commitment[:]}
	signed, err := blocks.NewSignedBeaconBlock(parentProto)
	require.NoError(t, err)
	parent, err := blocks.NewROBlock(signed)
	require.NoError(t, err)
	parentRoot := parent.Root()
	envelope := makeEnvelopeForRoot(t, parentSlot, parentRoot, parentHash, ancestorHash)
	cells, proofs := util.GenerateCellsAndProofs(t, []kzg.Blob{blob})
	columns, err := peerdas.DataColumnSidecars(cells, proofs, peerdas.PopulateFromBlock(parent))
	require.NoError(t, err)
	clock := startup.NewClock(makeGenesisTime(parentSlot*2), cfg.GenesisValidatorsRoot)
	ctxMap, err := prysmsync.ContextByteVersionsForValRoot(cfg.GenesisValidatorsRoot)
	require.NoError(t, err)
	gs := startup.NewClockSynchronizer()
	require.NoError(t, gs.SetClock(clock))
	initializer, err := verification.NewInitializerWaiter(gs, nil, nil, nil).WaitForInitializer(t.Context())
	require.NoError(t, err)
	newVerifier := newDataColumnsVerifierFromInitializer(initializer)

	tests := []struct {
		name            string
		supplied        bool
		empty           bool
		imported        bool
		headOnly        bool
		missingPayload  bool
		missingColumns  bool
		payloadRequests int32
		wantErr         string
	}{
		{name: "fetch missing parent outside range", payloadRequests: 1},
		{name: "parent available only from head", headOnly: true, payloadRequests: 1},
		{name: "supplied parent skipped by prefetch", supplied: true},
		{name: "empty parent", empty: true},
		{name: "imported parent", imported: true},
		{name: "unavailable payload", missingPayload: true, payloadRequests: 1, wantErr: "required parent execution payload envelope"},
		{name: "unavailable columns", supplied: true, missingColumns: true, wantErr: "data columns unavailable for parent"},
	}
	for _, mode := range []string{"regular", "batch"} {
		for _, tt := range tests {
			t.Run(mode+"/"+tt.name, func(t *testing.T) {
				ctx := t.Context()
				beaconDB := dbtest.SetupDB(t)
				storage := filesystem.NewEphemeralDataColumnStorage(t)
				key, _, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
				require.NoError(t, err)
				client, server := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t, libp2p.Identity(key))
				client.Connect(server)
				client.Peers().SetConnectionState(server.PeerID(), peers.Connected)
				client.Peers().SetChainState(server.PeerID(), &ethpb.StatusV2{HeadSlot: parentSlot * 2})
				server.ENR().Set(peerdas.Cgc(cfg.NumberOfCustodyGroups))
				client.Peers().UpdateENR(server.ENR(), server.PeerID())
				var payloadRequests, columnRequests atomic.Int32
				server.SetStreamHandler(p2p.RPCExecutionPayloadEnvelopesByRootTopicV1+"/ssz_snappy", func(stream network.Stream) {
					defer stream.Close() // nolint:errcheck
					request := new(p2ptypes.ExecutionPayloadEnvelopesByRootReq)
					assert.NoError(t, server.Encoding().DecodeWithMaxLength(stream, request))
					assert.DeepEqual(t, p2ptypes.ExecutionPayloadEnvelopesByRootReq{parentRoot}, *request)
					payloadRequests.Add(1)
					if !tt.missingPayload {
						assert.NoError(t, prysmsync.WriteExecutionPayloadEnvelopeChunk(stream, server.Encoding(), envelope.Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
					}
					assert.NoError(t, stream.CloseWrite())
				})
				server.SetStreamHandler(p2p.RPCDataColumnSidecarsByRootTopicV1+"/ssz_snappy", func(stream network.Stream) {
					defer stream.Close() // nolint:errcheck
					request := new(p2ptypes.DataColumnsByRootIdentifiers)
					assert.NoError(t, server.Encoding().DecodeWithMaxLength(stream, request))
					columnRequests.Add(1)
					assert.Equal(t, 1, len(*request))
					for _, item := range *request {
						assert.DeepEqual(t, parentRoot[:], item.BlockRoot)
						if !tt.missingColumns {
							for _, index := range item.Columns {
								assert.NoError(t, prysmsync.WriteDataColumnSidecarChunk(stream, clock, server.Encoding(), columns[index]))
							}
						}
					}
					assert.NoError(t, stream.CloseWrite())
				})
				childParentHash := parentHash
				if tt.empty {
					childParentHash = ancestorHash
				}
				child := makeGloasBlock(t, parentSlot.Add(3), parentRoot, childParentHash)
				st, err := util.NewBeaconStateGloas()
				require.NoError(t, err)
				require.NoError(t, st.SetSlot(parentSlot.Sub(1)))
				chain := &originColumnsChain{ChainService: &mock.ChainService{
					FinalizedCheckPoint: &ethpb.Checkpoint{}, DB: beaconDB, State: st,
					ForkchoiceRoots: map[[32]byte]bool{parentRoot: tt.imported},
				}}
				custodyGroupCount, err := client.CustodyGroupCount(ctx)
				require.NoError(t, err)
				info, _, err := peerdas.Info(client.NodeID(), max(custodyGroupCount, cfg.SamplesPerSlot))
				require.NoError(t, err)
				assertParent := func(envs []interfaces.ROSignedExecutionPayloadEnvelope) {
					require.Equal(t, 0, len(chain.BlocksReceived))
					if tt.empty || tt.imported {
						require.Equal(t, 0, len(envs))
						return
					}
					require.Equal(t, 1, len(envs))
					require.DeepEqual(t, envelope.Proto(), envs[0].Proto())
					for index := range info.CustodyColumns {
						require.Equal(t, true, storage.Summary(parentRoot).HasIndex(index))
					}
				}
				chain.onEnvelope = func(env interfaces.ROSignedExecutionPayloadEnvelope) {
					assertParent([]interfaces.ROSignedExecutionPayloadEnvelope{env})
				}
				chain.onBatch = func(blks []blocks.ROBlock, envs []interfaces.ROSignedExecutionPayloadEnvelope) {
					require.Equal(t, 1, len(blks))
					require.Equal(t, child.Root(), blks[0].Root())
					assertParent(envs)
				}
				service := NewService(ctx, &Config{Chain: chain, DB: beaconDB, P2P: client, DataColumnStorage: storage})
				t.Cleanup(service.cancel)
				service.clock, service.genesisTime = clock, clock.GenesisTime()
				service.ctxMap, service.newDataColumnsVerifier = ctxMap, newVerifier
				response := &fetchRequestResponse{blocksFrom: server.PeerID(), bwb: []blocks.BlockWithROSidecars{{Block: child}}}
				if tt.supplied {
					response.envelopes = []interfaces.ROSignedExecutionPayloadEnvelope{envelope}
				}
				fetcher := &blocksFetcher{clock: clock, p2p: client, db: beaconDB, chain: chain, dcs: storage}
				fetcher.fetchSidecars(ctx, response, []peer.ID{server.PeerID()})
				require.NoError(t, response.err)
				require.Equal(t, 0, len(response.columnsToSave))
				require.Equal(t, int32(0), columnRequests.Load())
				if tt.headOnly {
					chain.Block = parent
					chain.InitSyncBlockRoots = map[[32]byte]bool{parentRoot: true}
				} else {
					require.NoError(t, beaconDB.SaveBlock(ctx, parent))
				}
				require.NoError(t, st.SetSlot(parentSlot))
				chain.Root = parentRoot[:]
				if tt.imported {
					require.NoError(t, beaconDB.SaveExecutionPayloadEnvelope(ctx, envelope.Proto().(*ethpb.SignedExecutionPayloadEnvelope)))
				}
				var processed uint64
				if mode == "regular" {
					processed, err = service.processFetchedDataRegSync(ctx, response.blocksQueueFetchedData())
				} else {
					processed, err = service.processBatchedBlocks(ctx, response.bwb, response.envelopes, chain.ReceiveBlockBatch, server.PeerID())
				}
				if tt.wantErr != "" {
					require.ErrorContains(t, tt.wantErr, err)
					require.Equal(t, uint64(0), processed)
					require.Equal(t, 0, len(chain.BlocksReceived))
				} else {
					require.NoError(t, err)
					require.Equal(t, uint64(1), processed)
					require.Equal(t, 1, len(chain.BlocksReceived))
				}
				require.Equal(t, tt.payloadRequests, payloadRequests.Load())
				wantColumns := !tt.empty && !tt.imported && !tt.missingPayload
				require.Equal(t, wantColumns, columnRequests.Load() > 0)
			})
		}
	}
}
