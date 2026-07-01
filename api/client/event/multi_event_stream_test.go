package event

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// headEvent formats a single SSE "head" event for the given slot.
func headEvent(slot int) string {
	return fmt.Sprintf("event: head\ndata: {\"slot\":\"%d\"}\n\n", slot)
}

// sseServer starts an httptest server that writes the given events once and then
// either holds the connection open (until the client disconnects) or returns,
// closing the connection so the client reconnects.
func sseServer(t *testing.T, hold bool, events ...string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.Equal(t, true, ok)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range events {
			_, err := fmt.Fprint(w, e)
			require.NoError(t, err)
			flusher.Flush()
		}
		if hold {
			<-r.Context().Done()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newStream(t *testing.T, ctx context.Context, hosts ...string) *MultiEventStream {
	mes, err := NewMultiEventStream(ctx, &http.Client{}, hosts, []string{"head"})
	require.NoError(t, err)
	return mes
}

func TestNewMultiEventStream(t *testing.T) {
	tests := []struct {
		name    string
		hosts   []string
		topics  []string
		wantErr string
	}{
		{"no hosts", nil, []string{"head"}, "no hosts provided"},
		{"no topics", []string{"http://h:1"}, nil, "no topics provided"},
		{"valid", []string{"http://h:1"}, []string{"head"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMultiEventStream(t.Context(), &http.Client{}, tt.hosts, tt.topics)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, tt.wantErr, err)
		})
	}
}

func TestDeduper(t *testing.T) {
	t.Run("first sighting is unseen, repeat is seen", func(t *testing.T) {
		d := newDeduper(4)
		e := &Event{Type: EventHead, Data: []byte(`{"slot":"1"}`)}
		require.Equal(t, false, d.seen(e))
		require.Equal(t, true, d.seen(e))
	})

	t.Run("same data under a different type is not a duplicate", func(t *testing.T) {
		d := newDeduper(4)
		require.Equal(t, false, d.seen(&Event{Type: EventHead, Data: []byte("x")}))
		require.Equal(t, false, d.seen(&Event{Type: EventError, Data: []byte("x")}))
	})

	t.Run("evicts the oldest entry once capacity is exceeded", func(t *testing.T) {
		const capacity = 3
		d := newDeduper(capacity)

		ev := func(i int) *Event {
			return &Event{Type: EventHead, Data: []byte(fmt.Sprintf("%d", i))}
		}

		// Fill exactly to capacity with distinct events; each is new.
		for i := range capacity {
			require.Equal(t, false, d.seen(ev(i)))
		}
		// The oldest entry (0) is still remembered.
		require.Equal(t, true, d.seen(ev(0)))

		// A new distinct event pushes past capacity and evicts the oldest key (0).
		require.Equal(t, false, d.seen(ev(capacity)))

		// Having been evicted, 0 is reported as unseen again...
		require.Equal(t, false, d.seen(ev(0)))
		// ...while the bookkeeping stays bounded at capacity.
		require.Equal(t, capacity, len(d.seenSet))
		require.Equal(t, capacity, len(d.order))
	})
}

func TestMultiEventStream_Subscribe(t *testing.T) {
	t.Run("dedups identical events from multiple hosts", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		a := sseServer(t, true, headEvent(1))
		b := sseServer(t, true, headEvent(1)) // same event from a second node

		out := make(chan *Event, 8)
		go newStream(t, ctx, a.URL, b.URL).Subscribe(out)

		// Exactly one head event should be forwarded.
		select {
		case e := <-out:
			require.Equal(t, EventHead, e.Type)
		case <-time.After(2 * time.Second):
			t.Fatal("expected a head event")
		}
		select {
		case e := <-out:
			t.Fatalf("expected no duplicate event, got %s/%s", e.Type, string(e.Data))
		case <-time.After(300 * time.Millisecond):
		}
	})

	t.Run("forwards distinct events", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		a := sseServer(t, true, headEvent(1))
		b := sseServer(t, true, headEvent(2)) // different slot ⇒ not a duplicate

		out := make(chan *Event, 8)
		go newStream(t, ctx, a.URL, b.URL).Subscribe(out)

		got := map[string]bool{}
		for range 2 {
			select {
			case e := <-out:
				got[string(e.Data)] = true
			case <-time.After(2 * time.Second):
				t.Fatal("expected two distinct head events")
			}
		}
		require.Equal(t, true, got[`{"slot":"1"}`])
		require.Equal(t, true, got[`{"slot":"2"}`])
	})

	t.Run("survives a dead host", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		good := sseServer(t, true, headEvent(7))
		const deadHost = "http://127.0.0.1:1" // connection refused

		out := make(chan *Event, 8)
		go newStream(t, ctx, deadHost, good.URL).Subscribe(out)

		// The healthy host's event still arrives, and the dead host's connection
		// error is logged, not forwarded.
		select {
		case e := <-out:
			require.Equal(t, EventHead, e.Type)
			require.Equal(t, `{"slot":"7"}`, string(e.Data))
		case <-time.After(2 * time.Second):
			t.Fatal("expected the healthy host's event")
		}
	})

	t.Run("reconnects after the stream drops", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		var conns atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			flusher := w.(http.Flusher)
			w.Header().Set("Content-Type", "text/event-stream")
			// Each new connection emits a distinct event, then returns (closing the
			// stream) so the client must reconnect to receive the next one.
			n := int(conns.Add(1))
			_, _ = fmt.Fprint(w, headEvent(n))
			flusher.Flush()
		}))
		defer srv.Close()

		out := make(chan *Event, 8)
		go newStream(t, ctx, srv.URL).Subscribe(out)

		// First connection's event.
		select {
		case e := <-out:
			require.Equal(t, `{"slot":"1"}`, string(e.Data))
		case <-time.After(2 * time.Second):
			t.Fatal("expected first event")
		}
		// After the stream drops, the host reconnects (backoff) and delivers the next.
		select {
		case e := <-out:
			require.Equal(t, `{"slot":"2"}`, string(e.Data))
		case <-time.After(initialReconnectBackoff + 2*time.Second):
			t.Fatal("expected event after reconnect")
		}
	})
}
