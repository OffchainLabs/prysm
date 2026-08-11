package rest

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

type testResponse struct {
	Host string `json:"host"`
}

func TestNewMultiHandler(t *testing.T) {
	_, err := newMultiHandler(nil)
	require.ErrorContains(t, "at least one handler", err)
}

func TestHost(t *testing.T) {
	t.Run("single endpoint", func(t *testing.T) {
		mh := multi(t, "http://first:3500")
		require.Equal(t, "http://first:3500", mh.Host())
	})

	t.Run("all endpoints, comma-separated", func(t *testing.T) {
		mh := multi(t, "http://first:3500", "http://second:3500")
		require.Equal(t, "http://first:3500,http://second:3500", mh.Host())
	})

	t.Run("endpoints are raw, redaction is the caller's job", func(t *testing.T) {
		mh := multi(t, "http://user:password@first:3500", "http://second:3500")
		require.Equal(t, "http://user:password@first:3500,http://second:3500", mh.Host())
		require.Equal(t, "http://user:xxxxx@first:3500,http://second:3500", api.RedactEndpointList(mh.Host()))
	})
}

func TestMultiHandlerGet(t *testing.T) {
	t.Run("primary serves without fanout", func(t *testing.T) {
		var primaryHits, secondaryHits int32
		primary := jsonServer(t, 0, http.StatusOK, &primaryHits)
		secondary := jsonServer(t, 0, http.StatusOK, &secondaryHits)

		mh := multi(t, primary.URL, secondary.URL)
		var resp testResponse
		require.NoError(t, mh.Get(context.Background(), "/x", &resp))
		assert.NotEqual(t, "", resp.Host)
		assert.Equal(t, int32(1), atomic.LoadInt32(&primaryHits), "primary should serve the read")
		assert.Equal(t, int32(0), atomic.LoadInt32(&secondaryHits), "secondary should not be contacted while the primary is healthy")
	})

	t.Run("nil response discards the body but still contacts the node", func(t *testing.T) {
		var hits int32
		srv := jsonServer(t, 0, http.StatusOK, &hits)

		mh := multi(t, srv.URL)
		require.NoError(t, mh.Get(context.Background(), "/x", nil))
		assert.Equal(t, int32(1), atomic.LoadInt32(&hits), "the node should be queried even when the body is discarded")
	})
}

func TestMultiHandlerGetSSZ(t *testing.T) {
	srv := sszServer(t, "payload")

	mh := multi(t, srv.URL)
	body, header, err := mh.GetSSZ(context.Background(), "/x")
	require.NoError(t, err)
	assert.Equal(t, "payload", string(body))
	assert.Equal(t, api.OctetStreamMediaType, header.Get("Content-Type"))
}

func TestMultiHandlerGetStatusCode(t *testing.T) {
	t.Run("returns 200 when any node is ready", func(t *testing.T) {
		syncing := jsonServer(t, 0, http.StatusPartialContent, nil) // 206
		ready := jsonServer(t, 0, http.StatusOK, nil)               // 200

		mh := multi(t, syncing.URL, ready.URL)
		code, err := mh.GetStatusCode(context.Background(), "/eth/v1/node/health")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, code)
	})

	t.Run("returns a non-200 status when no node is ready", func(t *testing.T) {
		syncing := jsonServer(t, 0, http.StatusPartialContent, nil)         // 206
		unavailable := jsonServer(t, 0, http.StatusServiceUnavailable, nil) // 503

		mh := multi(t, syncing.URL, unavailable.URL)
		code, err := mh.GetStatusCode(context.Background(), "/eth/v1/node/health")
		require.NoError(t, err)
		assert.NotEqual(t, http.StatusOK, code)
		assert.Equal(t, true, code == http.StatusPartialContent || code == http.StatusServiceUnavailable,
			"should report the last non-200 status observed")
	})

	t.Run("returns an error when every node fails at the transport level", func(t *testing.T) {
		const deadHost = "http://127.0.0.1:1" // connection refused

		mh := multi(t, deadHost, deadHost)
		code, err := mh.GetStatusCode(context.Background(), "/eth/v1/node/health")
		require.NotNil(t, err)
		assert.Equal(t, 0, code)
	})

	t.Run("returns the context error when canceled before any node responds", func(t *testing.T) {
		slow := jsonServer(t, 30*time.Second, http.StatusOK, nil) // won't respond during the test
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		mh := multi(t, slow.URL)

		// Cancel once GetStatusCode is blocked waiting for a result, so its select
		// observes ctx.Done() before the (cancellation-induced) result arrives.
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		code, err := mh.GetStatusCode(ctx, "/eth/v1/node/health")
		require.NotNil(t, err)
		assert.Equal(t, 0, code)
	})
}

func TestMultiHandlerPost(t *testing.T) {
	const body = `{"a":1}`

	t.Run("single handler short-circuits to the sole node", func(t *testing.T) {
		var hits int32
		srv := jsonServer(t, 0, http.StatusOK, &hits)

		mh := multi(t, srv.URL)
		var resp testResponse
		require.NoError(t, mh.Post(context.Background(), "/submit", nil, bytes.NewBufferString(body), &resp))
		assert.NotEqual(t, "", resp.Host)
		assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
	})

	t.Run("broadcasts to every node", func(t *testing.T) {
		var hits1, hits2 int32
		bn1 := jsonServer(t, 0, http.StatusOK, &hits1)
		bn2 := jsonServer(t, 0, http.StatusOK, &hits2)

		mh := multi(t, bn1.URL, bn2.URL)
		var resp testResponse
		require.NoError(t, mh.Post(context.Background(), "/submit", nil, bytes.NewBufferString(body), &resp))

		// The second node may be written after Post returns (detached context).
		require.NoError(t, waitFor(func() bool {
			return atomic.LoadInt32(&hits1) == 1 && atomic.LoadInt32(&hits2) == 1
		}))
	})

	t.Run("succeeds when any node accepts", func(t *testing.T) {
		bad := jsonServer(t, 0, http.StatusInternalServerError, nil)
		good := jsonServer(t, 0, http.StatusOK, nil)

		mh := multi(t, bad.URL, good.URL)
		var resp testResponse
		require.NoError(t, mh.Post(context.Background(), "/submit", nil, bytes.NewBufferString(body), &resp))
	})

	t.Run("fails when every node fails", func(t *testing.T) {
		bad1 := jsonServer(t, 0, http.StatusInternalServerError, nil)
		bad2 := jsonServer(t, 0, http.StatusBadGateway, nil)

		mh := multi(t, bad1.URL, bad2.URL)
		var resp testResponse
		require.NotNil(t, mh.Post(context.Background(), "/submit", nil, bytes.NewBufferString(body), &resp))
	})

	t.Run("nil response discards the body across the fleet", func(t *testing.T) {
		var goodHits, badHits int32
		good := jsonServer(t, 0, http.StatusOK, &goodHits)
		bad := jsonServer(t, 0, http.StatusInternalServerError, &badHits)

		mh := multi(t, good.URL, bad.URL)
		require.NoError(t, mh.Post(context.Background(), "/submit", nil, bytes.NewBufferString(body), nil))

		// Both nodes are contacted; the failing node exercises the nil-resp error path.
		require.NoError(t, waitFor(func() bool {
			return atomic.LoadInt32(&goodHits) == 1 && atomic.LoadInt32(&badHits) == 1
		}))
	})

	t.Run("surfaces a response decode error", func(t *testing.T) {
		bn1 := jsonServer(t, 0, http.StatusOK, nil)
		bn2 := jsonServer(t, 0, http.StatusOK, nil)

		mh := multi(t, bn1.URL, bn2.URL)
		var resp int // the JSON object body cannot be decoded into an int
		require.NotNil(t, mh.Post(context.Background(), "/submit", nil, bytes.NewBufferString(body), &resp))
	})
}

func TestMultiHandlerPostSSZ(t *testing.T) {
	const body = "ssz-bytes"

	t.Run("single handler short-circuits to the sole node", func(t *testing.T) {
		srv := sszServer(t, "accepted")

		mh := multi(t, srv.URL)
		require.NoError(t, mh.PostSSZ(context.Background(), "/publish", nil, bytes.NewBufferString(body)))
	})

	t.Run("succeeds when any node accepts", func(t *testing.T) {
		bad := jsonServer(t, 0, http.StatusInternalServerError, nil)
		good := sszServer(t, "accepted")

		mh := multi(t, bad.URL, good.URL)
		require.NoError(t, mh.PostSSZ(context.Background(), "/publish", nil, bytes.NewBufferString(body)))
	})

	// In a mixed fleet where one node accepts SSZ and another rejects the body with 415,
	// PostSSZ must surface the 415 so the caller can re-broadcast via JSON, rather than
	// hiding it behind the accepting node's success and never reaching the rejecting node.
	t.Run("surfaces a 415 so the caller can fall back to JSON", func(t *testing.T) {
		accepts := sszServer(t, "accepted")
		rejects := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Unsupported media type: application/octet-stream", http.StatusUnsupportedMediaType)
		}))
		t.Cleanup(rejects.Close)

		mh := multi(t, accepts.URL, rejects.URL)
		err := mh.PostSSZ(context.Background(), "/publish", nil, bytes.NewBufferString(body))
		require.NotNil(t, err)
		assert.Equal(t, true, errors.Is(err, &httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType}),
			"a node's 415 must surface so the caller falls back to JSON")
	})

	t.Run("returns the joined error when every node fails", func(t *testing.T) {
		bad1 := jsonServer(t, 0, http.StatusInternalServerError, nil)
		bad2 := jsonServer(t, 0, http.StatusBadGateway, nil)

		mh := multi(t, bad1.URL, bad2.URL)
		err := mh.PostSSZ(context.Background(), "/publish", nil, bytes.NewBufferString(body))
		require.NotNil(t, err)
	})

	t.Run("returns the context error when canceled before any node accepts", func(t *testing.T) {
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			<-release
		}))
		t.Cleanup(srv.Close)
		t.Cleanup(func() { close(release) })

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		mh := multi(t, srv.URL, srv.URL)
		err := mh.PostSSZ(ctx, "/publish", nil, bytes.NewBufferString(body))
		assert.Equal(t, true, errors.Is(err, context.Canceled))
	})
}

func TestReadUntil(t *testing.T) {
	// noopFn is never consulted: the fake rounds below ignore the query function.
	noopFn := func(context.Context, *handler) (string, error) { return "", nil }
	// accept is likewise ignored by the fake rounds.
	accept := func(string) bool { return true }

	t.Run("returns immediately on a match", func(t *testing.T) {
		var calls int
		round := func(context.Context, []*handler, time.Time, func(string) bool, queryFunc[string]) (string, bool, bool, []error) {
			calls++
			return "matched", true, true, nil
		}

		val, matched, err := readUntil(context.Background(), nil, getConfig{}, accept, round, noopFn)
		require.NoError(t, err)
		assert.Equal(t, true, matched)
		assert.Equal(t, "matched", val)
		assert.Equal(t, 1, calls, "a match should stop after the first round")
	})

	t.Run("returns the best-effort fallback when nothing matches and repoll is off", func(t *testing.T) {
		round := func(context.Context, []*handler, time.Time, func(string) bool, queryFunc[string]) (string, bool, bool, []error) {
			return "fallback", false, true, nil
		}

		val, matched, err := readUntil(context.Background(), nil, getConfig{}, accept, round, noopFn)
		require.NoError(t, err)
		assert.Equal(t, false, matched)
		assert.Equal(t, "fallback", val)
	})

	t.Run("returns the joined error when nothing matches and no usable response", func(t *testing.T) {
		sentinel := errors.New("boom")
		round := func(context.Context, []*handler, time.Time, func(string) bool, queryFunc[string]) (string, bool, bool, []error) {
			return "", false, false, []error{sentinel}
		}

		_, matched, err := readUntil(context.Background(), nil, getConfig{}, accept, round, noopFn)
		assert.Equal(t, false, matched)
		require.NotNil(t, err)
		assert.Equal(t, true, errors.Is(err, sentinel))
	})

	t.Run("repolls until a match within the deadline", func(t *testing.T) {
		var calls int
		round := func(context.Context, []*handler, time.Time, func(string) bool, queryFunc[string]) (string, bool, bool, []error) {
			calls++
			if calls >= 3 {
				return "fresh", true, true, nil
			}
			return "stale", false, true, nil
		}

		cfg := getConfig{pollInterval: time.Millisecond, deadline: time.Now().Add(2 * time.Second)}
		val, matched, err := readUntil(context.Background(), nil, cfg, accept, round, noopFn)
		require.NoError(t, err)
		assert.Equal(t, true, matched)
		assert.Equal(t, "fresh", val)
		assert.Equal(t, true, calls >= 3, "should have repolled across multiple rounds")
	})

	t.Run("stops on the first usable response in UntilAny2xx mode", func(t *testing.T) {
		var calls int
		round := func(context.Context, []*handler, time.Time, func(string) bool, queryFunc[string]) (string, bool, bool, []error) {
			calls++
			return "stale", false, true, nil // usable 2xx response, but not a match
		}

		cfg := getConfig{pollInterval: time.Millisecond, deadline: time.Now().Add(2 * time.Second), repollMode: UntilAny2xx}
		val, matched, err := readUntil(context.Background(), nil, cfg, accept, round, noopFn)
		require.NoError(t, err)
		assert.Equal(t, false, matched)
		assert.Equal(t, "stale", val)
		assert.Equal(t, 1, calls, "any 2xx response should stop re-polling in UntilAny2xx mode")
	})

	t.Run("repolls on a total failure until a usable response in UntilAny2xx mode", func(t *testing.T) {
		var calls int
		round := func(context.Context, []*handler, time.Time, func(string) bool, queryFunc[string]) (string, bool, bool, []error) {
			calls++
			if calls >= 3 {
				return "stale", false, true, nil // finally a usable response
			}
			return "", false, false, nil // total failure: no usable response this round
		}

		cfg := getConfig{pollInterval: time.Millisecond, deadline: time.Now().Add(2 * time.Second), repollMode: UntilAny2xx}
		val, matched, err := readUntil(context.Background(), nil, cfg, accept, round, noopFn)
		require.NoError(t, err)
		assert.Equal(t, false, matched)
		assert.Equal(t, "stale", val)
		assert.Equal(t, true, calls >= 3, "should have repolled across rounds while every node failed")
	})

	t.Run("returns the fallback when canceled during the poll wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		round := func(context.Context, []*handler, time.Time, func(string) bool, queryFunc[string]) (string, bool, bool, []error) {
			return "stale", false, true, nil // ok=true records a fallback
		}

		// Cancel while readUntil is blocked in the (long) poll wait.
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		cfg := getConfig{pollInterval: time.Hour, deadline: time.Now().Add(time.Hour)}
		val, matched, err := readUntil(ctx, nil, cfg, accept, round, noopFn)
		require.NoError(t, err)
		assert.Equal(t, false, matched)
		assert.Equal(t, "stale", val)
	})

	t.Run("returns the context error when canceled during the poll wait with no fallback", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		round := func(context.Context, []*handler, time.Time, func(string) bool, queryFunc[string]) (string, bool, bool, []error) {
			return "", false, false, nil // ok=false leaves no fallback
		}

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		cfg := getConfig{pollInterval: time.Hour, deadline: time.Now().Add(time.Hour)}
		_, matched, err := readUntil(ctx, nil, cfg, accept, round, noopFn)
		assert.Equal(t, false, matched)
		require.NotNil(t, err)
		assert.Equal(t, true, errors.Is(err, context.Canceled))
	})
}

func TestRoundFor(t *testing.T) {
	// The two strategies can't be compared as function values, so distinguish them
	// by behavior: raceRound launches a goroutine per handler (every handler is
	// queried), while inOrderRound queries sequentially and stops at the first match.
	accept := func(string) bool { return true } // every response matches

	t.Run("race queries every handler concurrently", func(t *testing.T) {
		var calls int32
		fn := func(context.Context, *handler) (string, error) {
			atomic.AddInt32(&calls, 1)
			return "ok", nil
		}

		round := roundFor[string](getConfig{race: true})
		handlers := []*handler{newTestHandler("http://a"), newTestHandler("http://b")}
		val, matched, ok, _ := round(context.Background(), handlers, time.Time{}, accept, fn)
		assert.Equal(t, true, matched)
		assert.Equal(t, true, ok)
		assert.Equal(t, "ok", val)
		require.NoError(t, waitFor(func() bool { return atomic.LoadInt32(&calls) == 2 }))
	})

	t.Run("in-order stops at the first matching handler", func(t *testing.T) {
		var calls int32
		fn := func(context.Context, *handler) (string, error) {
			atomic.AddInt32(&calls, 1)
			return "ok", nil
		}

		round := roundFor[string](getConfig{race: false})
		handlers := []*handler{newTestHandler("http://a"), newTestHandler("http://b")}
		val, matched, ok, _ := round(context.Background(), handlers, time.Time{}, accept, fn)
		assert.Equal(t, true, matched)
		assert.Equal(t, true, ok)
		assert.Equal(t, "ok", val)
		assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "in-order should query only the first handler once it matches")
	})
}

func TestRaceRound(t *testing.T) {
	handlers := []*handler{newTestHandler("http://a"), newTestHandler("http://b")}

	t.Run("returns the first response satisfying accept", func(t *testing.T) {
		accept := func(s string) bool { return s == "fresh" }
		fn := func(_ context.Context, h *handler) (string, error) {
			if h == handlers[1] {
				return "fresh", nil
			}
			return "stale", nil
		}

		val, matched, ok, errs := raceRound(context.Background(), handlers, time.Time{}, accept, fn)
		assert.Equal(t, true, matched)
		assert.Equal(t, true, ok)
		assert.Equal(t, "fresh", val)
		assert.Equal(t, 0, len(errs))
	})

	t.Run("falls back to a usable response when none match", func(t *testing.T) {
		accept := func(string) bool { return false } // nothing matches
		fn := func(context.Context, *handler) (string, error) { return "stale", nil }

		val, matched, ok, _ := raceRound(context.Background(), handlers, time.Time{}, accept, fn)
		assert.Equal(t, false, matched)
		assert.Equal(t, true, ok, "a non-matching 2XX is still a usable response")
		assert.Equal(t, "stale", val)
	})

	t.Run("reports failure when every handler errors", func(t *testing.T) {
		sentinel := errors.New("boom")
		accept := func(string) bool { return true }
		fn := func(context.Context, *handler) (string, error) { return "", sentinel }

		_, matched, ok, errs := raceRound(context.Background(), handlers, time.Time{}, accept, fn)
		assert.Equal(t, false, matched)
		assert.Equal(t, false, ok)
		assert.Equal(t, len(handlers), len(errs))
		assert.Equal(t, true, errors.Is(errors.Join(errs...), sentinel))
	})

	t.Run("stops waiting for a hung handler at the fallback deadline", func(t *testing.T) {
		accept := func(string) bool { return false } // the responding node is lagging
		fn := func(ctx context.Context, h *handler) (string, error) {
			if h == handlers[1] {
				<-ctx.Done() // hung: only unblocks when the deadline fires
				return "", ctx.Err()
			}

			return "stale", nil
		}

		// A read deadline far beyond the fallback deadline stands in for the slot deadline.
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		start := time.Now()
		val, matched, ok, _ := raceRound(ctx, handlers, time.Now().Add(20*time.Millisecond), accept, fn)
		assert.Equal(t, false, matched)
		assert.Equal(t, true, ok)
		assert.Equal(t, "stale", val, "the usable response must be returned rather than waiting on the hung node")
		assert.Equal(t, true, time.Since(start) < 10*time.Second, "the round must not wait for the read deadline")
	})

	t.Run("returns the fallback when the context is canceled with a usable response in hand", func(t *testing.T) {
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })

		accept := func(string) bool { return false }
		fn := func(_ context.Context, h *handler) (string, error) {
			if h == handlers[1] {
				<-release // hung, and deaf to cancellation
				return "", errors.New("released")
			}

			return "stale", nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Cancel once the round is blocked waiting on the hung handler, so its
		// select observes ctx.Done() with the first response already recorded.
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		val, matched, ok, errs := raceRound(ctx, handlers, time.Time{}, accept, fn)
		assert.Equal(t, false, matched)
		assert.Equal(t, true, ok, "a response collected before the cancellation is still usable")
		assert.Equal(t, "stale", val)
		assert.Equal(t, true, errors.Is(errors.Join(errs...), context.Canceled))
	})

	t.Run("returns promptly when the context is canceled with nothing in hand", func(t *testing.T) {
		accept := func(string) bool { return true }
		fn := func(ctx context.Context, _ *handler) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, matched, ok, errs := raceRound(ctx, handlers, time.Time{}, accept, fn)
		assert.Equal(t, false, matched)
		assert.Equal(t, false, ok)
		assert.Equal(t, true, errors.Is(errors.Join(errs...), context.Canceled))
	})
}

func TestInOrderRound(t *testing.T) {
	handlers := []*handler{newTestHandler("http://a"), newTestHandler("http://b")}

	t.Run("returns the first response satisfying accept", func(t *testing.T) {
		accept := func(s string) bool { return s == "fresh" }
		fn := func(_ context.Context, h *handler) (string, error) {
			if h == handlers[0] {
				return "fresh", nil
			}
			return "stale", nil
		}

		val, matched, ok, errs := inOrderRound(context.Background(), handlers, time.Time{}, accept, fn)
		assert.Equal(t, true, matched)
		assert.Equal(t, true, ok)
		assert.Equal(t, "fresh", val)
		assert.Equal(t, 0, len(errs))
	})

	t.Run("falls back to a usable response when none match", func(t *testing.T) {
		accept := func(string) bool { return false } // nothing matches
		fn := func(context.Context, *handler) (string, error) { return "stale", nil }

		val, matched, ok, _ := inOrderRound(context.Background(), handlers, time.Time{}, accept, fn)
		assert.Equal(t, false, matched)
		assert.Equal(t, true, ok, "a non-matching 2XX is still a usable response")
		assert.Equal(t, "stale", val)
	})

	t.Run("reports failure when every handler errors", func(t *testing.T) {
		sentinel := errors.New("boom")
		accept := func(string) bool { return true }
		fn := func(context.Context, *handler) (string, error) { return "", sentinel }

		_, matched, ok, errs := inOrderRound(context.Background(), handlers, time.Time{}, accept, fn)
		assert.Equal(t, false, matched)
		assert.Equal(t, false, ok)
		assert.Equal(t, len(handlers), len(errs))
		assert.Equal(t, true, errors.Is(errors.Join(errs...), sentinel))
	})

	t.Run("stops when the context is already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		accept := func(string) bool { return true }
		fn := func(context.Context, *handler) (string, error) { return "ok", nil }

		_, matched, ok, errs := inOrderRound(ctx, handlers, time.Time{}, accept, fn)
		assert.Equal(t, false, matched)
		assert.Equal(t, false, ok)
		assert.Equal(t, true, errors.Is(errors.Join(errs...), context.Canceled))
	})
}

func TestBroadcastWrite(t *testing.T) {
	t.Run("propagates the caller's deadline to the detached writes", func(t *testing.T) {
		deadline := time.Now().Add(time.Hour)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		var (
			gotDeadline  time.Time
			haveDeadline bool
		)
		fn := func(c context.Context, _ *handler) (string, error) {
			gotDeadline, haveDeadline = c.Deadline()
			return "ok", nil
		}

		val, err := broadcastWrite(ctx, []*handler{newTestHandler("http://a")}, fn)
		require.NoError(t, err)
		assert.Equal(t, "ok", val)
		assert.Equal(t, true, haveDeadline, "the detached write should inherit the caller's deadline")
		assert.Equal(t, deadline, gotDeadline)
	})

	t.Run("returns the first success without a caller deadline", func(t *testing.T) {
		fn := func(context.Context, *handler) (string, error) { return "ok", nil }

		val, err := broadcastWrite(context.Background(), []*handler{newTestHandler("http://a")}, fn)
		require.NoError(t, err)
		assert.Equal(t, "ok", val)
	})

	t.Run("returns the joined error when every handler fails", func(t *testing.T) {
		sentinel := errors.New("boom")
		fn := func(context.Context, *handler) (string, error) { return "", sentinel }

		handlers := []*handler{newTestHandler("http://a"), newTestHandler("http://b")}
		_, err := broadcastWrite(context.Background(), handlers, fn)
		require.NotNil(t, err)
		assert.Equal(t, true, errors.Is(err, sentinel))
	})

	t.Run("returns the context error when canceled with nothing buffered", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })

		// fn blocks so no result is ever buffered; the drain must hit its default
		// branch and surface the context error.
		fn := func(context.Context, *handler) (string, error) {
			<-release
			return "late", nil
		}

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		_, err := broadcastWrite(ctx, []*handler{newTestHandler("http://a")}, fn)
		assert.Equal(t, true, errors.Is(err, context.Canceled))
	})

}

func TestCloneBuffer(t *testing.T) {
	t.Run("returns nil when the original data is nil", func(t *testing.T) {
		assert.Equal(t, true, cloneBuffer(nil, []byte("ignored")) == nil)
	})

	t.Run("returns an independent copy of raw", func(t *testing.T) {
		raw := []byte("payload")
		buf := cloneBuffer(bytes.NewBuffer(raw), raw)
		require.NotNil(t, buf)
		assert.Equal(t, "payload", buf.String())

		// Mutating the original bytes must not affect the clone.
		raw[0] = 'X'
		assert.Equal(t, "payload", buf.String())
	})
}

// waitFor polls cond for up to a second.
func waitFor(cond func() bool) error {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	if cond() {
		return nil
	}
	return context.DeadlineExceeded
}

// newTestHandler builds a *handler pointing at the given base URL.
func newTestHandler(host string) *handler {
	return newHandler(http.Client{Timeout: 5 * time.Second}, host)
}

func multi(t *testing.T, hosts ...string) *multiHandler {
	handlers := make([]*handler, 0, len(hosts))
	for _, host := range hosts {
		handlers = append(handlers, newTestHandler(host))
	}

	multiHandler, err := newMultiHandler(handlers)
	require.NoError(t, err)

	return multiHandler
}

func jsonServer(t *testing.T, delay time.Duration, status int, hits *int32) *httptest.Server {
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
		w.WriteHeader(status)
		if status >= 200 && status < 300 {
			_, _ = w.Write([]byte(`{"host":"` + r.Host + `"}`))
		} else {
			_, _ = w.Write([]byte(`{"code":` + http.StatusText(status) + `,"message":"err"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
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
