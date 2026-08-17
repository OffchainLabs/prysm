package client

import (
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
)

// latestHead returns the head currently tracked by h.
func latestHead(h *headTracker) (iface.Head, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.head, h.set
}

// hexRoot builds a valid 0x-prefixed 32-byte root whose every byte is b.
func hexRoot(b byte) string {
	const digits = "0123456789abcdef"
	return "0x" + strings.Repeat(string([]byte{digits[b>>4], digits[b&0x0f]}), 32)
}

func TestHeadTracker_Update(t *testing.T) {
	t.Run("first update records the head", func(t *testing.T) {
		h := newHeadTracker()
		require.NoError(t, h.update(42, hexRoot(0xab), api.PayloadStatusUnknown))

		head, ok := latestHead(h)
		require.Equal(t, true, ok)
		require.Equal(t, primitives.Slot(42), head.Slot)
		require.Equal(t, byte(0xab), head.Root[0])
	})

	t.Run("a higher slot overwrites the head", func(t *testing.T) {
		h := newHeadTracker()
		require.NoError(t, h.update(42, hexRoot(0xab), api.PayloadStatusUnknown))
		require.NoError(t, h.update(43, hexRoot(0xcd), api.PayloadStatusUnknown))

		head, _ := latestHead(h)
		require.Equal(t, primitives.Slot(43), head.Slot)
		require.Equal(t, byte(0xcd), head.Root[0])
	})

	t.Run("a lower slot is ignored", func(t *testing.T) {
		h := newHeadTracker()
		require.NoError(t, h.update(42, hexRoot(0xab), api.PayloadStatusUnknown))
		require.NoError(t, h.update(41, hexRoot(0xcd), api.PayloadStatusUnknown))

		head, _ := latestHead(h)
		require.Equal(t, primitives.Slot(42), head.Slot)
		require.Equal(t, byte(0xab), head.Root[0]) // unchanged
	})

	t.Run("an empty root (gRPC head event) is a no-op without error", func(t *testing.T) {
		h := newHeadTracker()
		require.NoError(t, h.update(42, "", api.PayloadStatusUnknown))

		_, ok := latestHead(h)
		require.Equal(t, false, ok)
	})

	t.Run("the announced payload status is recorded with the head", func(t *testing.T) {
		h := newHeadTracker()
		require.NoError(t, h.update(42, hexRoot(0xab), api.PayloadStatusFull))

		head, _ := latestHead(h)
		require.Equal(t, api.PayloadStatusFull, head.PayloadStatus)

		// A newer head carries its own status rather than inheriting the old one.
		require.NoError(t, h.update(43, hexRoot(0xcd), api.PayloadStatusEmpty))

		head, _ = latestHead(h)
		require.Equal(t, api.PayloadStatusEmpty, head.PayloadStatus)
	})
}

func TestWithHeadHint(t *testing.T) {
	component := params.BeaconConfig().AttestationDueBPS

	v := &validator{head: newHeadTracker(), genesisTime: time.Unix(0, 0)}
	const slot = primitives.Slot(7)
	require.NoError(t, v.head.update(slot, hexRoot(0xab), api.PayloadStatusFull))

	ctx, err := v.withHeadHint(t.Context(), slot, component)
	require.NoError(t, err)

	hint, ok := iface.FromContext(ctx)
	require.Equal(t, true, ok)

	wantDeadline, err := v.slotComponentDeadline(slot, component)
	require.NoError(t, err)
	require.Equal(t, wantDeadline, hint.Deadline)

	head, set := hint.Head()
	require.Equal(t, true, set)
	require.Equal(t, slot, head.Slot)
	require.Equal(t, byte(0xab), head.Root[0])
	require.Equal(t, api.PayloadStatusFull, head.PayloadStatus)

}

func TestWithPayloadHeadHint(t *testing.T) {
	component := params.BeaconConfig().PayloadAttestationDueBPS

	t.Run("attaches the announced payload root and the component deadline", func(t *testing.T) {
		v := &validator{payloadAvailability: newPayloadAvailability(), genesisTime: time.Unix(0, 0)}
		const slot = primitives.Slot(7)
		root := [32]byte{0xab}
		v.payloadAvailability.notify(slot, &root)

		ctx, err := v.withPayloadHeadHint(t.Context(), slot)
		require.NoError(t, err)

		hint, ok := iface.FromContext(ctx)
		require.Equal(t, true, ok)

		wantDeadline, err := v.slotComponentDeadline(slot, component)
		require.NoError(t, err)
		require.Equal(t, wantDeadline, hint.Deadline)

		gotHead, set := hint.Head()
		require.Equal(t, true, set)
		require.Equal(t, slot, gotHead.Slot)
		require.Equal(t, root, gotHead.Root)
		require.Equal(t, api.PayloadStatusFull, gotHead.PayloadStatus)
	})
}
