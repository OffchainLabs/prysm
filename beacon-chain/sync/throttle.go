package sync

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"
)

// ThrottleStreamsPerPeer is the maximum number of throttledStream writers that peerThrottle will allocate.
// Sized so that a peer running a coupled blocks+blobs/columns range batch alongside a couple of by-root
// lookups (as Lighthouse does) does not have to wait for a slot; the bandwidth budget is shared regardless.
const ThrottleStreamsPerPeer = 4

// ThrottleBurst is the maximum burst size for the rate limiter. This is essentially the capacity of the
// bucket when tokens are unused for enough intervals to fill the bucket (or when first started).
const ThrottleBurst = ThrottleStreamsPerPeer * 1024 * 1024 // 4 MiB

// ThrottleBPS is the number of bytes that can be written per second by a peerThrottle.
const ThrottleBPS rate.Limit = rate.Limit(float64(ThrottleBurst / ThrottleStreamsPerPeer))

// peerThrottlePruneInterval is how frequently the pruner should run
const peerThrottlePruneInterval = time.Minute

var (
	errAcquireWouldBlock   = errors.New("acquire for peerThrottle would block")
	errPeerThrottlesPruned = errors.New("peer stream throttles have been pruned")
)

// ThrottleMuxOption is a functional option for configuring the peerThrottleMux.
type ThrottleMuxOption func(*peerThrottleMux)

// WithThrottleBPS sets the per-peer upstream throttle in bytes per second.
// A value <= 0 disables rpc response throttling entirely.
func WithThrottleBPS(bps rate.Limit) ThrottleMuxOption {
	return func(m *peerThrottleMux) {
		m.bps = bps
	}
}

// WithThrottleBurst sets the burst size for the per-peer upstream throttle.
// We usually want this to be at least as large as the throttle bps so that the leaky bucket
// starts out with full tokens for the first interval. A good rule of thumb is to set this to
// streamsPerPeer * bps.
func WithThrottleBurst(burst int) ThrottleMuxOption {
	return func(m *peerThrottleMux) {
		m.burst = burst
	}
}

// WithThrottleStreamsPerPeer sets the number of concurrent streams allowed per peer.
// The throttle should not be applied to rpc methods needed for peering like ping, metadata etc.
func WithThrottleStreamsPerPeer(streamsPerPeer int) ThrottleMuxOption {
	return func(m *peerThrottleMux) {
		m.streamsPerPeer = streamsPerPeer
	}
}

// WithThrottleMuxOptions adds the provided ThrottleMuxOptions to the Service's peerThrottleMuxOptions.
func WithThrottleMuxOptions(opts ...ThrottleMuxOption) Option {
	return func(s *Service) error {
		s.peerThrottleMuxOptions = append(s.peerThrottleMuxOptions, opts...)
		return nil
	}
}

// peerThrottleMux allows a goroutine that wants to write to a stream to wait for an available
// throttledStream for a given peer.
type peerThrottleMux struct {
	ctx            context.Context // long-lived context bounding throttled writes and the pruner
	mu             sync.Mutex
	throttles      map[peer.ID]*peerThrottle
	bps            rate.Limit
	burst          int
	streamsPerPeer int
}

// newPeerThrottleMux builds a mux from the defaults and the given options, clamping invalid values:
// bps <= 0 disables throttling, burst is raised to at least bps and streamsPerPeer to at least 1.
// ctx should outlive every stream served through the mux, typically the sync service context.
func newPeerThrottleMux(ctx context.Context, opts ...ThrottleMuxOption) *peerThrottleMux {
	pw := &peerThrottleMux{
		ctx:            ctx,
		throttles:      make(map[peer.ID]*peerThrottle),
		bps:            ThrottleBPS,
		burst:          ThrottleBurst,
		streamsPerPeer: ThrottleStreamsPerPeer,
	}
	for _, opt := range opts {
		opt(pw)
	}
	if pw.burst < int(pw.bps) {
		pw.burst = int(pw.bps)
	}
	if pw.streamsPerPeer < 1 {
		pw.streamsPerPeer = 1
	}
	return pw
}

// enabled reports whether rpc responses should be throttled at all. It is safe to call on a nil mux.
func (m *peerThrottleMux) enabled() bool {
	return m != nil && m.bps > 0
}

// throttle blocks until a throttledStream slot is available for the peer, or waitCtx is done.
// The returned stream must be released by calling its cleanup method when the handler is done with it.
func (m *peerThrottleMux) throttle(waitCtx context.Context, pid peer.ID, w network.Stream) (*throttledStream, error) {
	for {
		ts, err := m.getOrCreate(pid).wait(waitCtx, w)
		if errors.Is(err, errPeerThrottlesPruned) {
			// The throttle was pruned between getOrCreate and wait; loop to create a fresh one.
			continue
		}
		return ts, err
	}
}

func (m *peerThrottleMux) getOrCreate(pid peer.ID) *peerThrottle {
	m.mu.Lock()
	defer m.mu.Unlock()

	if pt, ok := m.throttles[pid]; ok && !pt.isPruned() {
		return pt
	}
	rpcPeerThrottlesCreated.Inc()
	pt := newPeerThrottle(m.ctx, pid, m.bps, m.burst, m.streamsPerPeer)
	m.throttles[pid] = pt
	return pt
}

// spawnPruner periodically removes idle peer throttles until the mux context is done.
func (m *peerThrottleMux) spawnPruner() {
	if m == nil {
		return
	}
	go func() {
		next := time.Now().Add(peerThrottlePruneInterval)
		for {
			select {
			case now := <-time.After(time.Until(next)):
				next = now.Add(peerThrottlePruneInterval)
				m.prune()
			case <-m.ctx.Done():
				return
			}
		}
	}()
}

func (m *peerThrottleMux) prune() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, pt := range m.throttles {
		m.prunePeer(pt)
	}
}

// prunePeer removes the peerThrottle if it is idle. The caller must hold m.mu.
func (m *peerThrottleMux) prunePeer(pt *peerThrottle) {
	release, err := pt.acquireFast()
	if err != nil {
		return
	}
	defer release()

	if !pt.canPrune() {
		return
	}

	rpcPeerThrottlesPruned.Inc()
	delete(m.throttles, pt.pid)
	pt.markPruned()
}

type peerThrottle struct {
	ctx       context.Context // long-lived context handed to every throttledStream
	exclusive chan struct{}
	streams   []*throttledStream
	limiter   bpsLimiter
	cleanup   chan int
	pruned    *safeDone
	pid       peer.ID
}

func newPeerThrottle(ctx context.Context, pid peer.ID, bps rate.Limit, burst int, streamsPerPeer int) *peerThrottle {
	limiter := newBpsLimiter(bps, burst)
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return &peerThrottle{
		ctx:       ctx,
		exclusive: ch,
		limiter:   limiter,
		cleanup:   make(chan int, streamsPerPeer),
		pruned:    newSafeDone(),
		pid:       pid,
		streams:   make([]*throttledStream, streamsPerPeer),
	}
}

// wait blocks until a stream slot is free or waitCtx is done. The slot wait is bounded by waitCtx, but the
// returned stream is bound to the long-lived throttle context so that slow responses are not cut short.
func (pt *peerThrottle) wait(waitCtx context.Context, w network.Stream) (*throttledStream, error) {
	release, err := pt.acquire(waitCtx)
	if err != nil {
		return nil, err
	}
	defer release()

	for i := range pt.streams {
		if pt.streams[i] == nil {
			sw := newThrottledStream(pt.ctx, pt.pid, w, pt.limiter, pt.cleanupForIdx(i))
			pt.streams[i] = sw
			return sw, nil
		}
	}

	select {
	case idx := <-pt.cleanup:
		sw := newThrottledStream(pt.ctx, pt.pid, w, pt.limiter, pt.cleanupForIdx(idx))
		pt.streams[idx] = sw
		return sw, nil
	case <-waitCtx.Done():
		return nil, waitCtx.Err()
	}
}

// acquire waits to acquire exclusive access to the peerThrottle.
// If the peerThrottle has already been pruned, it will return errPeerThrottlesPruned.
// Otherwise, it will block until the exclusive access is available or the context is done.
func (pt *peerThrottle) acquire(ctx context.Context) (func(), error) {
	// Check pruned first to ensure deterministic behavior when already pruned.
	if pt.isPruned() {
		return nil, errPeerThrottlesPruned
	}

	select {
	case <-pt.exclusive:
		// The pruner holds exclusive access while pruning, so re-check after acquiring it.
		if pt.isPruned() {
			pt.exclusive <- struct{}{}
			return nil, errPeerThrottlesPruned
		}
		return func() { pt.exclusive <- struct{}{} }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// acquireFast attempts to acquire exclusive access to the peerThrottle without blocking.
// If the peerThrottle has already been pruned, it will return errPeerThrottlesPruned.
// If the exclusive access is not immediately available, it will return errAcquireWouldBlock.
func (pt *peerThrottle) acquireFast() (func(), error) {
	if pt.isPruned() {
		return nil, errPeerThrottlesPruned
	}

	select {
	case <-pt.exclusive:
		return func() { pt.exclusive <- struct{}{} }, nil
	default:
		return nil, errAcquireWouldBlock
	}
}

func (pt *peerThrottle) canPrune() bool {
	for {
		select {
		case idx := <-pt.cleanup:
			pt.streams[idx] = nil
		default:
			// If any stream is non-nil, cannot prune
			for _, w := range pt.streams {
				if w != nil {
					return false
				}
			}

			return true
		}
	}
}

func (pt *peerThrottle) markPruned() {
	pt.pruned.Close()
}

func (pt *peerThrottle) isPruned() bool {
	select {
	case <-pt.pruned.Done():
		return true
	default:
		return false
	}
}

func (pt *peerThrottle) cleanupForIdx(idx int) func() {
	start := time.Now()
	return func() {
		rpcThrottleStreamMilliseconds.Observe(float64(time.Since(start).Milliseconds()))
		pt.cleanup <- idx
	}
}

// bpsLimiter wraps a rate.Limiter with the largest chunk that can be reserved in one WaitN call.
type bpsLimiter struct {
	*rate.Limiter
	chunk int
}

// newBpsLimiter clamps the chunk size to burst so that WaitN never asks for more than the bucket can hold.
func newBpsLimiter(bps rate.Limit, burst int) bpsLimiter {
	chunk := max(min(int(bps), burst), 1)
	return bpsLimiter{Limiter: rate.NewLimiter(bps, burst), chunk: chunk}
}

// throttledStream wraps an io.Writer, like a libp2p Stream, and
// throttles writes to enable configurable bandwidth limiting.
type throttledStream struct {
	network.Stream
	ctx           context.Context
	limiter       bpsLimiter
	pid           peer.ID
	cleanup       func()       // see comment on newThrottledStream for notes re idempotency
	writeDeadline atomic.Int64 // unix nanos of the last write deadline set on the stream, 0 if none
}

// newThrottledStream initializes a throttledStream, wrapping the provided network.Stream
// and applying the provided rate limiter for bandwidth control.
// The context is retained by the underlying value and checked for cancellation between calls to the
// underlying writer. It should be long-lived (the service context): how long a write may block for is
// bounded by the write deadline set on the stream by the handler, not by this context.
//
// newThrottledStream ensures that throttledStream.cleanup is idempotent, however a throttledStream
// initialized directly without using newThrottledStream will not have that guarantee and must ensure its
// own idempotency.
func newThrottledStream(ctx context.Context, pid peer.ID, w network.Stream, limiter bpsLimiter, cleanup func()) *throttledStream {
	return &throttledStream{ctx: ctx, pid: pid, Stream: w, limiter: limiter, cleanup: sync.OnceFunc(cleanup)}
}

// Write implements the io.Writer interface for throttledStream.
func (s *throttledStream) Write(p []byte) (n int, err error) {
	ctx, cancel := s.writeContext()
	defer cancel()
	for len(p) > 0 {
		chunk := min(len(p), s.limiter.chunk)
		if err := s.limiter.WaitN(ctx, chunk); err != nil {
			return n, errors.Wrap(err, "rate limiter WaitN")
		}
		written, err := s.Stream.Write(p[:chunk])
		rpcThrottleWrittenBytes.Add(float64(written))
		n += written
		if err != nil {
			return n, err
		}
		p = p[written:]
	}
	return n, nil
}

// writeContext bounds the time a write may wait for tokens by the write deadline set on the stream, if any.
func (s *throttledStream) writeContext() (context.Context, context.CancelFunc) {
	deadline := s.writeDeadline.Load()
	if deadline == 0 {
		return s.ctx, func() {}
	}
	return context.WithDeadline(s.ctx, time.Unix(0, deadline))
}

// SetWriteDeadline records the deadline so that throttled writes fail fast instead of waiting past it.
func (s *throttledStream) SetWriteDeadline(t time.Time) error {
	s.recordWriteDeadline(t)
	return s.Stream.SetWriteDeadline(t)
}

// SetDeadline records the deadline so that throttled writes fail fast instead of waiting past it.
func (s *throttledStream) SetDeadline(t time.Time) error {
	s.recordWriteDeadline(t)
	return s.Stream.SetDeadline(t)
}

func (s *throttledStream) recordWriteDeadline(t time.Time) {
	if t.IsZero() {
		s.writeDeadline.Store(0)
		return
	}
	s.writeDeadline.Store(t.UnixNano())
}

// Close delegates to the underlying writer's Close method and cleans up the throttledStream,
// so that a new throttledStream can be used by another goroutine.
func (s *throttledStream) Close() error {
	defer s.cleanup()
	return s.Stream.Close()
}

// CloseWrite delegates to the underlying stream's CloseWrite method and cleans up the throttledStream,
// so that a new throttledStream can be used by another goroutine.
func (s *throttledStream) CloseWrite() error {
	defer s.cleanup()
	return s.Stream.CloseWrite()
}

var _ network.Stream = &throttledStream{}

// safeDone is a concurrency primitive that allows multiple goroutines to wait
// until some condition is satisfied, at which point the done channel is closed exactly once.
// It's important to only close once because closing a closed channel panics.
type safeDone struct {
	done chan struct{}
	once sync.Once
}

func newSafeDone() *safeDone {
	return &safeDone{
		done: make(chan struct{}),
	}
}

func (sd *safeDone) Done() <-chan struct{} {
	return sd.done
}

func (sd *safeDone) Close() {
	sd.once.Do(func() {
		close(sd.done)
	})
}
