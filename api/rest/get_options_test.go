package rest

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestNewGetConfig(t *testing.T) {
	t.Run("defaults accept any 2XX response", func(t *testing.T) {
		cfg := newGetConfig(nil)

		require.Equal(t, false, cfg.race)
		require.Equal(t, time.Duration(0), cfg.pollInterval)
		require.Equal(t, true, cfg.deadline.IsZero())

		// The default acceptance criteria accept anything.
		require.Equal(t, true, cfg.accept(json.RawMessage(`{"anything":true}`)))
		require.Equal(t, true, cfg.sszAccept([]byte("anything"), http.Header{}))
	})

	t.Run("applies options in order", func(t *testing.T) {
		var order []int
		first := func(c *getConfig) { order = append(order, 1) }
		second := func(c *getConfig) { order = append(order, 2) }

		newGetConfig([]GetOption{first, second})

		require.DeepEqual(t, []int{1, 2}, order)
	})
}

func TestWithRace(t *testing.T) {
	cfg := newGetConfig([]GetOption{WithRace()})
	require.Equal(t, true, cfg.race)
}

func TestWithAccept(t *testing.T) {
	t.Run("narrows the acceptance criterion", func(t *testing.T) {
		accept := func(raw json.RawMessage) bool { return string(raw) == "ok" }
		cfg := newGetConfig([]GetOption{WithAccept(accept)})

		require.Equal(t, true, cfg.accept(json.RawMessage("ok")))
		require.Equal(t, false, cfg.accept(json.RawMessage("nope")))
	})

	t.Run("ignores a nil acceptance function", func(t *testing.T) {
		cfg := newGetConfig([]GetOption{WithAccept(nil)})

		// The default (accept anything) is preserved.
		require.Equal(t, true, cfg.accept(json.RawMessage("whatever")))
	})
}

func TestWithSSZAccept(t *testing.T) {
	t.Run("narrows the acceptance criterion", func(t *testing.T) {
		accept := func(body []byte, hdr http.Header) bool {
			return string(body) == "ok" && hdr.Get("X-Test") == "yes"
		}
		cfg := newGetConfig([]GetOption{WithSSZAccept(accept)})

		require.Equal(t, true, cfg.sszAccept([]byte("ok"), http.Header{"X-Test": {"yes"}}))
		require.Equal(t, false, cfg.sszAccept([]byte("ok"), http.Header{}))
		require.Equal(t, false, cfg.sszAccept([]byte("nope"), http.Header{"X-Test": {"yes"}}))
	})

	t.Run("ignores a nil acceptance function", func(t *testing.T) {
		cfg := newGetConfig([]GetOption{WithSSZAccept(nil)})

		// The default (accept anything) is preserved.
		require.Equal(t, true, cfg.sszAccept([]byte("whatever"), http.Header{}))
	})
}

func TestWithDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	cfg := newGetConfig([]GetOption{WithDeadline(deadline)})
	require.Equal(t, deadline, cfg.deadline)
}

func TestResolveOptions(t *testing.T) {
	cfg := ResolveOptions()

	require.Equal(t, false, cfg.Race)
	require.Equal(t, time.Duration(0), cfg.PollInterval)
	require.Equal(t, true, cfg.Deadline.IsZero())

	// The default acceptance criteria accept anything.
	require.Equal(t, true, cfg.Accept(json.RawMessage(`{"anything":true}`)))
	require.Equal(t, true, cfg.SSZAccept([]byte("anything"), http.Header{}))
}

func TestWithRepoll(t *testing.T) {
	t.Run("sets the default interval and the mode", func(t *testing.T) {
		cfg := newGetConfig([]GetOption{WithRepoll(UntilAny2xx)})
		require.Equal(t, defaultPollInterval, cfg.pollInterval)
		require.Equal(t, UntilAny2xx, cfg.repollMode)
	})

	t.Run("sets the UntilAccepted mode", func(t *testing.T) {
		cfg := newGetConfig([]GetOption{WithRepoll(UntilAccepted)})
		require.Equal(t, defaultPollInterval, cfg.pollInterval)
		require.Equal(t, UntilAccepted, cfg.repollMode)
	})
}
