package partialattestationbroadcaster

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	"github.com/libp2p/go-libp2p/core/peer"
)

// storeHarness drives handleIncoming directly with counting callbacks and a
// controllable clock.
type storeHarness struct {
	b          *Broadcaster
	current    *primitives.Slot
	processed  []uint64        // index passed to the process callback, per call
	acceptOnly map[uint64]bool // when non-nil, only these indices are accepted
	process    ProcessAttestationFn
}

func newStoreHarness(t *testing.T) *storeHarness {
	current := testSlot
	h := &storeHarness{current: &current}
	h.process = func(_ string, att *ethpb.SingleAttestation) (bool, error) {
		idx := uint64(att.AttesterIndex)
		h.processed = append(h.processed, idx)
		if att.Signature[0] == 0xEE { // garbage-signature marker
			return false, nil
		}
		if h.acceptOnly != nil && !h.acceptOnly[idx] {
			return false, nil
		}
		return true, nil
	}
	h.b = NewBroadcaster(t.Context(), func() primitives.Slot { return *h.current })
	// Two heartbeats to signature expiry keeps the lifecycle tests short.
	h.b.sigTTL = 2
	return h
}

// idxSig derives a per-validator signature so distinct validators get
// distinct tuple identities across the tests.
func idxSig(idx uint64) []byte {
	return bytes.Repeat([]byte{byte(idx)}, 96)
}

// deliverSigsNoPump dispatches a bundle with explicit signatures, leaving
// its validation job queued.
func (h *storeHarness) deliverSigsNoPump(t *testing.T, slot primitives.Slot, indices []uint64, sigs [][]byte) {
	t.Helper()
	data := testAttData(slot)
	dataRoot, err := data.HashTreeRoot()
	require.NoError(t, err)
	h.b.handleIncoming(incomingRPC{
		From:     "peer",
		Topic:    testTopic,
		Slot:     slot,
		DataRoot: dataRoot,
		Bundle: &ethpb.AttestationBundle{
			CommitteeIndex:  3,
			AttestationData: data,
			AttesterIndices: indices,
			Signatures:      sigs,
		},
	})
}

// deliver dispatches a bundle with index-derived signatures and runs
// validation synchronously.
func (h *storeHarness) deliver(t *testing.T, slot primitives.Slot, indices ...uint64) {
	t.Helper()
	sigs := make([][]byte, len(indices))
	for i := range sigs {
		sigs[i] = idxSig(indices[i])
	}
	h.deliverSigsNoPump(t, slot, indices, sigs)
	h.pump(t)
}

// pump runs queued validation jobs and their completions synchronously.
func (h *storeHarness) pump(t *testing.T) {
	t.Helper()
	for {
		select {
		case j := <-h.b.valJobs:
			h.b.handleValDone(h.b.runValJob(h.process, j))
		default:
			return
		}
	}
}

// submit runs Submit and its Start-loop work synchronously.
func (h *storeHarness) submit(t *testing.T, slot primitives.Slot, attesterIndex primitives.ValidatorIndex, sig []byte) {
	t.Helper()
	att := &ethpb.SingleAttestation{
		CommitteeId:   3,
		AttesterIndex: attesterIndex,
		Data:          testAttData(slot),
		Signature:     sig,
	}
	h.b.Submit(testTopic, att)
	for {
		select {
		case s := <-h.b.submit:
			h.b.handleSubmission(s)
		default:
			h.pump(t)
			return
		}
	}
}

func (h *storeHarness) slotGroup(t *testing.T, slot primitives.Slot) *slotAtts {
	t.Helper()
	g := h.b.groups[testTopic][slot]
	require.NotNil(t, g)
	return g
}

func TestStoreDedupsValidatedIndices(t *testing.T) {
	h := newStoreHarness(t)

	h.deliver(t, testSlot, 101, 107)
	require.DeepEqual(t, []uint64{101, 107}, h.processed)

	// An identical replay never reaches validation.
	h.deliver(t, testSlot, 101, 107)
	require.DeepEqual(t, []uint64{101, 107}, h.processed)

	// An overlapping bundle only validates the new validator.
	h.deliver(t, testSlot, 107, 109)
	require.DeepEqual(t, []uint64{101, 107, 109}, h.processed)
}

func TestRejectedTupleNotRetriedWithinTTL(t *testing.T) {
	h := newStoreHarness(t)
	h.acceptOnly = map[uint64]bool{101: true}

	h.deliver(t, testSlot, 101, 107)
	require.DeepEqual(t, []uint64{101, 107}, h.processed)

	// The rejected tuple is remembered: the same signature is not validated
	// again within the TTL.
	h.deliver(t, testSlot, 101, 107)
	require.DeepEqual(t, []uint64{101, 107}, h.processed)

	// A different signature for the rejected validator is a different tuple
	// and validates immediately.
	h.acceptOnly = nil
	h.deliverSigsNoPump(t, testSlot, []uint64{107}, [][]byte{bytes.Repeat([]byte{0xBB}, 96)})
	h.pump(t)
	require.DeepEqual(t, []uint64{101, 107, 107}, h.processed)
	require.Equal(t, true, hasIdx(h.slotGroup(t, testSlot).validated, 107))
}

func TestSeenCacheExpiresAfterTTL(t *testing.T) {
	h := newStoreHarness(t)
	h.acceptOnly = map[uint64]bool{} // reject everything

	h.deliver(t, testSlot, 101)
	require.Equal(t, 1, len(h.processed))
	require.Equal(t, 0, len(h.b.groups))

	// Still cached until the TTL runs out.
	for range seenTTLHeartbeats() - 1 {
		h.b.cleanup(*h.current)
	}
	h.deliver(t, testSlot, 101)
	require.Equal(t, 1, len(h.processed))

	// Expired: the tuple validates again.
	h.b.cleanup(*h.current)
	h.deliver(t, testSlot, 101)
	require.Equal(t, 2, len(h.processed))
}

// A racing garbage signature is a different tuple: both validate and the
// honest one lands in the store.
func TestGarbageSignatureDoesNotSuppressHonest(t *testing.T) {
	h := newStoreHarness(t)
	garbage := bytes.Repeat([]byte{0xEE}, 96)
	good := bytes.Repeat([]byte{0xBB}, 96)

	h.deliverSigsNoPump(t, testSlot, []uint64{107}, [][]byte{garbage})
	h.deliverSigsNoPump(t, testSlot, []uint64{107}, [][]byte{good})
	// An exact duplicate of an in-flight tuple is dropped by the seen cache.
	h.deliverSigsNoPump(t, testSlot, []uint64{107}, [][]byte{garbage})
	require.Equal(t, 2, len(h.b.valJobs))

	h.pump(t)
	require.Equal(t, 2, len(h.processed))

	g := h.slotGroup(t, testSlot)
	require.Equal(t, true, hasIdx(g.validated, 107))
	require.DeepEqual(t, good, g.validated[107].Signature)
}

func TestStoreLifecycle(t *testing.T) {
	h := newStoreHarness(t)

	h.deliver(t, testSlot, 101, 107)
	g := h.slotGroup(t, testSlot)
	require.Equal(t, 2, len(g.validated))
	require.Equal(t, 1, len(g.attData))
	require.NotNil(t, g.validated[101].Signature)
	require.Equal(t, true, hasIdx(g.validated, 107))

	// Within the signature TTL nothing changes.
	h.b.cleanup(*h.current)
	require.Equal(t, 1, len(g.attData))

	// Past the TTL the signatures and their data are deleted together.
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(g.attData))
	require.Equal(t, 0, len(g.validated))

	// Replays of the expired tuples still never reach validation: the seen
	// cache outlives the signatures.
	before := len(h.processed)
	h.deliver(t, testSlot, 101, 107)
	require.Equal(t, before, len(h.processed))

	// A late reveal of a new validator validates and gets its own TTL.
	h.deliver(t, testSlot, 109)
	require.Equal(t, uint64(109), h.processed[len(h.processed)-1])
	require.Equal(t, 1, len(g.attData))
	require.NotNil(t, g.validated[109].Signature)
	require.Equal(t, true, hasIdx(g.validated, 109))

	// The late signature expires on its own schedule.
	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(g.attData))

	// Once the slot leaves the propagation window the group is deleted.
	twoEpochsLater := testSlot + 2*primitives.Slot(32)
	h.b.cleanup(twoEpochsLater)
	require.Equal(t, 0, len(h.b.groups))
}

// installPush installs a fake publishPartial capturing the bundles pushed to
// each peer, keyed by peer. peerStates and partial drive the per-peer diff.
func (h *storeHarness) installPush(t *testing.T, peerStates map[peer.ID]blocks.PartialMessagePeerState, partial map[peer.ID]bool) map[peer.ID][]*ethpb.AttestationBundle {
	pushed := map[peer.ID][]*ethpb.AttestationBundle{}
	h.b.publishPartial = func(topic string, groupID []byte, fn partialmessages.PublishActionsFn[blocks.PartialMessagePeerState]) error {
		require.Equal(t, testTopic, topic)
		require.DeepEqual(t, GroupID(testSlot), groupID)
		for pid, action := range fn(peerStates, func(p peer.ID) bool { return partial[p] }) {
			require.NoError(t, action.Err)
			bundle := &ethpb.AttestationBundle{}
			require.NoError(t, bundle.UnmarshalSSZ(action.EncodedPartialMessage))
			pushed[pid] = append(pushed[pid], bundle)
		}
		return nil
	}
	return pushed
}

// pushedIndices flattens the bundles pushed to a peer into a sorted index set.
func pushedIndices(bundles []*ethpb.AttestationBundle) []uint64 {
	var indices []uint64
	for _, b := range bundles {
		indices = append(indices, b.AttesterIndices...)
	}
	return indices
}

// installMetaPush installs a fake publishPartial capturing the parts metadata
// pushed to each peer.
func (h *storeHarness) installMetaPush(t *testing.T, peerStates map[peer.ID]blocks.PartialMessagePeerState) map[peer.ID]*ethpb.CommitteeAttestationPartsMetadata {
	pushed := map[peer.ID]*ethpb.CommitteeAttestationPartsMetadata{}
	h.b.publishPartial = func(topic string, groupID []byte, fn partialmessages.PublishActionsFn[blocks.PartialMessagePeerState]) error {
		require.Equal(t, testTopic, topic)
		require.DeepEqual(t, GroupID(testSlot), groupID)
		for pid, action := range fn(peerStates, func(peer.ID) bool { return true }) {
			require.NoError(t, action.Err)
			require.Equal(t, 0, len(action.EncodedPartialMessage)) // metadata never delivers
			meta := &ethpb.CommitteeAttestationPartsMetadata{}
			require.NoError(t, meta.UnmarshalSSZ(action.EncodedPartsMetadata))
			pushed[pid] = meta
		}
		return nil
	}
	return pushed
}

// avail builds a peer availability set with the given indices.
func avail(indices ...uint64) map[uint64]struct{} {
	set := make(map[uint64]struct{}, len(indices))
	for _, idx := range indices {
		set[idx] = struct{}{}
	}
	return set
}

// peerWith builds a peer state claiming the given indices for committee 3.
func peerWith(indices ...uint64) blocks.PartialMessagePeerState {
	return blocks.PartialMessagePeerState{Att: blocks.PartialAttestationPeerState{
		Available: map[primitives.CommitteeIndex]map[uint64]struct{}{3: avail(indices...)},
	}}
}

// deliverMeta hands handleIncoming a parts-metadata RPC for the test slot.
func (h *storeHarness) deliverMeta(t *testing.T, from peer.ID, available, requests []uint64) {
	t.Helper()
	h.b.handleIncoming(incomingRPC{
		From:  from,
		Topic: testTopic,
		Slot:  testSlot,
		Meta: &ethpb.CommitteeAttestationPartsMetadata{
			CommitteeIndex: 3,
			Available:      available,
			Requests:       requests,
		},
	})
}

// A gossip emission advertises the slot's validated validators once to the
// given peers; nothing about them is tracked.
func TestEmitGossip(t *testing.T) {
	h := newStoreHarness(t)

	h.deliver(t, testSlot, 101, 107)

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{}
	pushed := h.installMetaPush(t, peerStates)
	h.b.emitGossip(gossipEmit{topic: testTopic, slot: testSlot, peers: []peer.ID{"g1", "g2"}})

	require.Equal(t, 2, len(pushed))
	meta := pushed["g1"]
	require.NotNil(t, meta)
	require.Equal(t, primitives.CommitteeIndex(3), meta.CommitteeIndex)
	// Available is the full validated snapshot; requests are empty.
	require.DeepEqual(t, []uint64{101, 107}, meta.Available)
	require.Equal(t, 0, len(meta.Requests))
	require.NotNil(t, pushed["g2"])
	// Gossip is oneshot: the gossip peers stay untracked.
	require.Equal(t, 0, len(peerStates))
}

// A slot whose signatures all expired cannot serve and advertises nothing.
func TestEmitGossipSkipsExpired(t *testing.T) {
	h := newStoreHarness(t)
	pushed := h.installMetaPush(t, map[peer.ID]blocks.PartialMessagePeerState{})

	h.deliver(t, testSlot, 101)
	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).attData))

	h.b.emitGossip(gossipEmit{topic: testTopic, slot: testSlot, peers: []peer.ID{"g1"}})
	require.Equal(t, 0, len(pushed))
}

// Committing pushes nothing; the flush tick pushes to partial peers lacking
// the attestations and folds them into their availability.
func TestPushIsTickDriven(t *testing.T) {
	h := newStoreHarness(t)

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{
		"partialEmpty":   {},
		"partialCovered": peerWith(101, 107),
		"nonPartial":     {},
	}
	partial := map[peer.ID]bool{"partialEmpty": true, "partialCovered": true, "nonPartial": false}
	pushed := h.installPush(t, peerStates, partial)

	// Committing marks the validators dirty but pushes nothing.
	h.deliver(t, testSlot, 101, 107)
	require.Equal(t, 0, len(pushed))
	require.DeepEqual(t, []primitives.ValidatorIndex{101, 107}, h.slotGroup(t, testSlot).dirty)

	// The push happens on the tick.
	h.b.flushDirty()
	require.Equal(t, 1, len(pushed))
	require.DeepEqual(t, []uint64{101, 107}, pushedIndices(pushed["partialEmpty"]))

	// The sent attestations fold into the receiving peer's availability.
	got := peerStates["partialEmpty"].Att.Available[3]
	require.NotNil(t, got)
	require.Equal(t, true, hasIdx(got, 101))
	require.Equal(t, true, hasIdx(got, 107))

	// A flush with no new commits pushes nothing.
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).dirty))
	clear(pushed)
	h.b.flushDirty()
	require.Equal(t, 0, len(pushed))
}

// A validator is offered at most once: late mesh joiners get nothing
// retroactively, only genuinely new attestations.
func TestPushSkipsAlreadyBroadcast(t *testing.T) {
	h := newStoreHarness(t)

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"early": {}}
	partial := map[peer.ID]bool{"early": true, "late": true}
	pushed := h.installPush(t, peerStates, partial)

	// The early peer receives the initial attestations; sent now covers them.
	h.deliver(t, testSlot, 101, 107)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{101, 107}, pushedIndices(pushed["early"]))

	// No new validations: publishPartial is not even called.
	peerStates["late"] = blocks.PartialMessagePeerState{}
	clear(pushed)
	invoked := false
	h.b.publishPartial = func(_ string, _ []byte, fn partialmessages.PublishActionsFn[blocks.PartialMessagePeerState]) error {
		invoked = true
		for range fn(peerStates, func(p peer.ID) bool { return partial[p] }) {
			t.Fatal("a peer received a replayed push")
		}
		return nil
	}
	h.b.flushDirty()
	require.Equal(t, false, invoked)

	// Only the new attestation is offered, the late joiner included.
	pushed = h.installPush(t, peerStates, partial)
	h.deliver(t, testSlot, 109)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{109}, pushedIndices(pushed["early"]))
	require.DeepEqual(t, []uint64{109}, pushedIndices(pushed["late"]))
}

// A push of one data group larger than maxAttsPerBundle splits into
// multiple bundles, none oversized, covering every attestation.
func TestPushSplitsOversizedBundles(t *testing.T) {
	h := newStoreHarness(t)

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"peer": {}}
	partial := map[peer.ID]bool{"peer": true}
	pushed := h.installPush(t, peerStates, partial)

	indices := make([]uint64, 2*maxAttsPerBundle+1)
	for i := range indices {
		indices[i] = uint64(i)
	}
	h.deliver(t, testSlot, indices...)
	h.b.flushDirty()

	bundles := pushed["peer"]
	require.Equal(t, 3, len(bundles))
	for _, bundle := range bundles {
		require.Equal(t, true, len(bundle.AttesterIndices) <= maxAttsPerBundle,
			"bundle carries %d attestations", len(bundle.AttesterIndices))
	}
	require.DeepEqual(t, indices, pushedIndices(bundles))
}

// Signature expiry drops the data group but not sent: a late reveal gets its
// own push, old validators are never re-offered.
func TestPushAfterExpiryRevival(t *testing.T) {
	h := newStoreHarness(t)
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"p": {}}
	pushed := h.installPush(t, peerStates, map[peer.ID]bool{"p": true})

	h.deliver(t, testSlot, 101)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{101}, pushedIndices(pushed["p"]))

	// Two heartbeats expire the signature and delete the data group.
	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).attData))

	// A late reveal of a new validator is pushed on its own.
	clear(pushed)
	h.deliver(t, testSlot, 109)
	h.b.flushDirty()
	require.DeepEqual(t, []uint64{109}, pushedIndices(pushed["p"]))
}

// A dirty entry whose data group expired before the flush is skipped.
func TestFlushDirtySkipsExpired(t *testing.T) {
	h := newStoreHarness(t)
	pushed := h.installPush(t, map[peer.ID]blocks.PartialMessagePeerState{"partialEmpty": {}}, map[peer.ID]bool{"partialEmpty": true})

	h.deliver(t, testSlot, 101)
	require.DeepEqual(t, []primitives.ValidatorIndex{101}, h.slotGroup(t, testSlot).dirty)

	// Two heartbeats expire the signature and delete the data group,
	// leaving the dirty entry stale.
	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).attData))

	h.b.flushDirty()
	require.Equal(t, 0, len(pushed))
}

// A submitted attestation commits inline: Submit is a pre-validated ingress
// and never re-enters validation.
func TestSubmitCommitsInline(t *testing.T) {
	h := newStoreHarness(t)

	// Deliver 101,107 to create the data group.
	h.deliver(t, testSlot, 101, 107)
	require.Equal(t, 2, len(h.processed))
	h.slotGroup(t, testSlot).dirty = nil

	h.submit(t, testSlot, 109, idxSig(109))
	require.Equal(t, 2, len(h.processed)) // the process callback never ran for it
	g := h.slotGroup(t, testSlot)
	require.Equal(t, true, hasIdx(g.validated, 109))
	require.DeepEqual(t, idxSig(109), g.validated[109].Signature)
	require.DeepEqual(t, []primitives.ValidatorIndex{109}, g.dirty)
}

// A validated attestation is dropped on Submit: the feedback-loop neutralizer.
func TestSubmitDropsValidatedIndex(t *testing.T) {
	h := newStoreHarness(t)

	h.deliver(t, testSlot, 101)
	h.slotGroup(t, testSlot).dirty = nil

	h.submit(t, testSlot, 101, idxSig(101)) // validator 101 already validated
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).dirty))
	require.Equal(t, 1, len(h.processed))
}

// Submit with no existing group creates it inline; nothing touches the
// validation lane.
func TestSubmitCreatesGroupInline(t *testing.T) {
	h := newStoreHarness(t)

	h.submit(t, testSlot, 101, idxSig(101))
	require.Equal(t, 0, len(h.processed)) // pre-validated: no validation
	require.Equal(t, 0, len(h.b.valJobs))
	require.Equal(t, true, hasIdx(h.slotGroup(t, testSlot).validated, 101))
}

// Submit of an out-of-window slot is dropped before any channel enqueue.
func TestSubmitOutOfWindowDropped(t *testing.T) {
	h := newStoreHarness(t)

	att := &ethpb.SingleAttestation{
		CommitteeId:   3,
		AttesterIndex: 101,
		Data:          testAttData(testSlot + 100), // far in the future
		Signature:     idxSig(101),
	}
	h.b.Submit(testTopic, att)
	require.Equal(t, 0, len(h.b.submit))
}

// A request is answered immediately from live signatures, minus what the
// requester has; the fold makes a replayed request go quiet.
func TestServeRequestsImmediately(t *testing.T) {
	h := newStoreHarness(t)
	h.deliver(t, testSlot, 101, 107)
	h.slotGroup(t, testSlot).dirty = nil

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"req": {}}
	pushed := h.installPush(t, peerStates, map[peer.ID]bool{"req": true})

	// A request for held and unheld validators is answered at once with the
	// held ones; a duplicated index is served once.
	h.deliverMeta(t, "req", nil, []uint64{101, 102, 107, 101})
	require.DeepEqual(t, []uint64{101, 107}, pushedIndices(pushed["req"]))
	got := peerStates["req"].Att.Available[3]
	require.NotNil(t, got)
	require.Equal(t, true, hasIdx(got, 101))
	require.Equal(t, true, hasIdx(got, 107))

	// The served positions folded into the requester's availability: the
	// replayed request goes quiet.
	clear(pushed)
	h.deliverMeta(t, "req", nil, []uint64{101, 107})
	require.Equal(t, 0, len(pushed))
}

// A request against a slot with no live signatures is forgotten at arrival:
// requests are served from live signatures only.
func TestServeRequestsExpiredSlot(t *testing.T) {
	h := newStoreHarness(t)
	h.deliver(t, testSlot, 101)
	h.b.cleanup(*h.current)
	h.b.cleanup(*h.current)
	require.Equal(t, 0, len(h.slotGroup(t, testSlot).attData))

	pushed := h.installPush(t, map[peer.ID]blocks.PartialMessagePeerState{"req": {}}, map[peer.ID]bool{"req": true})
	h.deliverMeta(t, "req", nil, []uint64{101})
	require.Equal(t, 0, len(pushed))
}

// Advertised validators we lack are requested from the advertiser
// immediately; the fetch is oneshot, so a second advertiser is asked too.
func TestFetchAdvertisedIndices(t *testing.T) {
	h := newStoreHarness(t)
	h.deliver(t, testSlot, 101)

	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"adv": {}, "adv2": {}}
	metas := h.installMetaPush(t, peerStates)

	h.deliverMeta(t, "adv", []uint64{101, 102, 103}, nil)
	meta := metas["adv"]
	require.NotNil(t, meta)
	require.DeepEqual(t, []uint64{102, 103}, meta.Requests) // 101 already validated
	require.Equal(t, 0, len(meta.Available))

	// No requested state: a second advertiser is fetched from immediately.
	h.deliverMeta(t, "adv2", []uint64{102, 103}, nil)
	require.NotNil(t, metas["adv2"])
	require.DeepEqual(t, []uint64{102, 103}, metas["adv2"].Requests)
}

// Fetching for a slot we hold nothing of requests everything advertised:
// no data to gate on, no validation-lane involvement.
func TestFetchUnknownSlot(t *testing.T) {
	h := newStoreHarness(t)
	metas := h.installMetaPush(t, map[peer.ID]blocks.PartialMessagePeerState{"adv": {}})

	h.deliverMeta(t, "adv", []uint64{101, 102}, nil)
	require.Equal(t, 0, len(h.b.valJobs))
	require.NotNil(t, metas["adv"])
	require.DeepEqual(t, []uint64{101, 102}, metas["adv"].Requests)
	require.Equal(t, 0, len(metas["adv"].Available))
}

// OnEmitGossip removes the gossip peers from the tracked states and hands
// them to the Start loop; a full channel drops the emission.
func TestOnEmitGossipOneshot(t *testing.T) {
	b := NewBroadcaster(t.Context(), func() primitives.Slot { return testSlot })
	peerStates := map[peer.ID]blocks.PartialMessagePeerState{"g1": {}, "mesh": {}}

	b.OnEmitGossip(testTopic, GroupID(testSlot), []peer.ID{"g1"}, peerStates)
	require.Equal(t, 1, len(peerStates))
	_, meshKept := peerStates["mesh"]
	require.Equal(t, true, meshKept)

	ge := <-b.gossip
	require.Equal(t, testTopic, ge.topic)
	require.Equal(t, testSlot, ge.slot)
	require.DeepEqual(t, []peer.ID{"g1"}, ge.peers)

	// A full channel drops instead of blocking the gossipsub loop.
	for range 5 {
		b.OnEmitGossip(testTopic, GroupID(testSlot), []peer.ID{"g1"}, peerStates)
	}
	require.Equal(t, 3, len(b.gossip))
}

func TestStoreCapsGroupsPerTopic(t *testing.T) {
	h := newStoreHarness(t)
	for s := range maxGroupsPerTopic {
		h.deliver(t, testSlot+primitives.Slot(s), 101)
	}
	require.Equal(t, maxGroupsPerTopic, len(h.b.groups[testTopic]))

	// Past the cap, bundles are still validated but not tracked.
	before := len(h.processed)
	h.deliver(t, testSlot+primitives.Slot(maxGroupsPerTopic), 101)
	require.Equal(t, maxGroupsPerTopic, len(h.b.groups[testTopic]))
	require.Equal(t, before+1, len(h.processed))
}
