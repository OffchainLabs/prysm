package iface

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestWithHint(t *testing.T) {
	t.Run("round-trips a hint through the context", func(t *testing.T) {
		want := [32]byte{0x11, 0x22, 0x33}
		wantSlot := primitives.Slot(42)
		deadline := time.Now().Add(time.Hour)

		hint := Hint{
			Head:     func() (Head, bool) { return Head{Root: want, Slot: wantSlot}, true },
			Deadline: deadline,
		}
		ctx := WithHint(context.Background(), hint)

		got, ok := FromContext(ctx)
		require.Equal(t, true, ok)
		require.Equal(t, deadline, got.Deadline)

		gotHead, gotOK := got.Head()
		require.Equal(t, want, gotHead.Root)
		require.Equal(t, wantSlot, gotHead.Slot)
		require.Equal(t, true, gotOK)
	})

	t.Run("ignores a hint with a nil Head", func(t *testing.T) {
		// A hint without a head resolver carries no expectation, so the context
		// is returned unchanged and FromContext reports no hint.
		ctx := context.Background()
		got := WithHint(ctx, Hint{Deadline: time.Now()})
		require.Equal(t, ctx, got)

		_, ok := FromContext(got)
		require.Equal(t, false, ok)
	})
}

func TestFromContext(t *testing.T) {
	_, ok := FromContext(context.Background())
	require.Equal(t, false, ok)
}
