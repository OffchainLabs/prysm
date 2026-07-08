package integrationtest

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/partialattestationbroadcaster"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/partialmsgmux"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	simlibp2p "github.com/libp2p/go-libp2p/x/simlibp2p"
	"github.com/marcopolo/simnet"
)

const testSlot = primitives.Slot(100)

func testAttData(slot primitives.Slot) *ethpb.AttestationData {
	return &ethpb.AttestationData{
		Slot:            slot,
		BeaconBlockRoot: bytes.Repeat([]byte{0x0b}, 32),
		Source:          &ethpb.Checkpoint{Epoch: 2, Root: bytes.Repeat([]byte{0x0c}, 32)},
		Target:          &ethpb.Checkpoint{Epoch: 3, Root: bytes.Repeat([]byte{0x0d}, 32)},
	}
}

// idxSig derives a per-validator signature so distinct validators carry
// distinct tuple identities.
func idxSig(idx primitives.ValidatorIndex) []byte {
	return bytes.Repeat([]byte{byte(idx)}, 96)
}

// TestTwoNodePartialAttestationExchange runs two nodes over a simulated
// network with real latency and bandwidth. Each node originates two
// attestations of the same data through Submit (the pre-validated ingress);
// each must receive the other's through the partial-messages push path and
// replay them through its validation callback.
func TestTwoNodePartialAttestationExchange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		latency := time.Millisecond * 10
		network, meta, err := simlibp2p.SimpleLibp2pNetwork([]simlibp2p.NodeLinkSettingsAndCount{
			{LinkSettings: simnet.NodeBiDiLinkSettings{
				Downlink: simnet.LinkSettings{BitsPerSecond: 20 * simlibp2p.OneMbps},
				Uplink:   simnet.LinkSettings{BitsPerSecond: 20 * simlibp2p.OneMbps},
			}, Count: 2},
		}, simnet.StaticLatency(latency/2), simlibp2p.NetworkSettings{UseBlankHost: true})
		require.NoError(t, err)
		network.Start()
		defer network.Close()
		defer func() {
			for _, node := range meta.Nodes {
				require.NoError(t, node.Close())
			}
		}()

		h1, h2 := meta.Nodes[0], meta.Nodes[1]

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		currentSlot := func() primitives.Slot { return testSlot }
		b1 := partialattestationbroadcaster.NewBroadcaster(ctx, currentSlot)
		b2 := partialattestationbroadcaster.NewBroadcaster(ctx, currentSlot)

		mux1 := partialmsgmux.New()
		mux1.RegisterAttestationHandler(b1)
		mux2 := partialmsgmux.New()
		mux2.RegisterAttestationHandler(b2)

		psOpts := []pubsub.Option{
			pubsub.WithMessageSigning(false),
			pubsub.WithStrictSignatureVerification(false),
		}
		ps1, err := pubsub.NewGossipSub(ctx, h1, mux1.AppendPubSubOpts(psOpts)...)
		require.NoError(t, err)
		ps2, err := pubsub.NewGossipSub(ctx, h2, mux2.AppendPubSubOpts(psOpts)...)
		require.NoError(t, err)

		digest := params.ForkDigest(0)
		topicStr := fmt.Sprintf(p2p.AttestationSubnetTopicFormat, digest, 0) +
			encoder.SszNetworkEncoder{}.ProtocolSuffix()

		// observed collects the validator indices each node's validation
		// callback replays; everything is accepted.
		observe := func(
			obs chan primitives.ValidatorIndex,
		) partialattestationbroadcaster.ProcessAttestationFn {
			return func(topic string, att *ethpb.SingleAttestation) (bool, error) {
				require.Equal(t, topicStr, topic)
				require.Equal(t, testSlot, att.Data.Slot)
				require.DeepEqual(t, idxSig(att.AttesterIndex), att.Signature)
				obs <- att.AttesterIndex
				return true, nil
			}
		}
		obs1 := make(chan primitives.ValidatorIndex, 16)
		obs2 := make(chan primitives.ValidatorIndex, 16)
		go b1.Start(observe(obs1))
		go b2.Start(observe(obs2))

		require.NoError(t, h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}))

		topic1, err := ps1.Join(topicStr, pubsub.RequestPartialMessages())
		require.NoError(t, err)
		sub1, err := topic1.Subscribe()
		require.NoError(t, err)
		defer sub1.Cancel()

		topic2, err := ps2.Join(topicStr, pubsub.RequestPartialMessages())
		require.NoError(t, err)
		sub2, err := topic2.Subscribe()
		require.NoError(t, err)
		defer sub2.Cancel()

		// Wait for the partial-messages mesh to form.
		time.Sleep(2 * time.Second)

		// Each node originates a disjoint half of the committee.
		data := testAttData(testSlot)
		submit := func(
			b *partialattestationbroadcaster.Broadcaster, indices ...primitives.ValidatorIndex,
		) {
			for _, idx := range indices {
				b.Submit(topicStr, &ethpb.SingleAttestation{
					CommitteeId:   3,
					AttesterIndex: idx,
					Data:          data,
					Signature:     idxSig(idx),
				})
			}
		}
		submit(b1, 101, 107)
		submit(b2, 103, 109)

		// Each node must observe exactly the other's validators: its own
		// submissions are pre-validated and never re-enter its callback.
		await := func(
			obs chan primitives.ValidatorIndex, want map[primitives.ValidatorIndex]bool, node string,
		) {
			got := map[primitives.ValidatorIndex]bool{}
			timeout := time.After(30 * time.Second)
			for len(got) < len(want) {
				select {
				case idx := <-obs:
					require.Equal(t, true, want[idx], "node %s observed unexpected validator %d", node, idx)
					got[idx] = true
				case <-timeout:
					t.Fatalf("node %s observed %d/%d attestations", node, len(got), len(want))
				}
			}
		}
		await(obs1, map[primitives.ValidatorIndex]bool{103: true, 109: true}, "1")
		await(obs2, map[primitives.ValidatorIndex]bool{101: true, 107: true}, "2")

		// Nothing further arrives: no echo of a node's own attestations and
		// no replays of the exchanged ones.
		time.Sleep(2 * time.Second)
		require.Equal(t, 0, len(obs1), "node 1 received an echo or replay")
		require.Equal(t, 0, len(obs2), "node 2 received an echo or replay")
	})
}
