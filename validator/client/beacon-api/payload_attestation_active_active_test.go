package beacon_api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
)

func TestPayloadAttestationFreshnessOptions(t *testing.T) {
	octetHeader := http.Header{"Content-Type": {api.OctetStreamMediaType}}

	t.Run("no hint yields no options", func(t *testing.T) {
		// A ctx without a freshness hint yields no options: the read falls back
		// to its default (first-success) behavior.
		require.Equal(t, true, payloadAttestationFreshnessOptions(context.Background()) == nil)
	})

	for _, tt := range []struct {
		name     string
		deadline time.Time
	}{
		{name: "no due time"},
		{name: "before due", deadline: time.Now().Add(time.Hour)},
		{name: "at due", deadline: time.Now()},
		{name: "after due", deadline: time.Now().Add(-time.Hour)},
	} {
		t.Run(tt.name+" leaves retries and deadline to caller", func(t *testing.T) {
			ctx := iface.WithHint(context.Background(), headHint([32]byte{0xaa}, 10, true, tt.deadline))
			cfg := rest.ResolveOptions(payloadAttestationFreshnessOptions(ctx)...)

			require.Equal(t, true, cfg.Race)
			require.NotNil(t, cfg.SSZAccept)
			require.Equal(t, true, cfg.Deadline.IsZero())
			require.Equal(t, true, cfg.FallbackDeadline.IsZero())
			require.Equal(t, time.Duration(0), cfg.PollInterval)
			require.Equal(t, false, cfg.IndependentRepoll)
		})
	}

	for _, tt := range []struct {
		name         string
		hintDeadline time.Time
		withoutHint  bool
		withoutLimit bool
		withoutRetry bool
	}{
		{name: "past hint deadline", hintDeadline: time.Now().Add(-time.Hour)},
		{name: "future hint deadline", hintDeadline: time.Now().Add(time.Hour)},
		{name: "no hint", withoutHint: true},
		{name: "no caller deadline", withoutLimit: true},
		{name: "no retry policy", withoutRetry: true},
	} {
		t.Run(tt.name+" respects caller retry policy", func(t *testing.T) {
			ctx := context.Background()
			deadline := time.Now().Add(time.Minute)
			if !tt.withoutLimit {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, deadline)
				defer cancel()
			}
			if !tt.withoutHint {
				ctx = iface.WithHint(ctx, headHint([32]byte{0xaa}, 10, true, tt.hintDeadline))
			}
			interval := 37 * time.Millisecond
			if !tt.withoutRetry {
				ctx = iface.WithPayloadAttestationRetry(ctx, interval, func() {})
			}
			cfg := rest.ResolveOptions(payloadAttestationFreshnessOptions(ctx)...)
			polling := !tt.withoutLimit && !tt.withoutRetry
			require.Equal(t, polling, cfg.IndependentRepoll)
			require.Equal(t, true, cfg.FallbackDeadline.IsZero())
			if polling {
				require.Equal(t, deadline, cfg.Deadline)
				require.Equal(t, interval, cfg.PollInterval)
			} else {
				require.Equal(t, true, cfg.Deadline.IsZero())
				require.Equal(t, time.Duration(0), cfg.PollInterval)
			}
		})
	}

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

// payloadAttestationSSZ marshals a PayloadAttestationData whose beacon_block_root is root.
func payloadAttestationSSZ(t *testing.T, root [32]byte) []byte {
	body, err := (&ethpb.PayloadAttestationData{BeaconBlockRoot: root[:]}).MarshalSSZ()
	require.NoError(t, err)
	return body
}
