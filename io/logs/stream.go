package logs

import (
	"bytes"
	"io"
	"sync"

	"github.com/OffchainLabs/prysm/v7/async/event"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	"github.com/OffchainLabs/prysm/v7/crypto/rand"
	lru "github.com/hashicorp/golang-lru"
)

const (
	// The number of log entries to keep in memory.
	logCacheSize = 20
)

var (
	// Compile time interface checks.
	_ = io.Writer(&StreamServer{})
	_ = Streamer(&StreamServer{})
)

// Streamer defines a struct which can retrieve and stream process logs.
type Streamer interface {
	GetLastFewLogs() [][]byte
	SubscribeLogs(ch chan<- []byte) event.Subscription
}

// StreamServer receives every process log line (it is installed as a logrus output)
// and forwards it to the subscribed log streams.
type StreamServer struct {
	cache *lru.Cache

	mu          sync.RWMutex
	subscribers map[*logSubscription]bool
}

// NewStreamServer initializes a new stream server capable of
// streaming log events.
func NewStreamServer() *StreamServer {
	ss := &StreamServer{
		cache:       lruwrpr.New(logCacheSize),
		subscribers: make(map[*logSubscription]bool),
	}
	addLogWriter(ss)
	return ss
}

// GetLastFewLogs returns the last few entries of logs stored in an LRU cache.
func (ss *StreamServer) GetLastFewLogs() [][]byte {
	messages := make([][]byte, 0)
	for _, k := range ss.cache.Keys() {
		d, ok := ss.cache.Get(k)
		if ok {
			messages = append(messages, d.([]byte))
		}
	}
	return messages
}

// SubscribeLogs registers ch to receive every new log line.
// Lines are delivered without blocking: if ch is full, the line is dropped for this subscriber.
// Once Unsubscribe returns, nothing more is sent on ch, so the caller may close it.
func (ss *StreamServer) SubscribeLogs(ch chan<- []byte) event.Subscription {
	sub := &logSubscription{
		server: ss,
		ch:     ch,
		err:    make(chan error),
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.subscribers[sub] = true

	return sub
}

// Write forwards a log line to every subscriber.
func (ss *StreamServer) Write(p []byte) (n int, err error) {
	// io.Writer implementations must not retain p, and logrus reuses its buffers.
	msg := bytes.Clone(p)

	ss.mu.RLock()
	for sub := range ss.subscribers {
		select {
		case sub.ch <- msg:
		default:
			// The subscriber is not keeping up. Drop the line rather than block the logger.
		}
	}
	ss.mu.RUnlock()

	ss.cache.Add(rand.NewGenerator().Uint64(), msg)
	return len(p), nil
}

// logSubscription is the subscription returned by SubscribeLogs.
type logSubscription struct {
	server *StreamServer
	ch     chan<- []byte
	once   sync.Once
	err    chan error
}

// Unsubscribe stops the delivery of log lines and closes the error channel.
func (s *logSubscription) Unsubscribe() {
	s.once.Do(func() {
		s.server.mu.Lock()
		delete(s.server.subscribers, s)
		s.server.mu.Unlock()

		close(s.err)
	})
}

// Err returns a channel that is closed when Unsubscribe is called.
func (s *logSubscription) Err() <-chan error {
	return s.err
}
