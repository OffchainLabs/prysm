package sync

import (
	"bytes"
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	prysmP2P "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

func init() {
	transition.SkipSlotCache.Disable()
}

func TestRPCHandlerByTopicFromFork_LightClient(t *testing.T) {
	s := &Service{}
	want := map[string]rpcHandler{
		prysmP2P.RPCLightClientBootstrapTopicV1:        s.lightClientBootstrapRPCHandler,
		prysmP2P.RPCLightClientUpdatesByRangeTopicV1:   s.lightClientUpdatesByRangeRPCHandler,
		prysmP2P.RPCLightClientFinalityUpdateTopicV1:   s.lightClientFinalityUpdateRPCHandler,
		prysmP2P.RPCLightClientOptimisticUpdateTopicV1: s.lightClientOptimisticUpdateRPCHandler,
	}
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			t.Cleanup(features.InitWithReset(&features.Flags{EnableLightClient: enabled}))
			for _, fork := range version.AllIncludingUnreleased() {
				t.Run(version.String(fork), func(t *testing.T) {
					handlers, err := s.rpcHandlerByTopicFromFork(fork)
					require.NoError(t, err)
					for topic, handler := range want {
						got, exists := handlers[topic]
						require.Equal(t, enabled && fork >= version.Altair, exists, "topic %s", topic)
						if exists {
							require.Equal(t, reflect.ValueOf(handler).Pointer(), reflect.ValueOf(got).Pointer(), "wrong handler for %s", topic)
						}
					}
					require.NotNil(t, handlers[prysmP2P.RPCPingTopicV1])
				})
			}
			t.Run("invalid fork", func(t *testing.T) {
				handlers, err := s.rpcHandlerByTopicFromFork(-1)
				require.ErrorContains(t, "RPC handler not found", err)
				require.IsNil(t, handlers)
			})
		})
	}
}

func TestRegisterRPCHandlers_LightClient(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			t.Cleanup(features.InitWithReset(&features.Flags{EnableLightClient: enabled}))
			for _, fork := range []int{version.Deneb, version.Electra, version.Fulu, version.Gloas} {
				t.Run(version.String(fork), func(t *testing.T) {
					p := p2ptest.NewTestP2P(t)
					t.Cleanup(func() { require.NoError(t, p.BHost.Close()) })
					s := &Service{ctx: t.Context(), cfg: &config{p2p: p}}
					require.NoError(t, s.registerRPCHandlers(params.NetworkScheduleEntry{VersionEnum: fork}))
					checkLightClientRPCProtocols(t, s, enabled)
				})
			}
		})
	}
}

func TestLightClientRPCHandlers_RetainedAfterFork(t *testing.T) {
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch = 1, 2, 3
	cfg.DenebForkEpoch, cfg.ElectraForkEpoch, cfg.FuluForkEpoch = 4, 8, 12
	cfg.GloasForkEpoch, cfg.HezeForkEpoch = 16, 20
	params.SetActiveTestCleanup(t, cfg)
	t.Cleanup(features.InitWithReset(&features.Flags{EnableLightClient: true}))
	for _, fork := range []int{version.Deneb, version.Gloas} {
		t.Run(version.String(fork), func(t *testing.T) {
			p := p2ptest.NewTestP2P(t)
			t.Cleanup(func() { require.NoError(t, p.BHost.Close()) })
			s := &Service{ctx: t.Context(), cfg: &config{p2p: p}, subHandler: newSubTopicHandler()}
			epoch := params.BeaconConfig().VersionToForkEpochMap()[fork]
			require.Equal(t, fork, params.GetNetworkScheduleEntry(epoch+1).VersionEnum)
			previous := params.GetNetworkScheduleEntry(epoch - 1)
			require.NoError(t, s.registerRPCHandlers(previous))
			checkLightClientRPCProtocols(t, s, true)
			require.NoError(t, s.registerRPCHandlers(params.GetNetworkScheduleEntry(epoch)))
			require.NoError(t, s.ensureDeregistrationForEpoch(epoch+1))
			checkLightClientRPCProtocols(t, s, true)
		})
	}
}

func checkLightClientRPCProtocols(t *testing.T, s *Service, enabled bool) {
	t.Helper()
	registered := make(map[string]bool)
	for _, topic := range s.cfg.p2p.Host().Mux().Protocols() {
		registered[string(topic)] = true
	}
	for _, topic := range []string{
		prysmP2P.RPCLightClientBootstrapTopicV1,
		prysmP2P.RPCLightClientUpdatesByRangeTopicV1,
		prysmP2P.RPCLightClientFinalityUpdateTopicV1,
		prysmP2P.RPCLightClientOptimisticUpdateTopicV1,
	} {
		fullTopic := topic + s.cfg.p2p.Encoding().ProtocolSuffix()
		require.Equal(t, enabled, registered[fullTopic], "topic %s", fullTopic)
	}
	require.Equal(t, true, registered[prysmP2P.RPCPingTopicV1+s.cfg.p2p.Encoding().ProtocolSuffix()])
}

// expectSuccess status code from a stream in regular sync.
func expectSuccess(t *testing.T, stream network.Stream) {
	code, errMsg, err := ReadStatusCode(stream, &encoder.SszNetworkEncoder{})
	require.NoError(t, err)
	require.Equal(t, uint8(0), code, "Received non-zero response code")
	require.Equal(t, "", errMsg, "Received error message from stream")
}

// expectSuccess status code from a stream in regular sync.
func expectFailure(t *testing.T, expectedCode uint8, expectedErrorMsg string, stream network.Stream) {
	code, errMsg, err := ReadStatusCode(stream, &encoder.SszNetworkEncoder{})
	require.NoError(t, err)
	require.NotEqual(t, uint8(0), code, "Expected request to fail but got a 0 response code")
	require.Equal(t, expectedCode, code, "Received incorrect response code")
	require.Equal(t, expectedErrorMsg, errMsg)
}

// expectResetStream status code from a stream in regular sync.
func expectResetStream(t *testing.T, stream network.Stream) {
	expectedErr := "stream reset"
	_, _, err := ReadStatusCode(stream, &encoder.SszNetworkEncoder{})
	require.ErrorContains(t, expectedErr, err)
}

func TestRegisterRPC_ReceivesValidMessage(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	r := &Service{
		ctx:         t.Context(),
		cfg:         &config{p2p: p2p},
		rateLimiter: newRateLimiter(p2p),
	}

	var wg sync.WaitGroup
	wg.Add(1)
	topic := "/testing/foobar/1"
	handler := func(ctx context.Context, msg any, stream libp2pcore.Stream) error {
		m, ok := msg.(*ethpb.Fork)
		if !ok {
			t.Error("Object is not of type *pb.TestSimpleMessage")
		}
		assert.DeepEqual(t, []byte("fooo"), m.CurrentVersion, "Unexpected incoming message")
		wg.Done()

		return nil
	}
	prysmP2P.RPCTopicMappings[topic] = new(ethpb.Fork)
	// Cleanup Topic mappings
	defer func() {
		delete(prysmP2P.RPCTopicMappings, topic)
	}()
	r.registerRPC(topic, handler)

	p2p.ReceiveRPC(topic, &ethpb.Fork{CurrentVersion: []byte("fooo"), PreviousVersion: []byte("barr")})

	if util.WaitTimeout(&wg, time.Second) {
		t.Fatal("Did not receive RPC in 1 second")
	}
}

func TestRPC_ReceivesInvalidMessage(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	remotePeer := p2ptest.NewTestP2P(t)
	remotePeer.Connect(p2p)

	r := &Service{
		ctx:         t.Context(),
		cfg:         &config{p2p: p2p},
		rateLimiter: newRateLimiter(p2p),
	}

	topic := "/testing/foobar/1"
	handler := func(ctx context.Context, msg any, stream libp2pcore.Stream) error {
		m, ok := msg.(*ethpb.Fork)
		if !ok {
			t.Error("Object is not of type *pb.Fork")
		}
		if !bytes.Equal(m.CurrentVersion, []byte("fooo")) {
			t.Errorf("Unexpected incoming message: %+v", m)
		}
		return nil
	}
	prysmP2P.RPCTopicMappings[topic] = new(ethpb.Fork)
	// Cleanup Topic mappings
	defer func() {
		delete(prysmP2P.RPCTopicMappings, topic)
	}()
	r.registerRPC(topic, handler)

	stream, err := remotePeer.Host().NewStream(t.Context(), p2p.BHost.ID(), protocol.ID(topic+p2p.Encoding().ProtocolSuffix()))
	require.NoError(t, err)
	// Write invalid SSZ object to peer.
	_, err = stream.Write([]byte("JUNK MESSAGE"))
	require.NoError(t, err)

	time.Sleep(1 * time.Second)
	faultCount := p2p.PeerScoring().StrikeCount(remotePeer.BHost.ID())

	assert.Equal(t, 1, faultCount, "peer was not penalised for sending bad message")

}
