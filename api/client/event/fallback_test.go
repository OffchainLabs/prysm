package event_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestLegacyTopicFallback(t *testing.T) {
	// head_v2 is swapped for the legacy head topic; other topics are preserved.
	got, ok := event.LegacyTopicFallback([]string{event.EventHeadV2, event.EventExecutionPayloadAvailable})
	require.Equal(t, true, ok)
	require.DeepEqual(t, []string{event.EventHead, event.EventExecutionPayloadAvailable}, got)

	// No head_v2 present: nothing to fall back to.
	got, ok = event.LegacyTopicFallback([]string{event.EventHead, event.EventExecutionPayloadAvailable})
	require.Equal(t, false, ok)
	require.DeepEqual(t, []string{event.EventHead, event.EventExecutionPayloadAvailable}, got)
}
