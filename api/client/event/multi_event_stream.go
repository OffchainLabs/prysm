package event

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	initialReconnectBackoff  = 1 * time.Second  // Wait before the first reconnect attempt to a host.
	maxReconnectBackoff      = 16 * time.Second // Caps the per-host reconnect backoff.
	healthyReconnectDuration = 5 * time.Second  // A subscription that stays up at least this long is considered healthy and resets the backoff.
	dedupCapacity            = 256              // Bounds the set of recently-seen events used to drop duplicates emitted by multiple beacon nodes.
)

type MultiEventStream struct {
	ctx        context.Context
	httpClient *http.Client
	hosts      []string
	topics     []string
}

var _ EventStreamClient = &MultiEventStream{}

// NewMultiEventStream creates a MultiEventStream over the given hosts.
func NewMultiEventStream(ctx context.Context, httpClient *http.Client, hosts []string, topics []string) (*MultiEventStream, error) {
	if len(hosts) == 0 {
		return nil, errors.New("no hosts provided")
	}

	if len(topics) == 0 {
		return nil, errors.New("no topics provided")
	}

	multiEventStream := &MultiEventStream{
		ctx:        ctx,
		httpClient: httpClient,
		hosts:      hosts,
		topics:     topics,
	}

	return multiEventStream, nil
}

// Subscribe runs one reconnecting SSE subscription per host, merges them, drops
// duplicates, and forwards events to out. It blocks until the context is
// cancelled.
func (m *MultiEventStream) Subscribe(out chan<- *Event) error {
	// host-1 --\
	// host-2 ---+--> merged --> deduper --> out
	// host-3 --/

	merged := make(chan *Event, len(m.hosts))

	// Run one subscription per host, merging them into merged.
	var wg sync.WaitGroup
	for _, host := range m.hosts {
		wg.Go(func() {
			m.runHost(host, merged)
		})
	}

	go func() {
		wg.Wait()
		close(merged)
	}()

	// Deduplicate events from merged and forward them to out.
	deduper := newDeduper(dedupCapacity)

	for {
		select {
		case event, ok := <-merged:
			if !ok {
				return nil
			}

			if event.Type == EventError || event.Type == EventConnectionError {
				log.WithField("data", string(event.Data)).Warning("Beacon node event stream reported an error")
				continue
			}

			if deduper.seen(event) {
				continue
			}

			// Forward to out.
			select {
			case out <- event:
			case <-m.ctx.Done():
				return nil
			}
		case <-m.ctx.Done():
			return nil
		}
	}
}

// runHost maintains a single host's subscription, reconnecting with capped
// exponential backoff until the context is cancelled.
func (m *MultiEventStream) runHost(host string, merged chan<- *Event) {
	backoff := initialReconnectBackoff
	topics := m.topics

	for {
		if m.ctx.Err() != nil {
			return
		}

		stream, err := NewEventStream(m.ctx, m.httpClient, host, topics)
		if err != nil {
			select {
			case merged <- &Event{
				Type: EventConnectionError,
				Data: []byte(err.Error()),
			}:
			case <-m.ctx.Done():
			}

			return
		}

		start := time.Now()

		// Blocking call
		err = stream.Subscribe(merged)

		// If the context was cancelled, we're done.
		if m.ctx.Err() != nil {
			return
		}

		// If some topics are not supported, try falling back to the legacy topics.
		var subErr *httputil.DefaultJsonError
		if fallbackTopics, ok := LegacyTopicFallback(topics); ok && errors.As(err, &subErr) && subErr.Code == http.StatusBadRequest {
			log.
				WithFields(logrus.Fields{
					"host":            api.RedactEndpoint(host),
					"topics":          strings.Join(topics, ","),
					"fallback_topics": strings.Join(fallbackTopics, ","),
				}).
				WithError(err).
				Warning("Beacon node does not support the given topics. Falling back to the legacy topics")

			topics = fallbackTopics
			continue
		}

		// Context was not cancelled, so the subscription must have ended due to an error
		// Report it and reconnect with backoff.
		if time.Since(start) >= healthyReconnectDuration {
			backoff = initialReconnectBackoff
		}

		log.
			WithFields(logrus.Fields{"host": api.RedactEndpoint(host), "backoff": backoff}).
			Warning("Beacon node event stream disconnected, reconnecting")

		select {
		case <-time.After(backoff):
		case <-m.ctx.Done():
			return
		}

		if backoff *= 2; backoff > maxReconnectBackoff {
			backoff = maxReconnectBackoff
		}
	}
}

type deduper struct {
	cache *lru.Cache
}

func newDeduper(capacity int) *deduper {
	return &deduper{cache: lruwrpr.New(capacity)}
}

// seen reports whether e was already seen, recording it as seen otherwise.
func (d *deduper) seen(e *Event) bool {
	key := e.Type + "\x00" + canonicalPayload(e.Data)
	existed, _ := d.cache.ContainsOrAdd(key, nil)
	return existed
}

// canonicalPayload normalizes an event's JSON payload so that a single logical
// event emitted by multiple beacon nodes produces the same dedup key even when
// the nodes (for example, different client implementations) order object fields
// or insert whitespace differently. Non-JSON payloads are returned unchanged.
func canonicalPayload(data []byte) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return string(data)
	}

	// This cannot fail: fields came from a successful Unmarshal, so every
	// RawMessage is already valid JSON that re-marshals verbatim.
	canonical, _ := json.Marshal(fields)
	return string(canonical)
}
