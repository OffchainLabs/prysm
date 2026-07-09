package doublylinkedtree

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestPoC_PreviousSlotPayloadDecision_IgnoresPTC demonstrates the fork-choice
// deviation described in the Gloas spec's `get_weight` /
// `is_previous_slot_payload_decision`.
//
// Spec (specs/gloas/fork-choice.md):
//   - get_weight returns Gwei(0) for the EMPTY/FULL nodes of a block from the
//     *previous* slot (is_previous_slot_payload_decision == true).
//   - With both weights zeroed, get_head decides EMPTY vs FULL purely via
//     get_payload_status_tiebreaker, i.e. should_extend_payload (the PTC signal).
//
// Prysm (choosePayloadContent, gloas.go): compares the raw en.weight vs fn.weight
// FIRST and only consults shouldExtendPayload when they are *exactly* equal.
//
// Scenario (mirrors the spec's test_get_head_full_payload_tiebreak, but with the
// EMPTY side made strictly heavier by attestations):
//
//	         A (slot 32, payload delivered, PTC quorum: timely + available)
//	        / \
//	 A(empty)   A(full)
//	     |         |
//	   B(s33)    C(s33)      current slot = 33  => A is the "previous slot"
//	3 votes      2 votes     => emptyA.weight = 30 > fullA.weight = 20
//
// Because PTC quorum is met, should_extend_payload(A) == true, so a spec-compliant
// client zeroes both A weights and the tiebreaker selects FULL(A) => head == C.
// Prysm instead sees emptyA.weight(30) > fullA.weight(20) and selects EMPTY(A) => head == B.
func TestPoC_PreviousSlotPayloadDecision_IgnoresPTC(t *testing.T) {
	f := setupGloas(t, 1, 1)
	s := f.store
	ctx := t.Context()

	balances := make([]uint64, 64)
	for i := range balances {
		balances[i] = 10
	}
	f.justifiedBalances = balances
	s.committeeWeight = uint64(len(balances)*10) / uint64(params.BeaconConfig().SlotsPerEpoch)
	zeroHash := params.BeaconConfig().ZeroHash

	// --- Block A at slot 32 with a delivered execution payload. ---
	slotA := primitives.Slot(32)
	rootA := indexToHash(1)
	blockHashA := indexToHash(100)
	driftGenesisTime(f, slotA, 0)
	st, blk, err := prepareGloasForkchoiceState(ctx, slotA, rootA, zeroHash, blockHashA, zeroHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	pe, err := prepareGloasForkchoicePayload(rootA)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	// PTC reaches quorum for A's payload: timely AND data available.
	// => should_extend_payload(A) == true (the payload should be extended / FULL).
	emptyA := s.emptyNodeByRoot[rootA]
	for i := range uint64(fieldparams.PTCSize) {
		emptyA.node.payloadAvailabilityVote.SetBitAt(i, true)
		emptyA.node.payloadDataAvailabilityVote.SetBitAt(i, true)
	}

	// --- Advance to slot 33: now A (slot 32) is the "previous slot" block. ---
	slotB := slotA + 1
	driftGenesisTime(f, slotB, 0)
	require.NoError(t, f.NewSlot(ctx, slotB))

	// B (slot 33) builds on EMPTY(A): its bid parent hash != blockHashA.
	rootB := indexToHash(2)
	blockHashB := indexToHash(200)
	nonMatchingHash := indexToHash(999)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotB, rootB, rootA, blockHashB, nonMatchingHash, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	// C (slot 33) builds on FULL(A): its bid parent hash == blockHashA.
	rootC := indexToHash(3)
	blockHashC := indexToHash(300)
	st, blk, err = prepareGloasForkchoiceState(ctx, slotB, rootC, rootA, blockHashC, blockHashA, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))

	// Give the EMPTY(A) side strictly more attestation weight than the FULL(A) side:
	//   3 validators attest B (on empty A) -> emptyA.weight = 30
	//   2 validators attest C (on full A)  -> fullA.weight  = 20
	f.ProcessAttestation(ctx, []uint64{0, 1, 2}, rootB, slotB, false)
	f.ProcessAttestation(ctx, []uint64{3, 4}, rootC, slotB, false)

	headRoot, err := f.Head(ctx)
	require.NoError(t, err)

	fullA := s.fullNodeByRoot[rootA]

	// Preconditions of the divergence:
	//  (1) A is the previous-slot payload decision.
	require.Equal(t, true, emptyA.node.slot+1 == s.currentSlot())
	//  (2) PTC quorum => the payload should be extended (spec prefers FULL).
	require.Equal(t, true, s.shouldExtendPayload(fullA))
	//  (3) yet the EMPTY side carries strictly MORE attestation weight.
	require.Equal(t, uint64(30), emptyA.weight)
	require.Equal(t, uint64(20), fullA.weight)

	// ---- The deviation ----
	// Spec: get_weight zeroes both emptyA and fullA (previous-slot payload decision),
	// so get_payload_status_tiebreaker decides. should_extend_payload == true ranks
	// FULL(2) above EMPTY(1) => head is the FULL(A) subtree => rootC.
	//
	// Prysm: choosePayloadContent returns EMPTY(A) because emptyA.weight(30) >
	// fullA.weight(20); shouldExtendPayload is never consulted => head is rootB.
	require.Equal(t, rootC, headRoot) // spec-compliant: FULL path

	if headRoot != rootC {
		t.Fatalf("still diverges from spec: got %#x, want FULL(A) root %#x", headRoot, rootC)
	}
}
