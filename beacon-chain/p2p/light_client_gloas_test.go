package p2p

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	light_client "github.com/OffchainLabs/prysm/v7/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"google.golang.org/protobuf/proto"
)

func TestBroadcastLightClientGloas(t *testing.T) {
	cfg := params.MainnetTestConfig()
	if fieldparams.Preset == "minimal" {
		cfg = params.MinimalSpecConfig()
	}
	cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch = 0, 1, 2
	cfg.DenebForkEpoch, cfg.ElectraForkEpoch, cfg.FuluForkEpoch, cfg.GloasForkEpoch = 3, 4, 5, 6
	params.SetActiveTestCleanup(t, cfg)
	local, remote := p2ptest.NewTestP2P(t), p2ptest.NewTestP2P(t)
	t.Cleanup(func() {
		require.NoError(t, local.BHost.Close())
		require.NoError(t, remote.BHost.Close())
	})
	local.Connect(remote)
	s := &Service{
		host: local.BHost, pubsub: local.PubSub(), joinedTopics: make(map[string]*pubsub.Topic), cfg: &Config{},
		genesisTime: time.Now().Add(-2 * time.Hour),
	}
	for _, tt := range []struct {
		name  string
		slot  primitives.Slot
		epoch primitives.Epoch
	}{
		{"fulu signed in gloas", 6*fieldparams.SlotsPerEpoch - 1, 5},
		{"gloas", 6 * fieldparams.SlotsPerEpoch, 6},
	} {
		t.Run(tt.name, func(t *testing.T) {
			optimistic, finality := lightClientBroadcastUpdates(t, tt.slot)
			for _, msg := range []struct {
				topic     string
				want      proto.Message
				broadcast func(context.Context) error
			}{
				{LightClientOptimisticUpdateTopicFormat, optimistic.Proto(), func(ctx context.Context) error { return s.BroadcastLightClientOptimisticUpdate(ctx, optimistic) }},
				{LightClientFinalityUpdateTopicFormat, finality.Proto(), func(ctx context.Context) error { return s.BroadcastLightClientFinalityUpdate(ctx, finality) }},
			} {
				t.Run(msg.topic, func(t *testing.T) {
					topic := fmt.Sprintf(msg.topic, params.ForkDigest(tt.epoch)) + s.Encoding().ProtocolSuffix()
					sub, err := remote.SubscribeToTopic(topic)
					require.NoError(t, err)
					defer sub.Cancel()
					_, err = s.JoinTopic(topic)
					require.NoError(t, err)
					require.Eventually(t, func() bool { return len(s.pubsub.ListPeers(topic)) > 0 }, 5*time.Second, 10*time.Millisecond)
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					defer cancel()
					require.NoError(t, msg.broadcast(ctx))
					incoming, err := sub.Next(ctx)
					require.NoError(t, err)
					require.Equal(t, topic, incoming.GetTopic())
					decoded := msg.want.ProtoReflect().Type().New().Interface()
					require.NoError(t, s.Encoding().DecodeGossip(incoming.Data, decoded.(ssz.Unmarshaler)))
					require.Equal(t, true, proto.Equal(msg.want, decoded))
				})
			}
		})
	}
}

func lightClientBroadcastUpdates(t *testing.T, slot primitives.Slot) (interfaces.LightClientOptimisticUpdate, interfaces.LightClientFinalityUpdate) {
	t.Helper()
	roots := func(count int) [][]byte {
		result := make([][]byte, count)
		for i := range result {
			result[i] = bytes.Repeat([]byte{byte(i + 1)}, 32)
		}
		return result
	}
	beacon := util.HydrateBeaconHeader(&ethpb.BeaconBlockHeader{Slot: slot, ProposerIndex: 42})
	aggregate := &ethpb.SyncAggregate{
		SyncCommitteeBits:      bytes.Repeat([]byte{255}, fieldparams.SyncAggregateSyncCommitteeBytesLength),
		SyncCommitteeSignature: bytes.Repeat([]byte{42}, 96),
	}
	var optimisticProto, finalityProto proto.Message
	if slot >= 6*fieldparams.SlotsPerEpoch {
		header := &ethpb.LightClientHeaderGloas{Beacon: beacon, ExecutionBlockHash: bytes.Repeat([]byte{17}, 32), ExecutionBranch: roots(11)}
		finalized := proto.Clone(header).(*ethpb.LightClientHeaderGloas)
		finalized.Beacon.Slot = 5 * fieldparams.SlotsPerEpoch
		optimisticProto = &ethpb.LightClientOptimisticUpdateGloas{AttestedHeader: header, SyncAggregate: aggregate, SignatureSlot: slot + 1}
		finalityProto = &ethpb.LightClientFinalityUpdateGloas{AttestedHeader: header, FinalizedHeader: finalized, FinalityBranch: roots(9), SyncAggregate: aggregate, SignatureSlot: slot + 1}
	} else {
		header := &ethpb.LightClientHeaderDeneb{Beacon: beacon, Execution: util.NewBlindedBeaconBlockFulu().Message.Body.ExecutionPayloadHeader, ExecutionBranch: roots(4)}
		finalized := proto.Clone(header).(*ethpb.LightClientHeaderDeneb)
		finalized.Beacon.Slot = 5 * fieldparams.SlotsPerEpoch
		optimisticProto = &ethpb.LightClientOptimisticUpdateDeneb{AttestedHeader: header, SyncAggregate: aggregate, SignatureSlot: slot + 1}
		finalityProto = &ethpb.LightClientFinalityUpdateElectra{AttestedHeader: header, FinalizedHeader: finalized, FinalityBranch: roots(7), SyncAggregate: aggregate, SignatureSlot: slot + 1}
	}
	optimistic, err := light_client.NewWrappedOptimisticUpdate(optimisticProto)
	require.NoError(t, err)
	finality, err := light_client.NewWrappedFinalityUpdate(finalityProto)
	require.NoError(t, err)
	return optimistic, finality
}
