package partialattestationbroadcaster

import (
	"bytes"
	"iter"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/partialmsgmux"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

const testTopic = "/eth2/aabbccdd/beacon_attestation_0/ssz_snappy"

const testSlot = primitives.Slot(100)

func testAttData(slot primitives.Slot) *ethpb.AttestationData {
	return &ethpb.AttestationData{
		Slot:            slot,
		BeaconBlockRoot: bytes.Repeat([]byte{0x0b}, 32),
		Source:          &ethpb.Checkpoint{Epoch: 2, Root: bytes.Repeat([]byte{0x0c}, 32)},
		Target:          &ethpb.Checkpoint{Epoch: 3, Root: bytes.Repeat([]byte{0x0d}, 32)},
	}
}

func testBundle(t *testing.T, slot primitives.Slot) (*ethpb.AttestationBundle, []byte) {
	t.Helper()
	bundle := &ethpb.AttestationBundle{
		CommitteeIndex:  3,
		AttestationData: testAttData(slot),
		AttesterIndices: []uint64{101, 107},
		Signatures:      [][]byte{bytes.Repeat([]byte{0x01}, 96), bytes.Repeat([]byte{0x02}, 96)},
	}
	enc, err := bundle.MarshalSSZ()
	require.NoError(t, err)
	return bundle, enc
}

func testMeta(t *testing.T) (*ethpb.CommitteeAttestationPartsMetadata, []byte) {
	t.Helper()
	meta := &ethpb.CommitteeAttestationPartsMetadata{
		CommitteeIndex: 3,
		Available:      []uint64{101},
		Requests:       []uint64{102},
	}
	enc, err := meta.MarshalSSZ()
	require.NoError(t, err)
	return meta, enc
}

func TestOnIncomingRPC(t *testing.T) {
	bundle, bundleEnc := testBundle(t, testSlot)
	meta, metaEnc := testMeta(t)
	_, wrongSlotBundleEnc := testBundle(t, testSlot+1)

	lengthMismatch := &ethpb.AttestationBundle{
		CommitteeIndex:  3,
		AttestationData: testAttData(testSlot),
		AttesterIndices: []uint64{101, 107},
		Signatures:      [][]byte{bytes.Repeat([]byte{0x01}, 96)},
	}
	lengthMismatchEnc, err := lengthMismatch.MarshalSSZ()
	require.NoError(t, err)

	duplicateIdx := &ethpb.AttestationBundle{
		CommitteeIndex:  3,
		AttestationData: testAttData(testSlot),
		AttesterIndices: []uint64{107, 107},
		Signatures:      [][]byte{bytes.Repeat([]byte{0x01}, 96), bytes.Repeat([]byte{0x02}, 96)},
	}
	duplicateIdxEnc, err := duplicateIdx.MarshalSSZ()
	require.NoError(t, err)

	// testSlot is 100, so with 32 slots per epoch the propagation window
	// (current or previous epoch, at most one slot ahead) is slots 64-101.
	oldSlot, boundarySlot, futureSlot := primitives.Slot(63), primitives.Slot(64), primitives.Slot(102)
	boundaryBundle, boundaryBundleEnc := testBundle(t, boundarySlot)

	rpc := func(groupID, msg, md []byte) *pubsub_pb.PartialMessagesExtension {
		return &pubsub_pb.PartialMessagesExtension{GroupID: groupID, PartialMessage: msg, PartsMetadata: md}
	}

	tests := []struct {
		name       string
		rpc        *pubsub_pb.PartialMessagesExtension
		wantErr    bool
		slot       primitives.Slot
		wantBundle *ethpb.AttestationBundle
		wantMeta   *ethpb.CommitteeAttestationPartsMetadata
	}{
		{name: "bad group ID length", rpc: rpc([]byte{1, 2, 3}, bundleEnc, nil), wantErr: true},
		{name: "junk bundle", rpc: rpc(GroupID(testSlot), []byte{0xde, 0xad}, nil), wantErr: true},
		{name: "bundle slot mismatch", rpc: rpc(GroupID(testSlot), wrongSlotBundleEnc, nil), wantErr: true},
		{name: "indices and signatures length mismatch", rpc: rpc(GroupID(testSlot), lengthMismatchEnc, nil), wantErr: true},
		{name: "duplicate attester index in bundle", rpc: rpc(GroupID(testSlot), duplicateIdxEnc, nil), wantErr: true},
		{name: "junk metadata", rpc: rpc(GroupID(testSlot), nil, []byte{0xde, 0xad}), wantErr: true},
		{name: "empty rpc", rpc: rpc(GroupID(testSlot), nil, nil)},
		{name: "slot before the window is ignored", rpc: rpc(GroupID(oldSlot), bundleEnc, nil)},
		{name: "future slot is ignored", rpc: rpc(GroupID(futureSlot), bundleEnc, nil)},
		{name: "bundle", rpc: rpc(GroupID(testSlot), bundleEnc, nil), slot: testSlot, wantBundle: bundle},
		{name: "bundle at the window boundary", rpc: rpc(GroupID(boundarySlot), boundaryBundleEnc, nil), slot: boundarySlot, wantBundle: boundaryBundle},
		{name: "metadata", rpc: rpc(GroupID(testSlot), nil, metaEnc), slot: testSlot, wantMeta: meta},
		{name: "bundle and metadata", rpc: rpc(GroupID(testSlot), bundleEnc, metaEnc), slot: testSlot, wantBundle: bundle, wantMeta: meta},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBroadcaster(t.Context(), func() primitives.Slot { return testSlot })
			err := b.OnIncomingRPC("peer", nil, tt.rpc)
			if tt.wantErr {
				require.NotNil(t, err)
				require.Equal(t, 0, len(b.incoming))
				return
			}
			require.NoError(t, err)
			if tt.wantBundle == nil && tt.wantMeta == nil {
				require.Equal(t, 0, len(b.incoming))
				return
			}
			in := <-b.incoming
			require.Equal(t, peer.ID("peer"), in.From)
			require.Equal(t, tt.slot, in.Slot)
			require.DeepEqual(t, tt.wantBundle, in.Bundle)
			require.DeepEqual(t, tt.wantMeta, in.Meta)
		})
	}
}

// Bundle indices and metadata available lists both fold into the sender's
// per-committee availability on the GS event loop.
func TestOnIncomingRPCUpdatesPeerAvailable(t *testing.T) {
	b := NewBroadcaster(t.Context(), func() primitives.Slot { return testSlot })
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{}
	topicStr := testTopic

	_, bundleEnc := testBundle(t, testSlot)
	require.NoError(t, b.OnIncomingRPC("bundlePeer", peerStates, &pubsub_pb.PartialMessagesExtension{
		GroupID: GroupID(testSlot), PartialMessage: bundleEnc, TopicID: &topicStr,
	}))
	avail := peerStates["bundlePeer"].Att.Available[3]
	require.NotNil(t, avail)
	require.Equal(t, true, hasIdx(avail, 101))
	require.Equal(t, true, hasIdx(avail, 107))
	require.Equal(t, false, hasIdx(avail, 102))

	// Parts metadata claims fold into the same committee (testMeta lists 101).
	_, metaEnc := testMeta(t)
	require.NoError(t, b.OnIncomingRPC("metaPeer", peerStates, &pubsub_pb.PartialMessagesExtension{
		GroupID: GroupID(testSlot), PartsMetadata: metaEnc, TopicID: &topicStr,
	}))
	availMeta := peerStates["metaPeer"].Att.Available[3]
	require.NotNil(t, availMeta)
	require.Equal(t, true, hasIdx(availMeta, 101))
	require.Equal(t, false, hasIdx(availMeta, 107))
}

func hasIdx[K comparable, V any](set map[K]V, idx K) bool {
	_, ok := set[idx]
	return ok
}

// The event loop hands each bundled attestation to the classic validation
// callback untouched.
func TestStartHandsBundlesToCallbacks(t *testing.T) {
	bundle, bundleEnc := testBundle(t, testSlot)

	processedCh := make(chan *ethpb.SingleAttestation, len(bundle.AttesterIndices))
	process := func(topic string, att *ethpb.SingleAttestation) (bool, error) {
		require.Equal(t, testTopic, topic)
		require.Equal(t, bundle.CommitteeIndex, att.CommitteeId)
		require.Equal(t, testSlot, att.Data.Slot)
		processedCh <- att
		return true, nil
	}

	b := NewBroadcaster(t.Context(), func() primitives.Slot { return testSlot })
	go b.Start(process)

	topicStr := testTopic
	rpc := &pubsub_pb.PartialMessagesExtension{GroupID: GroupID(testSlot), PartialMessage: bundleEnc, TopicID: &topicStr}
	require.NoError(t, b.OnIncomingRPC("peer", nil, rpc))
	got := map[uint64][]byte{}
	for range bundle.AttesterIndices {
		select {
		case att := <-processedCh:
			got[uint64(att.AttesterIndex)] = att.Signature
		case <-time.After(5 * time.Second):
			t.Fatal("bundle never reached the validation callback")
		}
	}
	for i, idx := range bundle.AttesterIndices {
		require.DeepEqual(t, bundle.Signatures[i], got[idx])
	}
}

// newNode creates a libp2p host running gossipsub with the partial-messages
// extension registered through the mux, exactly as the beacon node does it.
func newNode(t *testing.T) (host.Host, *pubsub.PubSub, *Broadcaster) {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	b := NewBroadcaster(t.Context(), func() primitives.Slot { return testSlot })
	mux := partialmsgmux.New()
	mux.RegisterAttestationHandler(b)
	ps, err := pubsub.NewGossipSub(t.Context(), h, mux.AppendPubSubOpts(nil)...)
	require.NoError(t, err)
	return h, ps, b
}

// joinAndSubscribe joins and subscribes to the attestation subnet topic,
// announcing the partial-messages flags so the mesh can form.
func joinAndSubscribe(t *testing.T, ps *pubsub.PubSub, opts ...pubsub.TopicOpt) *pubsub.Topic {
	t.Helper()
	topic, err := ps.Join(testTopic, opts...)
	require.NoError(t, err)
	_, err = topic.Subscribe()
	require.NoError(t, err)
	return topic
}

// sendToAll returns a PublishActionsFn sending the encoded bundle and
// metadata to every tracked peer, recording which peers it saw.
func sendToAll(bundleEnc, metaEnc []byte, seen map[peer.ID]bool) partialmessages.PublishActionsFn[blocks.PartialMessagePeerState] {
	return func(peerStates map[peer.ID]blocks.PartialMessagePeerState, _ func(peer.ID) bool) iter.Seq2[peer.ID, partialmessages.PublishAction] {
		return func(yield func(peer.ID, partialmessages.PublishAction) bool) {
			for p := range peerStates {
				seen[p] = true
				action := partialmessages.PublishAction{EncodedPartialMessage: bundleEnc, EncodedPartsMetadata: metaEnc}
				if !yield(p, action) {
					return
				}
			}
		}
	}
}

// Happy path: a bundle plus metadata published by one node arrives decoded
// at the other's broadcaster.
func TestBundleExchange(t *testing.T) {
	h1, ps1, b1 := newNode(t)
	h2, ps2, b2 := newNode(t)
	require.NoError(t, h1.Connect(t.Context(), peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}))

	joinAndSubscribe(t, ps1, pubsub.RequestPartialMessages())
	joinAndSubscribe(t, ps2, pubsub.RequestPartialMessages())

	bundle, bundleEnc := testBundle(t, testSlot)
	meta, metaEnc := testMeta(t)

	// Publish until the mesh has formed and node2 is tracked for this group.
	seen := map[peer.ID]bool{}
	require.Eventually(t, func() bool {
		require.NoError(t, b1.publishPartial(testTopic, GroupID(testSlot), sendToAll(bundleEnc, metaEnc, seen)))
		return seen[h2.ID()]
	}, 10*time.Second, 100*time.Millisecond, "node2 never joined the partial-messages mesh")

	// The decoded bundle and metadata must arrive on node2's incoming queue.
	select {
	case got := <-b2.incoming:
		require.Equal(t, h1.ID(), got.From)
		require.Equal(t, testSlot, got.Slot)
		require.DeepEqual(t, bundle, got.Bundle)
		require.DeepEqual(t, meta, got.Meta)
	case <-time.After(10 * time.Second):
		t.Fatal("node2 never received the bundle")
	}
}

// acceptAllCallbacks accepts every attestation and reports each accepted
// validator index on obs.
func acceptAllCallbacks(obs chan []uint64) ProcessAttestationFn {
	return func(_ string, att *ethpb.SingleAttestation) (bool, error) {
		obs <- []uint64{uint64(att.AttesterIndex)}
		return true, nil
	}
}

// partialMeshPeers publishes a probe for the test slot's group and returns the
// partial-message peers gossipsub tracks for it. The closure reads only its
// local set, so it is safe to call while the Start loop runs.
func partialMeshPeers(t *testing.T, b *Broadcaster) map[peer.ID]bool {
	t.Helper()
	seen := map[peer.ID]bool{}
	fn := func(peerStates map[peer.ID]blocks.PartialMessagePeerState, requestsPartial func(peer.ID) bool) iter.Seq2[peer.ID, partialmessages.PublishAction] {
		return func(func(peer.ID, partialmessages.PublishAction) bool) {
			for p := range peerStates {
				if requestsPartial(p) {
					seen[p] = true
				}
			}
		}
	}
	require.NoError(t, b.publishPartial(testTopic, GroupID(testSlot), fn))
	return seen
}

// threeNodeChain wires three broadcasters over real gossipsub in a chain
// A-B-C (A and C never connect) and blocks until the meshes form.
func threeNodeChain(t *testing.T) (bA, bB, bC *Broadcaster, obsA, obsB, obsC chan []uint64) {
	t.Helper()
	hA, psA, bA := newNode(t)
	hB, psB, bB := newNode(t)
	hC, psC, bC := newNode(t)

	require.NoError(t, hA.Connect(t.Context(), peer.AddrInfo{ID: hB.ID(), Addrs: hB.Addrs()}))
	require.NoError(t, hB.Connect(t.Context(), peer.AddrInfo{ID: hC.ID(), Addrs: hC.Addrs()}))

	joinAndSubscribe(t, psA, pubsub.RequestPartialMessages())
	joinAndSubscribe(t, psB, pubsub.RequestPartialMessages())
	joinAndSubscribe(t, psC, pubsub.RequestPartialMessages())

	obsA = make(chan []uint64, 16)
	obsB = make(chan []uint64, 16)
	obsC = make(chan []uint64, 16)
	go bA.Start(acceptAllCallbacks(obsA))
	go bB.Start(acceptAllCallbacks(obsB))
	go bC.Start(acceptAllCallbacks(obsC))

	// A must track B, B must track A and C, C must track B. Mesh formation
	// dominates the runtime.
	require.Eventually(t, func() bool {
		return partialMeshPeers(t, bA)[hB.ID()] &&
			partialMeshPeers(t, bB)[hA.ID()] && partialMeshPeers(t, bB)[hC.ID()] &&
			partialMeshPeers(t, bC)[hB.ID()]
	}, 15*time.Second, 200*time.Millisecond, "partial-message meshes never formed")
	return bA, bB, bC, obsA, obsB, obsC
}

// TestEndToEndPropagation injects a bundle at A via the raw partial ingress;
// it must reach C by store-driven re-propagation through B.
func TestEndToEndPropagation(t *testing.T) {
	bA, _, _, obsA, obsB, obsC := threeNodeChain(t)

	// Inject with a fake from-peer so nothing suppresses the push toward B.
	data := testAttData(testSlot)
	dataRoot, err := data.HashTreeRoot()
	require.NoError(t, err)
	bA.incoming <- incomingRPC{
		From:     peer.ID("injector"),
		Topic:    testTopic,
		Slot:     testSlot,
		DataRoot: dataRoot,
		Bundle: &ethpb.AttestationBundle{
			CommitteeIndex:  3,
			AttestationData: data,
			AttesterIndices: []uint64{101, 107},
			Signatures:      [][]byte{idxSig(101), idxSig(107)},
		},
	}

	want := []uint64{101, 107}
	awaitPositions(t, obsA, want, 5*time.Second, "A") // the injection
	awaitPositions(t, obsB, want, 15*time.Second, "B")
	awaitPositions(t, obsC, want, 15*time.Second, "C")

	// The bundle must not echo back to A.
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, 0, len(obsA), "attestation echoed back to A")
}

// Attestations originated through Submit at A reach B and C; A's own
// ProcessAttestations never fires for a pre-validated submission.
func TestEndToEndPropagationViaSubmit(t *testing.T) {
	bA, _, _, obsA, obsB, obsC := threeNodeChain(t)

	data := testAttData(testSlot)
	for _, idx := range []uint64{101, 107} {
		bA.Submit(testTopic, &ethpb.SingleAttestation{
			CommitteeId:   3,
			AttesterIndex: primitives.ValidatorIndex(idx),
			Data:          data,
			Signature:     idxSig(idx),
		})
	}

	want := []uint64{101, 107}
	awaitPositions(t, obsB, want, 15*time.Second, "B")
	awaitPositions(t, obsC, want, 15*time.Second, "C")

	// A pre-validated submission never re-enters A's classic validator.
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, 0, len(obsA), "submitted attestation validated on the originating node")
}

// awaitPositions collects observed indices until it has seen all of want.
func awaitPositions(t *testing.T, obs chan []uint64, want []uint64, timeout time.Duration, node string) {
	t.Helper()
	got := map[uint64]bool{}
	deadline := time.After(timeout)
	for len(got) < len(want) {
		select {
		case indices := <-obs:
			for _, idx := range indices {
				got[idx] = true
			}
		case <-deadline:
			t.Fatalf("node %s never observed all attestations, got %v", node, got)
		}
	}
}

// A peer joining without RequestPartialMessages never appears in peer states
// and receives no partial RPCs.
func TestPeerWithoutPartialTopic(t *testing.T) {
	h1, ps1, b1 := newNode(t)
	h2, ps2, b2 := newNode(t)
	require.NoError(t, h1.Connect(t.Context(), peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}))

	topic1 := joinAndSubscribe(t, ps1, pubsub.RequestPartialMessages())
	joinAndSubscribe(t, ps2) // no partial-messages topic options

	// Wait until the nodes see each other on the topic at all.
	require.Eventually(t, func() bool {
		return len(topic1.ListPeers()) > 0
	}, 10*time.Second, 100*time.Millisecond, "nodes never met on the topic")

	// Node2 must stay invisible to the partial-messages extension.
	_, bundleEnc := testBundle(t, testSlot)
	_, metaEnc := testMeta(t)
	seen := map[peer.ID]bool{}
	for range 20 {
		require.NoError(t, b1.publishPartial(testTopic, GroupID(testSlot), sendToAll(bundleEnc, metaEnc, seen)))
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, false, seen[h2.ID()], "node2 negotiated partial messages despite not requesting them")
	require.Equal(t, 0, len(b2.incoming), "node2 received a partial RPC")
}
