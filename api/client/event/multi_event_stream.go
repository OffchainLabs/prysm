package event

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	initialReconnectBackoff = 1 * time.Second  // wait before the first reconnect attempt to a host.
	maxReconnectBackoff     = 16 * time.Second // caps the per-host reconnect backoff.
	dedupCapacity           = 256              // bounds the set of recently-seen events used to drop duplicates emitted by multiple beacon nodes.
)

// MultiEventStream subscribes to the Beacon API events endpoint of several beacon
// nodes at once and forwards a single, de-duplicated stream of
// events to the caller.
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
func (m *MultiEventStream) Subscribe(out chan<- *Event) {
	merged := make(chan *Event, len(m.hosts))

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

	deduper := newDeduper(dedupCapacity)

	for event := range merged {
		if event.Type == EventError || event.Type == EventConnectionError {
			log.WithField("data", string(event.Data)).Debug("Beacon node event stream reported an error")
			continue
		}

		if deduper.seen(event) {
			continue
		}

		select {
		case out <- event:
		case <-m.ctx.Done():
			return
		}
	}
}

// runHost maintains a single host's subscription, reconnecting with capped
// exponential backoff until the context is cancelled.
func (m *MultiEventStream) runHost(host string, merged chan<- *Event) {
	backoff := initialReconnectBackoff
	for {
		if m.ctx.Err() != nil {
			return
		}

		stream, err := NewEventStream(m.ctx, m.httpClient, host, m.topics)
		if err != nil {
			merged <- &Event{
				Type: EventConnectionError,
				Data: []byte(errors.Wrapf(err, "invalid event stream host %s", host).Error()),
			}
			return
		}

		start := time.Now()

		// Blocking call
		stream.Subscribe(merged)

		if m.ctx.Err() != nil {
			return
		}

		// Reset the backoff after a subscription that stayed up long enough to be considered healthy.
		if time.Since(start) > maxReconnectBackoff {
			backoff = initialReconnectBackoff
		}

		log.WithFields(logrus.Fields{"host": host, "backoff": backoff}).Debug("Beacon node event stream disconnected, reconnecting")

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

// deduper tracks recently-seen events in a bounded FIFO set so duplicate events
// emitted by multiple beacon nodes are forwarded only once. It is not safe for
// concurrent use.
type deduper struct {
	seenSet map[string]bool
	order   []string
	cap     int
}

func newDeduper(capacity int) *deduper {
	return &deduper{seenSet: make(map[string]bool, capacity), cap: capacity}
}

// seen reports whether e was already seen, recording it as seen otherwise.
func (d *deduper) seen(e *Event) bool {
	key := e.Type + "\x00" + string(e.Data)
	if d.seenSet[key] {
		return true
	}

	d.seenSet[key] = true
	d.order = append(d.order, key)

	if len(d.order) > d.cap {
		oldest := d.order[0]
		d.order = d.order[1:]
		delete(d.seenSet, oldest)
	}

	return false
}
