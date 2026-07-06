package beacon_api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
)

func root32(b byte) [32]byte {
	var r [32]byte
	for i := range r {
		r[i] = b
	}
	return r
}

func TestAttestationHeadExtractor(t *testing.T) {
	rootHex := "0x" + strings.Repeat("ab", 32)
	// data.slot is the requested duty slot, not the head slot, so it is ignored;
	// only the beacon_block_root is extracted.
	body, err := json.Marshal(map[string]any{
		"data": map[string]any{"beacon_block_root": rootHex, "slot": "77"},
	})
	require.NoError(t, err)

	root, ok := attestationRootExtractor.extract(body)
	assert.Equal(t, true, ok)
	assert.Equal(t, root32(0xab), root)

	// Garbage body yields ok=false and a zero root (never matches a real head).
	_, ok = attestationRootExtractor.extract(json.RawMessage(`{"data":{}}`))
	assert.Equal(t, false, ok)
}

func TestSyncBlockRootExtractor(t *testing.T) {
	rootHex := "0x" + strings.Repeat("cd", 32)
	body, err := json.Marshal(map[string]any{"data": map[string]any{"root": rootHex}})
	require.NoError(t, err)

	root, ok := syncBlockRootExtractor.extract(body)
	assert.Equal(t, true, ok)
	assert.Equal(t, root32(0xcd), root)

	_, ok = syncBlockRootExtractor.extract(json.RawMessage(`{"data":{}}`))
	assert.Equal(t, false, ok)
}

func TestFreshnessOptions_NoHintIsFirstSuccess(t *testing.T) {
	opts := readFreshnessOptions(context.Background(), attestationRootExtractor)
	assert.Equal(t, 0, len(opts), "no hint => no options => unchanged first-success behavior")
}

func TestBlockFreshnessOptions_NoHintIsFirstSuccess(t *testing.T) {
	decode := func([]byte, http.Header) (*ethpb.GenericBeaconBlock, error) { return nil, nil }
	opts := blockFreshnessOptions(context.Background(), decode)
	assert.Equal(t, 0, len(opts), "no hint => no options => unchanged first-success behavior")
}

func TestBlockFreshnessOptions_WithHintBuildsOptions(t *testing.T) {
	decode := func([]byte, http.Header) (*ethpb.GenericBeaconBlock, error) { return nil, nil }
	head := func() ([32]byte, primitives.Slot, bool) { return root32(0xaa), 10, true }
	ctx := iface.WithHint(context.Background(), iface.Hint{Head: head, Deadline: time.Now().Add(time.Hour)})
	opts := blockFreshnessOptions(ctx, decode)
	assert.Equal(t, 3, len(opts), "hint => race + ssz accept + deadline options")
}

func TestFreshnessOptions_WithHintBuildsOptions(t *testing.T) {
	head := func() ([32]byte, primitives.Slot, bool) { return root32(0xaa), 10, true }
	ctx := iface.WithHint(context.Background(), iface.Hint{Head: head, Deadline: time.Now().Add(time.Second)})

	opts := readFreshnessOptions(ctx, attestationRootExtractor)
	assert.Equal(t, 4, len(opts), "polling hint with deadline => race + accept + deadline + repoll options")

	// A non-polling extractor still bounds the single round with a deadline so a
	// slow node cannot stall the read, but does not re-poll.
	optsNoPoll := readFreshnessOptions(ctx, syncBlockRootExtractor)
	assert.Equal(t, 3, len(optsNoPoll), "single-round read => race + accept + deadline options")

	// Without a deadline, only the race + accept options are produced.
	ctxNoDeadline := iface.WithHint(context.Background(), iface.Hint{Head: head})
	optsNoDeadline := readFreshnessOptions(ctxNoDeadline, attestationRootExtractor)
	assert.Equal(t, 2, len(optsNoDeadline))
}

// The poll path is invoked at the attestation-due time, so its hint deadline is
// effectively already elapsed. It must still emit a bounded (floored) deadline
// plus repoll rather than racing against a dead context; the floor itself is
// exercised end-to-end by the rest package's readUntil tests.
func TestFreshnessOptions_PollPastDeadlineStillBounded(t *testing.T) {
	head := func() ([32]byte, primitives.Slot, bool) { return root32(0xaa), 10, true }
	ctx := iface.WithHint(context.Background(), iface.Hint{Head: head, Deadline: time.Now().Add(-time.Second)})

	opts := readFreshnessOptions(ctx, attestationRootExtractor)
	assert.Equal(t, 4, len(opts), "an already-elapsed poll deadline must still yield race + accept + (floored) deadline + repoll")
}
