package event

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

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

func TestSubscribe(t *testing.T) {
	t.Run("dedups identical events from multiple hosts", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		a := sseServer(t, true, headEvent(1))
		b := sseServer(t, true, headEvent(1)) // same event from a second node

		out := make(chan *Event, 8)
		errCh := make(chan error, 1)
		go func() { errCh <- newStream(t, ctx, a.URL, b.URL).Subscribe(out) }()

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

		cancel()
		require.NoError(t, <-errCh)
	})

	t.Run("returns when every host has exited", func(t *testing.T) {
		// An invalid request URI makes NewEventStream fail, so the host's
		// runHost goroutine reports a connection error and exits immediately.
		// With no host left running, merged is closed and Subscribe returns nil.
		out := make(chan *Event, 8)
		errCh := make(chan error, 1)
		go func() {
			errCh <- newStream(t, t.Context(), "not-a-valid-url").Subscribe(out)
		}()

		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("expected Subscribe to return after every host exited")
		}
	})

	t.Run("returns when the context is canceled while forwarding", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())

		srv := sseServer(t, true, headEvent(3))

		// out is unbuffered and never read, so once the event reaches the
		// forwarding send it blocks there until the context is canceled.
		out := make(chan *Event)
		errCh := make(chan error, 1)
		go func() {
			errCh <- newStream(t, ctx, srv.URL).Subscribe(out)
		}()

		// Give the event time to travel to the blocking send, then cancel.
		time.Sleep(200 * time.Millisecond)
		cancel()

		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("expected Subscribe to return after the context was canceled")
		}
	})

	t.Run("host exits immediately when the context is already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel() // Cancel before subscribing, so runHost returns on its first loop check.

		srv := sseServer(t, true, headEvent(5))

		out := make(chan *Event, 8)
		errCh := make(chan error, 1)
		go func() {
			errCh <- newStream(t, ctx, srv.URL).Subscribe(out)
		}()

		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("expected Subscribe to return promptly with an already-canceled context")
		}
	})

	t.Run("falls back to the legacy topics on a 400", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		// The server rejects the modern topic with a 400 but serves the legacy
		// one, so the host must fall back from head_v2 to head and reconnect.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("topics") == EventHeadV2 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			flusher, ok := w.(http.Flusher)
			require.Equal(t, true, ok)
			w.Header().Set("Content-Type", "text/event-stream")
			_, err := fmt.Fprint(w, headEvent(9))
			require.NoError(t, err)
			flusher.Flush()
			<-r.Context().Done()
		}))
		// Registered as a cleanup (not a defer) so it runs after cancel() closes
		// the client connection, letting the held-open handler return.
		t.Cleanup(srv.Close)

		mes, err := NewMultiEventStream(ctx, &http.Client{}, []string{srv.URL}, []string{EventHeadV2})
		require.NoError(t, err)

		out := make(chan *Event, 8)
		errCh := make(chan error, 1)
		go func() { errCh <- mes.Subscribe(out) }()

		// The event only arrives once the host has fallen back to the legacy topic.
		select {
		case e := <-out:
			require.Equal(t, EventHead, e.Type)
			require.Equal(t, `{"slot":"9"}`, string(e.Data))
		case <-time.After(2 * time.Second):
			t.Fatal("expected the fallback (legacy topic) event")
		}

		cancel()
		require.NoError(t, <-errCh)
	})

	t.Run("survives a dead host", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		good := sseServer(t, true, headEvent(7))
		const deadHost = "http://127.0.0.1:1" // connection refused

		out := make(chan *Event, 8)
		errCh := make(chan error, 1)
		go func() { errCh <- newStream(t, ctx, deadHost, good.URL).Subscribe(out) }()

		// The healthy host's event still arrives, and the dead host's connection
		// error is logged, not forwarded.
		select {
		case e := <-out:
			require.Equal(t, EventHead, e.Type)
			require.Equal(t, `{"slot":"7"}`, string(e.Data))
		case <-time.After(2 * time.Second):
			t.Fatal("expected the healthy host's event")
		}

		cancel()
		require.NoError(t, <-errCh)
	})
}

func TestCanonicalPayload(t *testing.T) {
	t.Run("normalizes field order and whitespace", func(t *testing.T) {
		a := canonicalPayload([]byte(`{"slot":"1","block":"0xabc"}`))
		b := canonicalPayload([]byte("{ \"block\": \"0xabc\",\n\"slot\": \"1\" }"))
		require.Equal(t, a, b)
	})

	t.Run("returns non-JSON payloads unchanged", func(t *testing.T) {
		require.Equal(t, "not json", canonicalPayload([]byte("not json")))
	})
}

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
