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

func TestNewEventStream(t *testing.T) {
	validURL := "http://localhost:8080"
	invalidURL := "://invalid"
	topics := []string{"topic1", "topic2"}

	tests := []struct {
		name    string
		host    string
		topics  []string
		wantErr bool
	}{
		{"Valid input", validURL, topics, false},
		{"Invalid URL", invalidURL, topics, true},
		{"No topics", validURL, []string{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEventStream(t.Context(), &http.Client{}, tt.host, tt.topics)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEventStream() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEventStream(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/eth/v1/events", func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.Equal(t, true, ok)
		for i := 1; i <= 3; i++ {
			events := [3]string{"event: head\ndata: data%d\n\n", "event: head\rdata: data%d\r\r", "event: head\r\ndata: data%d\r\n\r\n"}
			_, err := fmt.Fprintf(w, events[i-1], i)
			require.NoError(t, err)
			flusher.Flush()                    // Trigger flush to simulate streaming data
			time.Sleep(100 * time.Millisecond) // Simulate delay between events
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	topics := []string{"head"}
	eventsChannel := make(chan *Event, 1)
	stream, err := NewEventStream(t.Context(), http.DefaultClient, server.URL, topics)
	require.NoError(t, err)
	go stream.Subscribe(eventsChannel)

	// Collect events
	var events []*Event

	for len(events) != 3 {
		select {
		case event := <-eventsChannel:
			log.Info(event)
			events = append(events, event)
		}
	}

	// Assertions to verify the events content
	expectedData := []string{"data1", "data2", "data3"}
	for i, event := range events {
		if string(event.Data) != expectedData[i] {
			t.Errorf("Expected event data %q, got %q", expectedData[i], string(event.Data))
		}
	}
}

func TestEventStreamRequestError(t *testing.T) {
	topics := []string{"head"}
	eventsChannel := make(chan *Event, 1)
	ctx := t.Context()

	// use valid url that will result in failed request with nil body
	stream, err := NewEventStream(ctx, http.DefaultClient, "http://badhost:1234", topics)
	require.NoError(t, err)

	// error will happen when request is made, should be received over events channel
	go stream.Subscribe(eventsChannel)

	event := <-eventsChannel
	if event.EventType != EventConnectionError {
		t.Errorf("Expected event type %q, got %q", EventConnectionError, event.EventType)
	}
}

func TestEventStream_ContextCancelDuringBlockedSend(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/eth/v1/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.Equal(t, true, ok)
		// Send events continuously until the client disconnects.
		for i := 0; ; i++ {
			_, err := fmt.Fprintf(w, "event: head\ndata: data%d\n\n", i)
			if err != nil {
				return
			}
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Use an unbuffered channel so sends will block.
	eventsChannel := make(chan *Event)
	ctx, cancel := context.WithCancel(t.Context())
	stream, err := NewEventStream(ctx, http.DefaultClient, server.URL, []string{"head"})
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		stream.Subscribe(eventsChannel)
		close(done)
	}()

	// Cancel the context while the goroutine is trying to send on the blocked channel.
	cancel()

	// The goroutine should exit promptly.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe goroutine did not exit after context cancel")
	}
}

func TestEventStream_DoesNotCloseChannel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/eth/v1/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.Equal(t, true, ok)
		_, err := fmt.Fprintf(w, "event: head\ndata: data1\n\n")
		if err != nil {
			return
		}
		flusher.Flush()
		// Close the connection after one event to end the scanner loop.
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	eventsChannel := make(chan *Event, 10)
	stream, err := NewEventStream(t.Context(), http.DefaultClient, server.URL, []string{"head"})
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		stream.Subscribe(eventsChannel)
		close(done)
	}()

	// Wait for Subscribe to finish.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe goroutine did not exit")
	}

	// Channel should still be open (not closed). Verify by sending to it.
	select {
	case eventsChannel <- &Event{EventType: "test"}:
		// Successfully sent, channel is open.
	default:
		t.Fatal("Channel appears to be closed or blocked unexpectedly")
	}
}
