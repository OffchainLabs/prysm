package sync

import (
	"io"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// mockStream implements network.Stream using io.Pipe for testing.
// This provides a simpler alternative to mocknet that avoids its
// internal buffering, timers, and async transport goroutine.
type mockStream struct {
	reader    *io.PipeReader
	writer    *io.PipeWriter
	closeOnce sync.Once
}

// newMockStreamPair creates two connected mock streams.
// Data written to one can be read from the other.
func newMockStreamPair() (*mockStream, *mockStream) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	s1 := &mockStream{reader: r1, writer: w2}
	s2 := &mockStream{reader: r2, writer: w1}
	return s1, s2
}

func (s *mockStream) Read(p []byte) (int, error)  { return s.reader.Read(p) }
func (s *mockStream) Write(p []byte) (int, error) { return s.writer.Write(p) }

func (s *mockStream) Close() error {
	s.closeOnce.Do(func() {
		_ = s.reader.Close()
		_ = s.writer.Close()
	})
	return nil
}

func (s *mockStream) CloseWrite() error                              { return s.writer.Close() }
func (s *mockStream) CloseRead() error                               { return s.reader.Close() }
func (s *mockStream) Reset() error                                   { return s.Close() }
func (s *mockStream) ResetWithError(_ network.StreamErrorCode) error { return s.Close() }

// Stub implementations for interface compliance
func (s *mockStream) SetDeadline(_ time.Time) error      { return nil }
func (s *mockStream) SetReadDeadline(_ time.Time) error  { return nil }
func (s *mockStream) SetWriteDeadline(_ time.Time) error { return nil }
func (s *mockStream) ID() string                         { return "mock" }
func (s *mockStream) Protocol() protocol.ID              { return "" }
func (s *mockStream) SetProtocol(_ protocol.ID) error    { return nil }
func (s *mockStream) Stat() network.Stats                { return network.Stats{} }
func (s *mockStream) Conn() network.Conn                 { return nil }
func (s *mockStream) Scope() network.StreamScope         { return &network.NullScope{} }

var _ network.Stream = (*mockStream)(nil)
