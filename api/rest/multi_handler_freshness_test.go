package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

type rootResponse struct {
	Root string `json:"root"`
}

// rootServer serves {"root": rootFn()} after an optional delay, counting hits.
func rootServer(t *testing.T, delay time.Duration, rootFn func() string, hits *int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", api.JsonMediaType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"root":"` + rootFn() + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func acceptRoot(want string) func(json.RawMessage) bool {
	return func(raw json.RawMessage) bool {
		var r rootResponse
		if err := json.Unmarshal(raw, &r); err != nil {
			return false
		}
		return r.Root == want
	}
}

func constRoot(s string) func() string { return func() string { return s } }

// A fast-but-stale node must not win over a slower node that has the fresh head.
func TestMultiHandler_Get_Accept_FreshWinsOverFastStale(t *testing.T) {
	stale := rootServer(t, 0, constRoot("stale"), nil)
	fresh := rootServer(t, 30*time.Millisecond, constRoot("fresh"), nil)

	mh := multi(t, stale.URL, fresh.URL)
	var resp rootResponse
	err := mh.Get(context.Background(), "/x", &resp,
		WithRace(),
		WithAccept(acceptRoot("fresh")),
		WithDeadline(time.Now().Add(2*time.Second)),
	)
	require.NoError(t, err)
	assert.Equal(t, "fresh", resp.Root)
}

// When no node ever matches, the freshest successful body is returned as a
// best-effort fallback (no error).
func TestMultiHandler_Get_Accept_BestAvailableOnDeadline(t *testing.T) {
	stale1 := rootServer(t, 0, constRoot("stale"), nil)
	stale2 := rootServer(t, 0, constRoot("stale"), nil)

	mh := multi(t, stale1.URL, stale2.URL)
	var resp rootResponse
	err := mh.Get(context.Background(), "/x", &resp,
		WithRace(),
		WithAccept(acceptRoot("fresh")),
		WithDeadline(time.Now().Add(100*time.Millisecond)),
	)
	require.NoError(t, err)
	assert.Equal(t, "stale", resp.Root, "should fall back to the freshest available body")
}

// If every node errors, the accept path returns an error.
func TestMultiHandler_Get_Accept_AllFail(t *testing.T) {
	bad1 := jsonServer(t, 0, http.StatusInternalServerError, nil)
	bad2 := jsonServer(t, 0, http.StatusBadGateway, nil)

	mh := multi(t, bad1.URL, bad2.URL)
	var resp rootResponse
	err := mh.Get(context.Background(), "/x", &resp,
		WithRace(),
		WithAccept(acceptRoot("fresh")),
		WithDeadline(time.Now().Add(100*time.Millisecond)),
	)
	require.NotNil(t, err)
}

// A single lagging node is re-polled until it imports the announced head.
func TestMultiHandler_Get_Accept_SingleNodeCatchesUp(t *testing.T) {
	var hits int32
	rootFn := func() string {
		if atomic.LoadInt32(&hits) >= 3 {
			return "fresh"
		}
		return "stale"
	}
	srv := rootServer(t, 0, rootFn, &hits)

	mh := multi(t, srv.URL) // single handler still re-polls via the race path
	var resp rootResponse
	err := mh.Get(context.Background(), "/x", &resp,
		WithRace(),
		WithAccept(acceptRoot("fresh")),
		WithDeadline(time.Now().Add(2*time.Second)),
		WithRepoll(5*time.Millisecond),
	)
	require.NoError(t, err)
	assert.Equal(t, "fresh", resp.Root)
	assert.Equal(t, true, atomic.LoadInt32(&hits) >= 3, "should have re-polled the lagging node across multiple rounds")
}

// Without WithRace, an accept predicate still applies in order: nodes are tried
// in sequence and the first one satisfying accept is returned, even when an
// earlier node succeeded with a non-matching body.
func TestMultiHandler_Get_Accept_InOrderPrefersMatch(t *testing.T) {
	stale := rootServer(t, 0, constRoot("stale"), nil)
	fresh := rootServer(t, 0, constRoot("fresh"), nil)

	mh := multi(t, stale.URL, fresh.URL)
	var resp rootResponse
	err := mh.Get(context.Background(), "/x", &resp,
		WithAccept(acceptRoot("fresh")),
	)
	require.NoError(t, err)
	assert.Equal(t, "fresh", resp.Root, "in-order read should skip the non-matching primary")
}

// Without WithRace and with no node matching accept, the in-order read falls back
// to the first successful response.
func TestMultiHandler_Get_Accept_InOrderFallsBackToFirstSuccess(t *testing.T) {
	first := rootServer(t, 0, constRoot("first"), nil)
	second := rootServer(t, 0, constRoot("second"), nil)

	mh := multi(t, first.URL, second.URL)
	var resp rootResponse
	err := mh.Get(context.Background(), "/x", &resp,
		WithAccept(acceptRoot("fresh")),
	)
	require.NoError(t, err)
	assert.Equal(t, "first", resp.Root, "no match should fall back to the first success")
}

// readUntil returns matched=true and stops as soon as a response satisfies the
// predicate.
func TestReadUntil_Race_ReturnsFirstMatch(t *testing.T) {
	h1 := newTestHandler("http://h1")
	h2 := newTestHandler("http://h2")
	fn := func(_ context.Context, h *handler) (json.RawMessage, error) {
		if h == h2 {
			return json.RawMessage(`{"root":"fresh"}`), nil
		}
		return json.RawMessage(`{"root":"stale"}`), nil
	}
	cfg := getConfig{race: true, pollInterval: time.Millisecond, deadline: time.Now().Add(time.Second)}
	raw, matched, err := readUntil(context.Background(), []*handler{h1, h2}, cfg, acceptRoot("fresh"), raceRound[json.RawMessage], fn)
	require.NoError(t, err)
	assert.Equal(t, true, matched)
	var r rootResponse
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "fresh", r.Root)
}

// With the deadline already passed, exactly one best-effort round runs and the
// freshest successful body is returned with matched=false.
func TestReadUntil_Race_DeadlinePassed_BestEffortSingleRound(t *testing.T) {
	var rounds int32
	h1 := newTestHandler("http://h1")
	fn := func(_ context.Context, _ *handler) (json.RawMessage, error) {
		atomic.AddInt32(&rounds, 1)
		return json.RawMessage(`{"root":"stale"}`), nil
	}
	cfg := getConfig{race: true, pollInterval: time.Second, deadline: time.Now().Add(-time.Second)}
	raw, matched, err := readUntil(context.Background(), []*handler{h1}, cfg, acceptRoot("fresh"), raceRound[json.RawMessage], fn)
	require.NoError(t, err)
	assert.Equal(t, false, matched)
	assert.Equal(t, int32(1), atomic.LoadInt32(&rounds), "deadline in the past should run exactly one round")
	var r rootResponse
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "stale", r.Root)
}

// If every handler errors and nothing matches, the joined error is returned.
func TestReadUntil_Race_AllError(t *testing.T) {
	h1 := newTestHandler("http://h1")
	sentinel := errors.New("boom")
	fn := func(_ context.Context, _ *handler) (json.RawMessage, error) {
		return nil, sentinel
	}
	cfg := getConfig{race: true, pollInterval: time.Millisecond, deadline: time.Now().Add(-time.Second)}
	_, matched, err := readUntil(context.Background(), []*handler{h1}, cfg, acceptRoot("fresh"), raceRound[json.RawMessage], fn)
	require.NotNil(t, err)
	assert.Equal(t, false, matched)
	assert.Equal(t, true, errors.Is(err, sentinel))
}

// A cancelled context returns promptly (never matched). With a realistic fn
// that honors ctx, no success is recorded, so the context error surfaces.
func TestReadUntil_Race_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h1 := newTestHandler("http://h1")
	fn := func(ctx context.Context, _ *handler) (json.RawMessage, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return json.RawMessage(`{"root":"stale"}`), nil
	}
	cfg := getConfig{race: true, pollInterval: time.Second, deadline: time.Now().Add(time.Hour)}
	_, matched, err := readUntil(ctx, []*handler{h1}, cfg, acceptRoot("fresh"), raceRound[json.RawMessage], fn)
	assert.Equal(t, false, matched)
	assert.Equal(t, true, errors.Is(err, context.Canceled))
}

// If a best-available response was recorded before cancellation, it is returned
// (rather than the context error) when the caller later cancels.
func TestReadUntil_Race_ContextCancelReturnsBestAvailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h1 := newTestHandler("http://h1")
	var calls int32
	fn := func(_ context.Context, _ *handler) (json.RawMessage, error) {
		// First round yields a stale (best-available) response; cancel so the
		// next round's drain observes ctx.Done() with a best-available in hand.
		if atomic.AddInt32(&calls, 1) == 1 {
			return json.RawMessage(`{"root":"stale"}`), nil
		}
		cancel()
		return json.RawMessage(`{"root":"stale"}`), nil
	}
	cfg := getConfig{race: true, pollInterval: time.Millisecond, deadline: time.Now().Add(time.Hour)}
	raw, matched, err := readUntil(ctx, []*handler{h1}, cfg, acceptRoot("fresh"), raceRound[json.RawMessage], fn)
	require.NoError(t, err)
	assert.Equal(t, false, matched)
	var r rootResponse
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "stale", r.Root)
}

// In order, WithRepoll re-polls a single lagging node until it matches — repoll
// is decoupled from the race strategy.
func TestReadUntil_InOrder_RepollCatchesUp(t *testing.T) {
	var calls int32
	h1 := newTestHandler("http://h1")
	fn := func(_ context.Context, _ *handler) (json.RawMessage, error) {
		if atomic.AddInt32(&calls, 1) >= 3 {
			return json.RawMessage(`{"root":"fresh"}`), nil
		}
		return json.RawMessage(`{"root":"stale"}`), nil
	}
	cfg := getConfig{pollInterval: time.Millisecond, deadline: time.Now().Add(2 * time.Second)}
	raw, matched, err := readUntil(context.Background(), []*handler{h1}, cfg, acceptRoot("fresh"), inOrderRound[json.RawMessage], fn)
	require.NoError(t, err)
	assert.Equal(t, true, matched)
	var r rootResponse
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "fresh", r.Root)
}

// Without an accept predicate, WithRepoll still re-polls: it retries a node that
// keeps failing until it returns a 2XX. Repoll is orthogonal to WithAccept.
func TestReadUntil_Repoll_NoAccept_RetriesUntilSuccess(t *testing.T) {
	var calls int32
	sentinel := errors.New("boom")
	h1 := newTestHandler("http://h1")
	fn := func(_ context.Context, _ *handler) (json.RawMessage, error) {
		if atomic.AddInt32(&calls, 1) >= 3 {
			return json.RawMessage(`{"root":"ok"}`), nil
		}
		return nil, sentinel
	}
	// acceptAny mirrors the wrapper Get/GetSSZ build when no predicate is set.
	acceptAny := func(json.RawMessage) bool { return true }
	cfg := getConfig{pollInterval: time.Millisecond, deadline: time.Now().Add(2 * time.Second)}
	raw, matched, err := readUntil(context.Background(), []*handler{h1}, cfg, acceptAny, inOrderRound[json.RawMessage], fn)
	require.NoError(t, err)
	assert.Equal(t, true, matched)
	assert.Equal(t, true, atomic.LoadInt32(&calls) >= 3, "should have re-polled past the failures")
	var r rootResponse
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "ok", r.Root)
}

// raceRound returns the first result satisfying accept.
func TestRaceRound_ReturnsFirstMatch(t *testing.T) {
	h1 := newTestHandler("http://h1")
	h2 := newTestHandler("http://h2")
	fn := func(_ context.Context, h *handler) (json.RawMessage, error) {
		if h == h2 {
			return json.RawMessage(`{"root":"fresh"}`), nil
		}
		return json.RawMessage(`{"root":"stale"}`), nil
	}
	raw, matched, ok, _ := raceRound(context.Background(), []*handler{h1, h2}, acceptRoot("fresh"), fn)
	assert.Equal(t, true, matched)
	assert.Equal(t, true, ok)
	var r rootResponse
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "fresh", r.Root)
}

// When no node matches, raceRound reports a best-effort success (matched=false,
// ok=true).
func TestRaceRound_FallsBackWhenNoMatch(t *testing.T) {
	h1 := newTestHandler("http://h1")
	fn := func(_ context.Context, _ *handler) (json.RawMessage, error) {
		return json.RawMessage(`{"root":"stale"}`), nil
	}
	raw, matched, ok, _ := raceRound(context.Background(), []*handler{h1}, acceptRoot("fresh"), fn)
	assert.Equal(t, false, matched)
	assert.Equal(t, true, ok)
	var r rootResponse
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "stale", r.Root)
}

// A fast-but-stale node lets raceRound fall back when ctx is done without waiting
// for a hung node.
func TestRaceRound_DoesNotBlockOnHungHandler(t *testing.T) {
	fast := newTestHandler("http://fast")
	hung := newTestHandler("http://hung")
	fn := func(ctx context.Context, h *handler) (json.RawMessage, error) {
		if h == hung {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return json.RawMessage(`{"root":"stale"}`), nil
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(80*time.Millisecond))
	defer cancel()
	start := time.Now()
	raw, matched, ok, _ := raceRound(ctx, []*handler{fast, hung}, acceptRoot("fresh"), fn)
	assert.Equal(t, false, matched)
	assert.Equal(t, true, ok)
	assert.Equal(t, true, time.Since(start) < time.Second, "must not wait on the hung handler")
	var r rootResponse
	require.NoError(t, json.Unmarshal(raw, &r))
	assert.Equal(t, "stale", r.Root)
}

// If every handler errors, raceRound reports ok=false with the joined error.
func TestRaceRound_AllFail(t *testing.T) {
	h1 := newTestHandler("http://h1")
	sentinel := errors.New("boom")
	fn := func(_ context.Context, _ *handler) (json.RawMessage, error) {
		return nil, sentinel
	}
	_, matched, ok, errs := raceRound(context.Background(), []*handler{h1}, acceptRoot("fresh"), fn)
	assert.Equal(t, false, matched)
	assert.Equal(t, false, ok)
	require.NotNil(t, errors.Join(errs...))
	assert.Equal(t, true, errors.Is(errors.Join(errs...), sentinel))
}

// sszServer serves body as an octet-stream 200.
func sszServer(t *testing.T, body string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", api.OctetStreamMediaType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// GetSSZ with WithSSZAccept prefers the node whose body satisfies the predicate.
func TestMultiHandler_GetSSZ_Accept_PrefersMatch(t *testing.T) {
	stale := sszServer(t, "stale")
	fresh := sszServer(t, "fresh")

	mh := multi(t, stale.URL, fresh.URL)
	body, _, err := mh.GetSSZ(context.Background(), "/x",
		WithRace(),
		WithSSZAccept(func(b []byte, _ http.Header) bool { return string(b) == "fresh" }),
		WithDeadline(time.Now().Add(2*time.Second)),
	)
	require.NoError(t, err)
	assert.Equal(t, "fresh", string(body))
}

// GetSSZ with WithSSZAccept falls back to a successful body when none match.
func TestMultiHandler_GetSSZ_Accept_FallsBackWhenNoMatch(t *testing.T) {
	a := sszServer(t, "stale")
	b := sszServer(t, "stale")

	mh := multi(t, a.URL, b.URL)
	body, _, err := mh.GetSSZ(context.Background(), "/x",
		WithRace(),
		WithSSZAccept(func(bd []byte, _ http.Header) bool { return string(bd) == "fresh" }),
		WithDeadline(time.Now().Add(100*time.Millisecond)),
	)
	require.NoError(t, err)
	assert.Equal(t, "stale", string(body))
}
