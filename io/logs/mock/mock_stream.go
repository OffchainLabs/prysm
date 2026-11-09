package mock

import "github.com/OffchainLabs/prysm/v7/async/event"

type MockStreamer struct {
	logs [][]byte
	feed *event.Feed
}

// NewMockStreamer creates a new instance of MockStreamer.
// It's useful to set up the default state for the mock, like initializing the feed.
func NewMockStreamer(logs [][]byte) *MockStreamer {
	return &MockStreamer{
		logs: logs,
		feed: new(event.Feed),
	}
}

// GetLastFewLogs returns the predefined logs.
func (m *MockStreamer) GetLastFewLogs() [][]byte {
	return m.logs
}

// SubscribeLogs subscribes ch to the logs sent with Send.
func (m *MockStreamer) SubscribeLogs(ch chan<- []byte) event.Subscription {
	return m.feed.Subscribe(ch)
}

// Send delivers a log line to every subscriber.
func (m *MockStreamer) Send(log []byte) {
	m.feed.Send(log)
}
