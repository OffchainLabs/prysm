package event

import (
	"errors"
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
	errCh := make(chan error, 1)
	go func() {
		errCh <- stream.Subscribe(eventsChannel)
	}()

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
	require.NoError(t, <-errCh)
}

func TestEventStream_InvalidTopic(t *testing.T) {
	const invalidTopic = "bogus"

	mux := http.NewServeMux()
	mux.HandleFunc("/eth/v1/events", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte(`{"message":"invalid topic name: ` + invalidTopic + `","code":400}`))
		require.NoError(t, err)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	eventsChannel := make(chan *Event, 1)
	stream, err := NewEventStream(t.Context(), http.DefaultClient, server.URL, []string{invalidTopic})
	require.NoError(t, err)

	err = stream.Subscribe(eventsChannel)
	var subErr *SubscriptionError
	require.Equal(t, true, errors.As(err, &subErr))
	require.Equal(t, http.StatusBadRequest, subErr.StatusCode)
	require.StringContains(t, "invalid topic name: "+invalidTopic, subErr.Body)
	select {
	case event := <-eventsChannel:
		t.Fatalf("unexpected event for subscription error: %#v", event)
	default:
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
	err = stream.Subscribe(eventsChannel)
	require.NotNil(t, err)

	event := <-eventsChannel
	if event.EventType != EventConnectionError {
		t.Errorf("Expected event type %q, got %q", EventConnectionError, event.EventType)
	}

}
