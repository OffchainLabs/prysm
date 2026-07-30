package beacon_api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/pkg/errors"
)

func TestReadFreshnessOptions(t *testing.T) {
	t.Run("no hint yields no options", func(t *testing.T) {
		// A ctx without a freshness hint yields no options: the read falls back
		// to its default (first-success) behavior.
		require.Equal(t, true, readFreshnessOptions(context.Background(), attestationMatcher) == nil)
	})

	t.Run("hint without deadline sets race and accept only", func(t *testing.T) {
		ctx := iface.WithHint(context.Background(), headHint([32]byte{0xaa}, 10, true, time.Time{}))
		cfg := rest.ResolveOptions(readFreshnessOptions(ctx, attestationMatcher)...)

		// With a zero deadline we get race + accept but neither a deadline nor
		// repolling.
		require.Equal(t, true, cfg.Race)
		require.NotNil(t, cfg.Accept)
		require.Equal(t, true, cfg.Deadline.IsZero())
		require.Equal(t, time.Duration(0), cfg.PollInterval)
	})

	t.Run("accept matches the announced head", func(t *testing.T) {
		want := [32]byte{0x11, 0x22, 0x33}
		other := [32]byte{0x44}

		ctx := iface.WithHint(context.Background(), headHint(want, 10, true, time.Time{}))
		cfg := rest.ResolveOptions(readFreshnessOptions(ctx, attestationMatcher)...)

		require.Equal(t, true, cfg.Accept(attestationDataJSON(want)))       // carries the announced head
		require.Equal(t, false, cfg.Accept(attestationDataJSON(other)))     // carries a different head
		require.Equal(t, false, cfg.Accept(json.RawMessage(`{"data":{}}`))) // root missing
		require.Equal(t, false, cfg.Accept(json.RawMessage(`not json`)))    // unparseable
	})

	t.Run("accept falls back to first-success when head unknown", func(t *testing.T) {
		// When the tracked head is not yet known (ok=false), the accept criterion
		// cannot do better than first-success and must accept any response.
		ctx := iface.WithHint(context.Background(), headHint([32]byte{}, 0, false, time.Time{}))
		cfg := rest.ResolveOptions(readFreshnessOptions(ctx, attestationMatcher)...)

		require.Equal(t, true, cfg.Accept(attestationDataJSON([32]byte{0x99})))
		require.Equal(t, true, cfg.Accept(json.RawMessage(`garbage`)))
	})

	t.Run("accept matches the announced payload status", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 1
		params.OverrideBeaconConfig(cfg)

		gloasSlot := primitives.Slot(cfg.SlotsPerEpoch)
		preGloasSlot := gloasSlot - 1

		want := [32]byte{0x11}

		for _, tt := range []struct {
			name        string
			slot        primitives.Slot
			status      api.PayloadStatus
			wantIndex   uint64
			otherIndex  uint64
			acceptsBoth bool
		}{
			{name: "full accepts index 1 only", slot: gloasSlot, status: api.PayloadStatusFull, wantIndex: 1, otherIndex: 0},
			{name: "empty accepts index 0 only", slot: gloasSlot, status: api.PayloadStatusEmpty, wantIndex: 0, otherIndex: 1},
			{name: "unknown accepts any index", slot: gloasSlot, status: api.PayloadStatusUnknown, wantIndex: 1, otherIndex: 0, acceptsBoth: true},
			{name: "unrecognized accepts any index", slot: gloasSlot, status: api.PayloadStatus("pending"), wantIndex: 1, otherIndex: 0, acceptsBoth: true},
			{name: "pre-Gloas full accepts any index", slot: preGloasSlot, status: api.PayloadStatusFull, wantIndex: 1, otherIndex: 0, acceptsBoth: true},
			{name: "pre-Gloas empty accepts any index", slot: preGloasSlot, status: api.PayloadStatusEmpty, wantIndex: 0, otherIndex: 1, acceptsBoth: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				ctx := iface.WithHint(context.Background(), headHintWithPayload(want, tt.slot, true, time.Time{}, tt.status))
				cfg := rest.ResolveOptions(readFreshnessOptions(ctx, attestationMatcher)...)

				require.Equal(t, true, cfg.Accept(attestationDataJSONWithIndex(want, tt.wantIndex)))
				require.Equal(t, tt.acceptsBoth, cfg.Accept(attestationDataJSONWithIndex(want, tt.otherIndex)))
				// A matching index never rescues a different head.
				require.Equal(t, false, cfg.Accept(attestationDataJSONWithIndex([32]byte{0x44}, tt.wantIndex)))
			})
		}
	})

	t.Run("repolls until the announced head or the deadline", func(t *testing.T) {
		deadline := time.Now().Add(time.Hour)
		ctx := iface.WithHint(context.Background(), headHint([32]byte{0x01}, 10, true, deadline))

		for _, matcher := range []func(json.RawMessage, iface.Head) bool{attestationMatcher, syncCommitteeMatcher} {
			cfg := rest.ResolveOptions(readFreshnessOptions(ctx, matcher)...)
			require.Equal(t, true, cfg.Race)
			require.Equal(t, deadline, cfg.Deadline)
			// WithRepoll uses the default (non-zero) poll interval.
			require.NotEqual(t, time.Duration(0), cfg.PollInterval)
			require.Equal(t, rest.UntilAccepted, cfg.RepollMode)
		}
	})

	t.Run("a past deadline is floored so a lagging node still gets time", func(t *testing.T) {
		// A deadline already in the past would leave no budget; it is raised to
		// now + readFreshnessBudget.
		ctx := iface.WithHint(context.Background(), headHint([32]byte{0x01}, 10, true, time.Now().Add(-time.Hour)))

		before := time.Now()
		cfg := rest.ResolveOptions(readFreshnessOptions(ctx, attestationMatcher)...)
		after := time.Now()

		require.Equal(t, true, cfg.Deadline.After(before.Add(readFreshnessBudget-time.Second)))
		require.Equal(t, true, cfg.Deadline.Before(after.Add(readFreshnessBudget+time.Second)))
	})
}

func TestBlockFreshnessOptions(t *testing.T) {
	// decodeWithParent returns a decode func yielding a block whose parent root is root.
	decodeWithParent := func(root [32]byte) func([]byte, http.Header) (*ethpb.GenericBeaconBlock, error) {
		return func([]byte, http.Header) (*ethpb.GenericBeaconBlock, error) {
			return genericBlockWithParent(root), nil
		}
	}

	t.Run("no hint yields no options", func(t *testing.T) {
		// A ctx without a freshness hint yields no options: the read falls back
		// to its default (first-success) behavior.
		require.Equal(t, true, blockFreshnessOptions(context.Background(), decodeWithParent([32]byte{})) == nil)
	})

	t.Run("sets race, ssz-accept, repolls until any 2xx and uses the caller deadline", func(t *testing.T) {
		deadline := time.Now().Add(time.Minute)
		base, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()
		ctx := iface.WithHint(base, headHint([32]byte{0xaa}, 10, true, time.Time{}))
		cfg := rest.ResolveOptions(blockFreshnessOptions(ctx, decodeWithParent([32]byte{0xaa}))...)

		require.Equal(t, true, cfg.Race)
		require.NotNil(t, cfg.SSZAccept)
		// Re-poll until at least one node returns a block (any 2xx).
		require.NotEqual(t, time.Duration(0), cfg.PollInterval)
		require.Equal(t, rest.UntilAny2xx, cfg.RepollMode)
		// The read is bounded by the caller's context deadline (the slot deadline).
		require.Equal(t, deadline, cfg.Deadline)
	})

	t.Run("no caller deadline yields no read deadline", func(t *testing.T) {
		// Without a deadline on the caller context there is nothing to bound the
		// re-polling, so no read deadline is set.
		ctx := iface.WithHint(context.Background(), headHint([32]byte{0xaa}, 10, true, time.Time{}))
		cfg := rest.ResolveOptions(blockFreshnessOptions(ctx, decodeWithParent([32]byte{0xaa}))...)

		require.Equal(t, true, cfg.Deadline.IsZero())
	})

	t.Run("accept matches the announced head against the block parent root", func(t *testing.T) {
		want := [32]byte{0x11, 0x22, 0x33}
		other := [32]byte{0x44}

		ctx := iface.WithHint(context.Background(), headHint(want, 10, true, time.Time{}))

		accepting := rest.ResolveOptions(blockFreshnessOptions(ctx, decodeWithParent(want))...)
		require.Equal(t, true, accepting.SSZAccept(nil, http.Header{})) // block builds on the announced head

		rejecting := rest.ResolveOptions(blockFreshnessOptions(ctx, decodeWithParent(other))...)
		require.Equal(t, false, rejecting.SSZAccept(nil, http.Header{})) // block builds on a different head
	})

	t.Run("accept rejects an undecodable or nil block", func(t *testing.T) {
		ctx := iface.WithHint(context.Background(), headHint([32]byte{0x11}, 10, true, time.Time{}))

		decodeErr := func([]byte, http.Header) (*ethpb.GenericBeaconBlock, error) {
			return nil, errors.New("boom")
		}
		errCfg := rest.ResolveOptions(blockFreshnessOptions(ctx, decodeErr)...)
		require.Equal(t, false, errCfg.SSZAccept(nil, http.Header{}))

		decodeNilBlock := func([]byte, http.Header) (*ethpb.GenericBeaconBlock, error) {
			return &ethpb.GenericBeaconBlock{}, nil
		}
		nilCfg := rest.ResolveOptions(blockFreshnessOptions(ctx, decodeNilBlock)...)
		require.Equal(t, false, nilCfg.SSZAccept(nil, http.Header{}))
	})

	t.Run("accept falls back to first-success when head unknown", func(t *testing.T) {
		// When the tracked head is not yet known (ok=false), the accept criterion
		// accepts any response without even decoding it.
		ctx := iface.WithHint(context.Background(), headHint([32]byte{}, 0, false, time.Time{}))

		decodeShouldNotRun := func([]byte, http.Header) (*ethpb.GenericBeaconBlock, error) {
			t.Fatal("decode must not run when the head is unknown")
			return nil, nil
		}
		cfg := rest.ResolveOptions(blockFreshnessOptions(ctx, decodeShouldNotRun)...)
		require.Equal(t, true, cfg.SSZAccept(nil, http.Header{}))
	})

	t.Run("deadline comes from the caller context, not the hint", func(t *testing.T) {
		ctxDeadline := time.Now().Add(time.Minute)
		base, cancel := context.WithDeadline(context.Background(), ctxDeadline)
		defer cancel()
		hintDeadline := time.Now().Add(time.Second)
		ctx := iface.WithHint(base, headHint([32]byte{0x01}, 10, true, hintDeadline))

		cfg := rest.ResolveOptions(blockFreshnessOptions(ctx, decodeWithParent([32]byte{0x01}))...)
		require.Equal(t, ctxDeadline, cfg.Deadline)
		require.Equal(t, hintDeadline.Add(-blockPublishMargin), cfg.FallbackDeadline)
	})
}

func TestPayloadAttestationFreshnessOptions(t *testing.T) {
	octetHeader := http.Header{"Content-Type": {api.OctetStreamMediaType}}

	t.Run("no hint yields no options", func(t *testing.T) {
		// A ctx without a freshness hint yields no options: the read falls back
		// to its default (first-success) behavior.
		require.Equal(t, true, payloadAttestationFreshnessOptions(context.Background()) == nil)
	})

	t.Run("hint without deadline sets race and ssz-accept only", func(t *testing.T) {
		ctx := iface.WithHint(context.Background(), headHint([32]byte{0xaa}, 10, true, time.Time{}))
		cfg := rest.ResolveOptions(payloadAttestationFreshnessOptions(ctx)...)

		// With a zero deadline we get race + ssz-accept but neither a deadline nor repolling.
		require.Equal(t, true, cfg.Race)
		require.NotNil(t, cfg.SSZAccept)
		require.Equal(t, true, cfg.Deadline.IsZero())
		require.Equal(t, time.Duration(0), cfg.PollInterval)
	})

	t.Run("hint with a deadline sets that deadline and repolls", func(t *testing.T) {
		deadline := time.Now().Add(time.Hour)
		ctx := iface.WithHint(context.Background(), headHint([32]byte{0x01}, 10, true, deadline))
		cfg := rest.ResolveOptions(payloadAttestationFreshnessOptions(ctx)...)

		require.Equal(t, true, cfg.Race)
		require.Equal(t, deadline, cfg.Deadline)
		// WithRepoll(0) falls back to the default poll interval.
		require.NotEqual(t, time.Duration(0), cfg.PollInterval)
	})

	t.Run("a past deadline is floored so a lagging node still gets time", func(t *testing.T) {
		// A deadline already in the past would leave no budget; it is raised to
		// now + readFreshnessBudget.
		ctx := iface.WithHint(context.Background(), headHint([32]byte{0x01}, 10, true, time.Now().Add(-time.Hour)))

		before := time.Now()
		cfg := rest.ResolveOptions(payloadAttestationFreshnessOptions(ctx)...)
		after := time.Now()

		require.Equal(t, true, cfg.Deadline.After(before.Add(readFreshnessBudget-time.Second)))
		require.Equal(t, true, cfg.Deadline.Before(after.Add(readFreshnessBudget+time.Second)))
	})

	t.Run("accept matches the announced head against an SSZ response", func(t *testing.T) {
		want := [32]byte{0x11, 0x22, 0x33}
		other := [32]byte{0x44}

		ctx := iface.WithHint(context.Background(), headHint(want, 10, true, time.Time{}))
		cfg := rest.ResolveOptions(payloadAttestationFreshnessOptions(ctx)...)

		require.Equal(t, true, cfg.SSZAccept(payloadAttestationSSZ(t, want), octetHeader))
		require.Equal(t, false, cfg.SSZAccept(payloadAttestationSSZ(t, other), octetHeader))
		require.Equal(t, false, cfg.SSZAccept([]byte("not ssz"), octetHeader))
	})

	t.Run("accept falls back to first-success when head unknown", func(t *testing.T) {
		// When the tracked head is not yet known (ok=false), the accept criterion
		// accepts any response without inspecting it.
		ctx := iface.WithHint(context.Background(), headHint([32]byte{}, 0, false, time.Time{}))
		cfg := rest.ResolveOptions(payloadAttestationFreshnessOptions(ctx)...)

		require.Equal(t, true, cfg.SSZAccept(payloadAttestationSSZ(t, [32]byte{0x99}), octetHeader))
		require.Equal(t, true, cfg.SSZAccept([]byte("garbage"), http.Header{}))
	})
}

func TestPayloadAttestationBeaconBlockRoot(t *testing.T) {
	root := [32]byte{0x11, 0x22, 0x33}

	t.Run("decodes a JSON response when the content type is not octet-stream", func(t *testing.T) {
		for _, hdr := range []http.Header{
			{},                                     // no content type
			{"Content-Type": {"application/json"}}, // explicit JSON
		} {
			got, ok := payloadAttestationBeaconBlockRoot(attestationDataJSON(root), hdr)
			require.Equal(t, true, ok)
			require.Equal(t, root, got)
		}
	})

	t.Run("rejects a JSON response whose root is not a valid 32-byte hex", func(t *testing.T) {
		// Present and non-empty, but too short to decode into a 32-byte root.
		_, ok := payloadAttestationBeaconBlockRoot([]byte(`{"data":{"beacon_block_root":"0x1234"}}`), http.Header{})
		require.Equal(t, false, ok)
	})
}

// headHint returns a Hint whose Head resolver reports the given root/slot/ok and
// whose deadline is the given time.
func headHint(root [32]byte, slot primitives.Slot, ok bool, deadline time.Time) iface.Hint {
	return headHintWithPayload(root, slot, ok, deadline, api.PayloadStatusUnknown)
}

// headHintWithPayload is headHint with an announced payload status.
func headHintWithPayload(root [32]byte, slot primitives.Slot, ok bool, deadline time.Time, status api.PayloadStatus) iface.Hint {
	head := iface.Head{Root: root, Slot: slot, PayloadStatus: status}

	return iface.Hint{
		Head:     func() (iface.Head, bool) { return head, ok },
		Deadline: deadline,
	}
}

// attestationDataJSON builds a minimal produce-attestation-data JSON body whose
// beacon_block_root is root.
func attestationDataJSON(root [32]byte) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"data":{"beacon_block_root":"%#x"}}`, root))
}

// attestationDataJSONWithIndex is attestationDataJSON carrying an index, which
// post-Gloas signals the payload status of the attested head.
func attestationDataJSONWithIndex(root [32]byte, index uint64) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"data":{"beacon_block_root":"%#x","index":"%d"}}`, root, index))
}

// payloadAttestationSSZ marshals a PayloadAttestationData whose beacon_block_root is root.
func payloadAttestationSSZ(t *testing.T, root [32]byte) []byte {
	body, err := (&ethpb.PayloadAttestationData{BeaconBlockRoot: root[:]}).MarshalSSZ()
	require.NoError(t, err)
	return body
}

// genericBlockWithParent returns a Phase0 GenericBeaconBlock whose parent root is root.
func genericBlockWithParent(root [32]byte) *ethpb.GenericBeaconBlock {
	blk := util.NewBeaconBlock()
	blk.Block.ParentRoot = root[:]
	return &ethpb.GenericBeaconBlock{Block: &ethpb.GenericBeaconBlock_Phase0{Phase0: blk.Block}}
}
