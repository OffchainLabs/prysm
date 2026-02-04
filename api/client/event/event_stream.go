package event

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/client"
	"github.com/pkg/errors"
)

const (
	EventHead = "head"

	EventError           = "error"
	EventConnectionError = "connection_error"
)

var (
	_ = EventStreamClient(&EventStream{})
)

var DefaultEventTopics = []string{EventHead}

type EventStreamClient interface {
	Subscribe(eventsChannel chan<- *Event)
}

type Event struct {
	EventType string
	Data      []byte
}

// Send sends an event to the channel, respecting context cancellation.
// Returns true if the event was sent, false if the context was cancelled.
func Send(ctx context.Context, ch chan<- *Event, e *Event) bool {
	select {
	case ch <- e:
		return true
	case <-ctx.Done():
		return false
	}
}

// EventStream is responsible for subscribing to the Beacon API events endpoint
// and dispatching received events to subscribers.
type EventStream struct {
	ctx        context.Context
	httpClient *http.Client
	host       string
	topics     []string
}

func NewEventStream(ctx context.Context, httpClient *http.Client, host string, topics []string) (*EventStream, error) {
	// Check if the host is a valid URL
	_, err := url.ParseRequestURI(host)
	if err != nil {
		return nil, err
	}
	if len(topics) == 0 {
		return nil, errors.New("no topics provided")
	}

	return &EventStream{
		ctx:        ctx,
		httpClient: httpClient,
		host:       host,
		topics:     topics,
	}, nil
}

func (h *EventStream) Subscribe(eventsChannel chan<- *Event) {
	allTopics := strings.Join(h.topics, ",")
	log.WithField("topics", allTopics).Info("Listening to Beacon API events")
	fullUrl := h.host + "/eth/v1/events?topics=" + allTopics
	req, err := http.NewRequestWithContext(h.ctx, http.MethodGet, fullUrl, nil)
	if err != nil {
		Send(h.ctx, eventsChannel, &Event{
			EventType: EventConnectionError,
			Data:      []byte(errors.Wrap(err, "failed to create HTTP request").Error()),
		})
		return
	}
	req.Header.Set("Accept", api.EventStreamMediaType)
	req.Header.Set("Connection", api.KeepAlive)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		Send(h.ctx, eventsChannel, &Event{
			EventType: EventConnectionError,
			Data:      []byte(errors.Wrap(err, client.ErrConnectionIssue.Error()).Error()),
		})
		return
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.WithError(closeErr).Error("Failed to close events response body")
		}
	}()
	// Create a new scanner to read lines from the response body
	scanner := bufio.NewScanner(resp.Body)
	// Set the split function for the scanning operation
	scanner.Split(scanLinesWithCarriage)

	var eventType, data string // Variables to store event type and data

	// Iterate over lines of the event stream
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			// Empty line indicates the end of an event
			if eventType != "" && data != "" {
				if !Send(h.ctx, eventsChannel, &Event{EventType: eventType, Data: []byte(data)}) {
					return
				}
			}
			eventType, data = "", ""
			continue
		}
		et, ok := strings.CutPrefix(line, "event: ")
		if ok {
			eventType = et
		}
		d, ok := strings.CutPrefix(line, "data: ")
		if ok {
			data = d
		}
	}

	if err := scanner.Err(); err != nil {
		Send(h.ctx, eventsChannel, &Event{
			EventType: EventConnectionError,
			Data:      []byte(errors.Wrap(err, errors.Wrap(client.ErrConnectionIssue, "scanner failed").Error()).Error()),
		})
	}
}
