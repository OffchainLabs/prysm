package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestIndependentRepollCutoff(t *testing.T) {
	for _, earlierParent := range []bool{false, true} {
		name := "configured deadline"
		if earlierParent {
			name = "earlier parent deadline"
		}
		t.Run(name, func(t *testing.T) {
			deadline := time.Now().Add(100 * time.Millisecond)
			wantDeadline := deadline
			ctx := t.Context()
			if earlierParent {
				wantDeadline = deadline.Add(-50 * time.Millisecond)
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, wantDeadline)
				defer cancel()
			}
			handlers := []*handler{{}, {}}
			var calls, active [2]atomic.Int32
			var retries atomic.Int32
			var overlap, wrongDeadline atomic.Bool
			status := &httputil.DefaultJsonError{Code: http.StatusServiceUnavailable}
			fn := func(ctx context.Context, h *handler) (string, error) {
				i := 0
				if h == handlers[1] {
					i = 1
				}
				calls[i].Add(1)
				if active[i].Add(1) > 1 {
					overlap.Store(true)
				}
				defer active[i].Add(-1)
				if actual, _ := ctx.Deadline(); actual != wantDeadline {
					wrongDeadline.Store(true)
				}
				if i == 0 {
					select {
					case <-time.After(10 * time.Millisecond):
						return "", status
					case <-ctx.Done():
					}
				} else {
					<-ctx.Done()
				}
				return "", ctx.Err()
			}
			cfg := newQueryConfig([]QueryOption{WithDeadline(deadline), WithIndependentRepoll(time.Millisecond, func() { retries.Add(1) })})
			_, matched, err := queryUntilAccepted(ctx, handlers, cfg, func(string) bool { return false }, raceRound[string], fn)
			require.ErrorIs(t, err, status)
			if earlierParent {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
			require.Equal(t, false, matched)
			require.NoError(t, waitFor(func() bool { return active[0].Load()+active[1].Load() == 0 }))
			assert.Equal(t, false, wrongDeadline.Load())
			assert.Equal(t, false, overlap.Load())
			assert.Equal(t, true, calls[0].Load() > 1)
			assert.Equal(t, int32(1), calls[1].Load())
			assert.Equal(t, calls[0].Load()-1, retries.Load())
		})
	}
}

func TestIndependentRepollRetainsResults(t *testing.T) {
	for _, tt := range []struct {
		name         string
		fallback     bool
		cancelParent bool
	}{
		{name: "latest fallback survives final stalled attempt", fallback: true},
		{name: "latest status survives final stalled attempt"},
		{name: "parent cancellation preserves status", cancelParent: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if n > 2 {
					if tt.cancelParent {
						cancel()
					}
					<-r.Context().Done()
					return
				}
				if tt.fallback {
					if n == 1 {
						_, _ = w.Write([]byte("old"))
					} else {
						_, _ = w.Write([]byte("new"))
					}
					return
				}
				if n == 1 {
					w.WriteHeader(http.StatusBadGateway)
				} else {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
			}))
			t.Cleanup(server.Close)
			body, _, err := multi(t, server.URL).GetSSZ(ctx, "/x",
				WithIndependentRepoll(10*time.Millisecond, nil),
				WithDeadline(time.Now().Add(150*time.Millisecond)),
				WithSSZAccept(func([]byte, http.Header) bool { return false }),
			)
			if tt.fallback {
				require.NoError(t, err)
				require.Equal(t, "new", string(body))
			} else {
				require.ErrorIs(t, err, &httputil.DefaultJsonError{Code: http.StatusServiceUnavailable})
				assert.Equal(t, false, errors.Is(err, &httputil.DefaultJsonError{Code: http.StatusBadGateway}))
				assert.Equal(t, false, errors.Is(err, context.DeadlineExceeded))
				if tt.cancelParent {
					require.ErrorIs(t, err, context.Canceled)
				}
			}
			require.Equal(t, int32(3), calls.Load())
		})
	}
}

func TestIndependentRepollRequiresIntervalAndDeadline(t *testing.T) {
	for _, tt := range []struct {
		name     string
		interval time.Duration
		deadline time.Time
	}{
		{name: "no deadline", interval: time.Millisecond},
		{name: "zero interval", deadline: time.Now().Add(time.Second)},
		{name: "negative interval", interval: -time.Millisecond, deadline: time.Now().Add(time.Second)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls, retries atomic.Int32
			fn := func(context.Context, *handler) (string, error) {
				calls.Add(1)
				return "fallback", nil
			}
			cfg := newQueryConfig([]QueryOption{WithDeadline(tt.deadline), WithIndependentRepoll(tt.interval, func() { retries.Add(1) })})
			got, matched, err := queryUntilAccepted(t.Context(), []*handler{{}}, cfg, func(string) bool { return false }, raceRound[string], fn)
			require.NoError(t, err)
			require.Equal(t, "fallback", got)
			require.Equal(t, false, matched)
			require.Equal(t, int32(1), calls.Load())
			require.Equal(t, int32(0), retries.Load())
		})
	}
}
