package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/async/event"
	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtesting "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	lightclient "github.com/OffchainLabs/prysm/v7/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	light_client "github.com/OffchainLabs/prysm/v7/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"google.golang.org/protobuf/proto"
)

func TestLightClientGloas_Gossip(t *testing.T) {
	setupLightClientNetworkConfig(t)
	p := p2ptest.NewTestP2P(t)
	t.Cleanup(func() { require.NoError(t, p.BHost.Close()) })
	store := lightclient.NewLightClientStore(&p2ptest.FakeP2P{}, new(event.Feed), dbtesting.SetupDB(t))
	s := &Service{
		cfg:     &config{p2p: p, initialSync: &mockSync.Sync{}, clock: startup.NewClock(time.Now().Add(-2*time.Hour), [32]byte{})},
		lcStore: store,
	}
	for _, fork := range []int{version.Fulu, version.Gloas} {
		t.Run(version.String(fork), func(t *testing.T) {
			update := lightClientNetworkUpdate(t, fork)
			optimistic, err := light_client.NewOptimisticUpdateFromUpdate(update)
			require.NoError(t, err)
			finality, err := light_client.NewFinalityUpdateFromUpdate(update)
			require.NoError(t, err)
			store.SetLastOptimisticUpdate(optimistic, false)
			store.SetLastFinalityUpdate(finality, false)
			for _, tt := range []struct {
				topic    string
				message  proto.Message
				validate func(context.Context, peer.ID, *pubsub.Message) (pubsub.ValidationResult, error)
			}{
				{p2p.LightClientOptimisticUpdateTopicFormat, optimistic.Proto(), s.validateLightClientOptimisticUpdate},
				{p2p.LightClientFinalityUpdateTopicFormat, finality.Proto(), s.validateLightClientFinalityUpdate},
			} {
				t.Run(tt.topic, func(t *testing.T) {
					for _, scenario := range []string{"matching", "different", "wrong fork", "truncated"} {
						t.Run(scenario, func(t *testing.T) {
							message := proto.Clone(tt.message)
							if scenario == "different" {
								field := message.ProtoReflect().Descriptor().Fields().ByName("signature_slot")
								message.ProtoReflect().Clear(field)
							}
							var buf bytes.Buffer
							_, err := p.Encoding().EncodeGossip(&buf, message.(ssz.Marshaler))
							require.NoError(t, err)
							epoch := primitives.Epoch(5)
							if fork == version.Gloas {
								epoch = 6
							}
							if scenario == "wrong fork" {
								if fork == version.Gloas {
									epoch = 5
								} else {
									epoch = 6
								}
							}
							topic := fmt.Sprintf(tt.topic, params.ForkDigest(epoch)) + p.Encoding().ProtocolSuffix()
							data := buf.Bytes()
							if scenario == "truncated" {
								data = data[:len(data)/2]
							}
							msg := &pubsub.Message{Message: &pubsubpb.Message{Data: data, Topic: &topic}}
							result, err := tt.validate(t.Context(), "remote", msg)
							switch scenario {
							case "matching":
								require.NoError(t, err)
								require.Equal(t, pubsub.ValidationAccept, result)
								require.Equal(t, true, proto.Equal(tt.message, msg.ValidatorData.(proto.Message)))
								decoded, err := s.decodePubsubMessage(msg)
								require.NoError(t, err)
								decodedProto := decoded.(interface{ Proto() proto.Message }).Proto()
								require.Equal(t, true, proto.Equal(tt.message, decodedProto))
							case "different":
								require.NoError(t, err)
								require.Equal(t, pubsub.ValidationIgnore, result)
								require.Equal(t, nil, msg.ValidatorData)
							default:
								require.NotNil(t, err)
								require.Equal(t, pubsub.ValidationReject, result)
								require.Equal(t, nil, msg.ValidatorData)
							}
						})
					}
				})
			}
		})
	}
}

func TestLightClientGloas_RPC(t *testing.T) {
	setupLightClientNetworkConfig(t)
	local, remote := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
	t.Cleanup(func() {
		require.NoError(t, local.BHost.Close())
		require.NoError(t, remote.BHost.Close())
	})
	local.Connect(remote)
	db := dbtesting.SetupDB(t)
	store := lightclient.NewLightClientStore(&p2ptest.FakeP2P{}, new(event.Feed), db)
	head, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockGloas())
	require.NoError(t, err)
	s := &Service{
		cfg: &config{
			p2p: local, chain: &mockChain.ChainService{Block: head}, beaconDB: db,
			clock: startup.NewClock(time.Now().Add(-2*time.Hour), [32]byte{}),
		},
		lcStore: store, rateLimiter: newRateLimiter(local),
	}
	updates := []interfaces.LightClientUpdate{lightClientNetworkUpdate(t, version.Fulu), lightClientNetworkUpdate(t, version.Gloas)}
	for i, update := range updates {
		epoch := primitives.Epoch(5 + i)
		require.NoError(t, db.SaveLightClientUpdate(t.Context(), uint64(epoch), update))
		t.Run(version.String(update.Version()), func(t *testing.T) {
			var bootstrap interfaces.LightClientBootstrap
			var err error
			switch p := update.Proto().(type) {
			case *ethpb.LightClientUpdateElectra:
				bootstrap, err = light_client.NewWrappedBootstrap(&ethpb.LightClientBootstrapElectra{
					Header: p.AttestedHeader, CurrentSyncCommittee: p.NextSyncCommittee, CurrentSyncCommitteeBranch: p.NextSyncCommitteeBranch,
				})
			case *ethpb.LightClientUpdateGloas:
				bootstrap, err = light_client.NewWrappedBootstrap(&ethpb.LightClientBootstrapGloas{
					Header: p.AttestedHeader, CurrentSyncCommittee: p.NextSyncCommittee, CurrentSyncCommitteeBranch: p.NextSyncCommitteeBranch,
				})
			}
			require.NoError(t, err)
			root, err := bootstrap.Header().Beacon().HashTreeRoot()
			require.NoError(t, err)
			require.NoError(t, db.SaveLightClientBootstrap(t.Context(), root[:], bootstrap))
			optimistic, err := light_client.NewOptimisticUpdateFromUpdate(update)
			require.NoError(t, err)
			finality, err := light_client.NewFinalityUpdateFromUpdate(update)
			require.NoError(t, err)
			store.SetLastOptimisticUpdate(optimistic, false)
			store.SetLastFinalityUpdate(finality, false)
			for _, tt := range []struct {
				topic   string
				handler rpcHandler
				request any
				want    proto.Message
			}{
				{p2p.RPCLightClientBootstrapTopicV1, s.lightClientBootstrapRPCHandler, &root, bootstrap.Proto()},
				{p2p.RPCLightClientOptimisticUpdateTopicV1, s.lightClientOptimisticUpdateRPCHandler, nil, optimistic.Proto()},
				{p2p.RPCLightClientFinalityUpdateTopicV1, s.lightClientFinalityUpdateRPCHandler, nil, finality.Proto()},
			} {
				t.Run(tt.topic, func(t *testing.T) {
					checkLightClientNetworkRPC(t, local, remote, tt.topic, tt.handler, tt.request, []proto.Message{tt.want}, []primitives.Epoch{epoch})
				})
			}
		})
	}
	t.Run("mixed range", func(t *testing.T) {
		checkLightClientNetworkRPC(t, local, remote, p2p.RPCLightClientUpdatesByRangeTopicV1, s.lightClientUpdatesByRangeRPCHandler,
			&ethpb.LightClientUpdatesByRangeRequest{StartPeriod: 5, Count: 2},
			[]proto.Message{updates[0].Proto(), updates[1].Proto()}, []primitives.Epoch{5, 6})
	})
}

func checkLightClientNetworkRPC(t *testing.T, local, remote *p2ptest.TestP2P, topic string, handler rpcHandler, request any, want []proto.Message, epochs []primitives.Epoch) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	id := protocol.ID(topic + local.Encoding().ProtocolSuffix())
	streams := make(chan network.Stream, 1)
	remote.BHost.SetStreamHandler(id, func(stream network.Stream) { streams <- stream })
	out, err := local.BHost.NewStream(ctx, remote.PeerID(), id)
	require.NoError(t, err)
	defer out.Close() // nolint:errcheck
	finished := make(chan error, 1)
	go func() { finished <- handler(ctx, request, out) }()
	var in network.Stream
	select {
	case in = <-streams:
	case <-ctx.Done():
		t.Fatal("timed out receiving RPC stream")
	}
	defer in.Close() // nolint:errcheck
	deadline, ok := ctx.Deadline()
	require.Equal(t, true, ok)
	require.NoError(t, in.SetReadDeadline(deadline))
	for i, expected := range want {
		expectSuccess(t, in)
		digest, err := readContextFromStream(in)
		require.NoError(t, err)
		wantDigest := params.ForkDigest(epochs[i])
		require.DeepEqual(t, wantDigest[:], digest)
		decoded := expected.ProtoReflect().Type().New().Interface()
		require.NoError(t, local.Encoding().DecodeWithMaxLength(in, decoded.(ssz.Unmarshaler)))
		require.Equal(t, true, proto.Equal(expected, decoded))
	}
	remaining, err := io.ReadAll(in)
	require.NoError(t, err)
	require.Equal(t, 0, len(remaining))
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("timed out completing RPC handler")
	}
}

func setupLightClientNetworkConfig(t *testing.T) {
	t.Helper()
	t.Cleanup(types.InitializeDataMaps)
	cfg := params.MainnetTestConfig()
	if fieldparams.Preset == "minimal" {
		cfg = params.MinimalSpecConfig()
	}
	cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch = 0, 1, 2
	cfg.DenebForkEpoch, cfg.ElectraForkEpoch, cfg.FuluForkEpoch = 3, 4, 5
	cfg.GloasForkEpoch, cfg.HezeForkEpoch = 6, 7
	cfg.EpochsPerSyncCommitteePeriod = 1
	params.SetActiveTestCleanup(t, cfg)
	types.InitializeDataMaps()
}

func lightClientNetworkUpdate(t *testing.T, fork int) interfaces.LightClientUpdate {
	t.Helper()
	var p any = util.NewBeaconBlockFulu()
	slot := primitives.Slot(6*fieldparams.SlotsPerEpoch - 1)
	if fork == version.Gloas {
		p = util.NewBeaconBlockGloas()
		slot++
	}
	block, err := blocks.NewSignedBeaconBlock(p)
	require.NoError(t, err)
	block.SetSlot(slot)
	update, err := lightclient.CreateDefaultLightClientUpdate(block)
	require.NoError(t, err)
	update.AttestedHeader().Beacon().BodyRoot[0] = 17
	update.FinalizedHeader().Beacon().Slot = 5 * fieldparams.SlotsPerEpoch
	update.SetSignatureSlot(slot + 1)
	update.SetSyncAggregate(&ethpb.SyncAggregate{
		SyncCommitteeBits:      bytes.Repeat([]byte{255}, fieldparams.SyncAggregateSyncCommitteeBytesLength),
		SyncCommitteeSignature: bytes.Repeat([]byte{42}, 96),
	})
	if fork == version.Gloas {
		update.AttestedHeader().Proto().(*ethpb.LightClientHeaderGloas).ExecutionBlockHash[0] = 42
		update.FinalizedHeader().Proto().(*ethpb.LightClientHeaderGloas).ExecutionBlockHash[0] = 43
	}
	return update
}
