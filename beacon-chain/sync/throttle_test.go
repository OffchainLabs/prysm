package sync

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/time/rate"
)

// newTestStream creates a mock stream pair and returns the local end.
// A drain reader is started on the remote end to consume all written data.
func newTestStream(t *testing.T) network.Stream {
	local, remote := newMockStreamPair()

	// Start drain reader on remote end
	go func() {
		_, _ = io.Copy(io.Discard, remote)
	}()

	t.Cleanup(func() {
		_ = local.Close()
		_ = remote.Close()
	})

	return local
}

// errorInjectingStream wraps a network.Stream and allows injecting errors for testing.
type errorInjectingStream struct {
	network.Stream
	mu           sync.Mutex
	writeErr     error
	closeErr     error
	partialWrite int
}

func (s *errorInjectingStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	writeErr := s.writeErr
	partialWrite := s.partialWrite
	s.mu.Unlock()

	if writeErr != nil {
		return 0, writeErr
	}
	if partialWrite > 0 && partialWrite < len(p) {
		// Write only partialWrite bytes and return that count
		// This simulates a partial write scenario
		n, err := s.Stream.Write(p[:partialWrite])
		if err != nil {
			return n, err
		}
		// Return however many bytes were actually written (up to partialWrite)
		return n, nil
	}
	return s.Stream.Write(p)
}

func (s *errorInjectingStream) Close() error {
	s.mu.Lock()
	closeErr := s.closeErr
	s.mu.Unlock()

	if closeErr != nil {
		_ = s.Stream.Close()
		return closeErr
	}
	return s.Stream.Close()
}

func (s *errorInjectingStream) CloseWrite() error {
	s.mu.Lock()
	closeErr := s.closeErr
	s.mu.Unlock()

	if closeErr != nil {
		_ = s.Stream.CloseWrite()
		return closeErr
	}
	return s.Stream.CloseWrite()
}

// streamWriteCounter wraps a network.Stream and counts writes.
type streamWriteCounter struct {
	network.Stream
	mu         sync.Mutex
	chunks     int
	totalBytes int
}

func (s *streamWriteCounter) Write(p []byte) (int, error) {
	n, err := s.Stream.Write(p)
	s.mu.Lock()
	s.chunks++
	s.totalBytes += n
	s.mu.Unlock()
	return n, err
}

func (s *streamWriteCounter) stats() (chunks int, totalBytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chunks, s.totalBytes
}

// testBpsLimiter creates a bpsLimiter for testing with specified bps and burst.
func testBpsLimiter(bps, burst int) bpsLimiter {
	return newBpsLimiter(rate.Limit(bps), burst)
}

func TestThrottledStream_Write(t *testing.T) {
	cases := []struct {
		name         string
		bps          int
		burst        int
		writeSize    int
		wantChunks   int
		wantTotal    int
		partialWrite int
		writeErr     error
		wantErr      bool
	}{
		{
			name:       "small write no chunking",
			bps:        1000,
			burst:      2000,
			writeSize:  500,
			wantChunks: 1,
			wantTotal:  500,
		},
		{
			name:       "larger than bps chunks into two",
			bps:        1000,
			burst:      2000,
			writeSize:  1500,
			wantChunks: 2,
			wantTotal:  1500,
		},
		{
			name:       "three chunks",
			bps:        1000,
			burst:      3000,
			writeSize:  2500,
			wantChunks: 3,
			wantTotal:  2500,
		},
		{
			name:       "burst smaller than bps clamps chunks to burst",
			bps:        1000,
			burst:      500,
			writeSize:  600,
			wantChunks: 2,
			wantTotal:  600,
		},
		{
			name:         "partial write continues",
			bps:          1000,
			burst:        2000,
			writeSize:    500,
			partialWrite: 100,
			wantChunks:   5, // 500 bytes written 100 at a time = 5 writes
			wantTotal:    500,
		},
		{
			name:      "write error propagates",
			bps:       1000,
			burst:     2000,
			writeSize: 500,
			writeErr:  errors.New("write failed"),
			wantErr:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := newTestStream(t)

			// Wrap with error injector if needed
			var wrappedStream network.Stream
			var counter *streamWriteCounter

			if tc.writeErr != nil || tc.partialWrite > 0 {
				errStream := &errorInjectingStream{
					Stream:       stream,
					writeErr:     tc.writeErr,
					partialWrite: tc.partialWrite,
				}
				counter = &streamWriteCounter{Stream: errStream}
				wrappedStream = counter
			} else {
				counter = &streamWriteCounter{Stream: stream}
				wrappedStream = counter
			}

			limiter := testBpsLimiter(tc.bps, tc.burst)
			ts := newThrottledStream(context.Background(), "", wrappedStream, limiter, func() {})

			data := make([]byte, tc.writeSize)
			for i := range data {
				data[i] = byte(i % 256)
			}
			n, err := ts.Write(data)

			if tc.wantErr {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantTotal, n)

			chunks, totalBytes := counter.stats()
			require.Equal(t, tc.wantChunks, chunks)
			require.Equal(t, tc.wantTotal, totalBytes)
		})
	}
}

func TestThrottledStream_Write_RateLimiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stream := newTestStream(t)
		counter := &streamWriteCounter{Stream: stream}

		// Very slow rate: 100 bytes per second, 100 byte burst
		limiter := testBpsLimiter(100, 100)
		ts := newThrottledStream(t.Context(), "", counter, limiter, func() {})

		// Write 200 bytes - should require 2 chunks with rate limiting
		data := make([]byte, 200)
		start := time.Now()
		n, err := ts.Write(data)

		require.NoError(t, err)

		// Let the drain reader consume everything before we check assertions
		synctest.Wait()

		require.Equal(t, 200, n)

		// Should have taken at least 1 second (first chunk instant, second waits 1s)
		elapsed := time.Since(start)
		if elapsed < 1*time.Second {
			t.Fatalf("expected elapsed >= 1s, got %v", elapsed)
		}

		chunks, totalBytes := counter.stats()
		require.Equal(t, 2, chunks)
		require.Equal(t, 200, totalBytes)
	})
}

func TestThrottledStream_Write_HonorsWriteDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stream := newTestStream(t)
		counter := &streamWriteCounter{Stream: stream}

		// 100 bytes per second: the second chunk would have to wait a full second.
		limiter := testBpsLimiter(100, 100)
		ts := newThrottledStream(t.Context(), "", counter, limiter, func() {})
		require.NoError(t, ts.SetWriteDeadline(time.Now().Add(500*time.Millisecond)))

		start := time.Now()
		n, err := ts.Write(make([]byte, 300))
		synctest.Wait()

		// The first chunk goes out on the burst, the second fails fast instead of waiting past the deadline.
		require.NotNil(t, err)
		require.Equal(t, 100, n)
		require.Equal(t, time.Duration(0), time.Since(start))
		chunks, _ := counter.stats()
		require.Equal(t, 1, chunks)
	})
}

func TestThrottledStream_Write_ContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stream := newTestStream(t)

		// Very slow rate limiter
		limiter := testBpsLimiter(10, 10)
		ctx, cancel := context.WithCancel(t.Context())
		ts := newThrottledStream(ctx, "", stream, limiter, func() {})

		errCh := make(chan error, 1)
		go func() {
			// Try to write large amount that requires multiple rate-limited chunks
			data := make([]byte, 1000)
			_, err := ts.Write(data)
			errCh <- err
		}()

		// Let the write start
		time.Sleep(50 * time.Millisecond)
		synctest.Wait()

		// Cancel context
		cancel()

		// Write should fail with context error
		err := <-errCh
		require.NotNil(t, err)
	})
}

func TestThrottledStream_CloseCleansUp(t *testing.T) {
	cases := []struct {
		name     string
		close    func(*throttledStream) error
		closeErr error
	}{
		{name: "Close", close: (*throttledStream).Close},
		{name: "CloseWrite", close: (*throttledStream).CloseWrite},
		{name: "Close error propagates", close: (*throttledStream).Close, closeErr: errors.New("close failed")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := &errorInjectingStream{Stream: newTestStream(t), closeErr: tc.closeErr}
			cleanupCalled := atomic.Int32{}
			ts := newThrottledStream(context.Background(), "", stream, testBpsLimiter(1000, 2000), func() {
				cleanupCalled.Add(1)
			})

			err := tc.close(ts)
			if tc.closeErr != nil {
				require.ErrorIs(t, err, tc.closeErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, int32(1), cleanupCalled.Load())

			// A second close must not run cleanup again (idempotent).
			_ = tc.close(ts)
			require.Equal(t, int32(1), cleanupCalled.Load())
		})
	}
}

func TestNewBpsLimiter_ChunkClampedToBurst(t *testing.T) {
	require.Equal(t, 1000, newBpsLimiter(1000, 2000).chunk)
	require.Equal(t, 500, newBpsLimiter(1000, 500).chunk)
	require.Equal(t, 1, newBpsLimiter(0, 0).chunk)
}

func TestPeerThrottle_Wait(t *testing.T) {
	t.Run("fills all slots", func(t *testing.T) {
		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)
		streams := make([]*throttledStream, ThrottleStreamsPerPeer)

		for i := range ThrottleStreamsPerPeer {
			stream := newTestStream(t)
			ts, err := pt.wait(context.Background(), stream)
			require.NoError(t, err)
			require.NotNil(t, ts)
			streams[i] = ts
		}

		// All slots filled
		for i := range pt.streams {
			require.NotNil(t, pt.streams[i])
		}
	})

	t.Run("stream uses the long-lived throttle context, not the wait context", func(t *testing.T) {
		stream := newTestStream(t)
		waitCtx, cancel := context.WithCancel(t.Context())
		defer cancel()

		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)
		ts, err := pt.wait(waitCtx, stream)
		require.NoError(t, err)
		require.Equal(t, t.Context(), ts.ctx)
	})

	t.Run("blocks when full then unblocks on cleanup", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)

			// Fill all slots
			streams := make([]*throttledStream, ThrottleStreamsPerPeer)
			for i := range ThrottleStreamsPerPeer {
				stream := newTestStream(t)
				ts, err := pt.wait(t.Context(), stream)
				require.NoError(t, err)
				streams[i] = ts
			}

			// Start blocked wait
			result := make(chan *throttledStream, 1)
			go func() {
				stream := newTestStream(t)
				ts, _ := pt.wait(t.Context(), stream)
				result <- ts
			}()

			// Let goroutine block
			synctest.Wait()

			// Close one stream to free slot
			require.NoError(t, streams[0].Close())

			// Should unblock
			ts := <-result
			require.NotNil(t, ts)
		})
	})

	t.Run("respects context cancellation when full", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)

			// Fill all slots
			for range ThrottleStreamsPerPeer {
				stream := newTestStream(t)
				_, err := pt.wait(t.Context(), stream)
				require.NoError(t, err)
			}

			ctx, cancel := context.WithCancel(t.Context())

			// Start blocked wait
			result := make(chan error, 1)
			go func() {
				stream := newTestStream(t)
				_, err := pt.wait(ctx, stream)
				result <- err
			}()

			synctest.Wait()
			cancel()

			err := <-result
			require.ErrorIs(t, err, context.Canceled)
		})
	})
}

func TestPeerThrottle_Acquire(t *testing.T) {
	t.Run("acquireFast would block while held", func(t *testing.T) {
		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)

		release, err := pt.acquire(context.Background())
		require.NoError(t, err)
		_, err = pt.acquireFast()
		require.ErrorIs(t, err, errAcquireWouldBlock)
		release()

		release, err = pt.acquireFast()
		require.NoError(t, err)
		release()
	})

	t.Run("fails when pruned", func(t *testing.T) {
		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)
		pt.markPruned()

		_, err := pt.acquire(context.Background())
		require.ErrorIs(t, err, errPeerThrottlesPruned)
		_, err = pt.acquireFast()
		require.ErrorIs(t, err, errPeerThrottlesPruned)
	})
}

func TestPeerThrottle_CanPrune(t *testing.T) {
	t.Run("true when all streams nil", func(t *testing.T) {
		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)

		require.Equal(t, true, pt.canPrune())
	})

	t.Run("false when stream active", func(t *testing.T) {
		stream := newTestStream(t)

		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)
		_, err := pt.wait(context.Background(), stream)
		require.NoError(t, err)

		require.Equal(t, false, pt.canPrune())
	})

	t.Run("true after cleanup received", func(t *testing.T) {
		stream := newTestStream(t)

		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)
		ts, err := pt.wait(context.Background(), stream)
		require.NoError(t, err)

		// Close triggers cleanup
		require.NoError(t, ts.Close())

		// canPrune drains cleanup channel
		require.Equal(t, true, pt.canPrune())
	})
}

func TestNewPeerThrottleMux(t *testing.T) {
	cases := []struct {
		name        string
		opts        []ThrottleMuxOption
		wantEnabled bool
		wantBPS     rate.Limit
		wantBurst   int
		wantStreams int
	}{
		{name: "defaults", wantEnabled: true, wantBPS: ThrottleBPS, wantBurst: ThrottleBurst, wantStreams: ThrottleStreamsPerPeer},
		{name: "options applied", opts: []ThrottleMuxOption{WithThrottleBPS(10), WithThrottleBurst(30), WithThrottleStreamsPerPeer(3)}, wantEnabled: true, wantBPS: 10, wantBurst: 30, wantStreams: 3},
		{name: "zero bps disables", opts: []ThrottleMuxOption{WithThrottleBPS(0)}, wantBPS: 0, wantBurst: ThrottleBurst, wantStreams: ThrottleStreamsPerPeer},
		{name: "negative bps disables", opts: []ThrottleMuxOption{WithThrottleBPS(-1)}, wantBPS: -1, wantBurst: ThrottleBurst, wantStreams: ThrottleStreamsPerPeer},
		{name: "burst raised to at least bps", opts: []ThrottleMuxOption{WithThrottleBPS(1000), WithThrottleBurst(10)}, wantEnabled: true, wantBPS: 1000, wantBurst: 1000, wantStreams: ThrottleStreamsPerPeer},
		{name: "streams per peer at least one", opts: []ThrottleMuxOption{WithThrottleStreamsPerPeer(0)}, wantEnabled: true, wantBPS: ThrottleBPS, wantBurst: ThrottleBurst, wantStreams: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := newPeerThrottleMux(t.Context(), tc.opts...)
			require.Equal(t, tc.wantEnabled, mux.enabled())
			require.Equal(t, tc.wantBPS, mux.bps)
			require.Equal(t, tc.wantBurst, mux.burst)
			require.Equal(t, tc.wantStreams, mux.streamsPerPeer)
		})
	}

	t.Run("nil mux is disabled", func(t *testing.T) {
		var mux *peerThrottleMux
		require.Equal(t, false, mux.enabled())
		mux.spawnPruner() // must not panic
	})
}

func TestPeerThrottleMux_GetOrCreate(t *testing.T) {
	mux := newPeerThrottleMux(t.Context())

	pt1 := mux.getOrCreate(peer.ID("peer-1"))
	require.NotNil(t, pt1)
	require.Equal(t, pt1, mux.getOrCreate(peer.ID("peer-1")))
	require.Equal(t, 1, len(mux.throttles))

	pt2 := mux.getOrCreate(peer.ID("peer-2"))
	require.NotEqual(t, pt1, pt2)
	require.Equal(t, 2, len(mux.throttles))
}

func TestPeerThrottleMux_ThrottleSurvivesPruneRace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mux := newPeerThrottleMux(t.Context())
		pid := peer.ID("test-peer")

		// A request looked up the throttle and is now waiting for exclusive access held by the pruner.
		pt := mux.getOrCreate(pid)
		releasePruner, err := pt.acquireFast()
		require.NoError(t, err)

		type result struct {
			ts  *throttledStream
			err error
		}
		done := make(chan result, 1)
		go func() {
			ts, err := mux.throttle(t.Context(), pid, newTestStream(t))
			done <- result{ts: ts, err: err}
		}()
		synctest.Wait()

		// The pruner removes the idle throttle from the mux and releases exclusive access.
		mux.mu.Lock()
		delete(mux.throttles, pid)
		pt.markPruned()
		mux.mu.Unlock()
		releasePruner()

		// The request transparently gets a stream from a fresh throttle.
		res := <-done
		require.NoError(t, res.err)
		require.NotNil(t, res.ts)
		mux.mu.Lock()
		fresh := mux.throttles[pid]
		mux.mu.Unlock()
		require.NotNil(t, fresh)
		require.NotEqual(t, pt, fresh)
		require.Equal(t, res.ts, fresh.streams[0])
	})
}

func TestPeerThrottleMux_PrunePeer(t *testing.T) {
	t.Run("removes idle throttle", func(t *testing.T) {
		mux := newPeerThrottleMux(t.Context())
		pid := peer.ID("test-peer")

		// Create throttle
		pt := mux.getOrCreate(pid)
		require.Equal(t, 1, len(mux.throttles))

		// Prune should remove it
		mux.mu.Lock()
		mux.prunePeer(pt)
		mux.mu.Unlock()

		require.Equal(t, 0, len(mux.throttles))
		require.Equal(t, true, pt.isPruned())
	})

	t.Run("keeps active throttle", func(t *testing.T) {
		stream := newTestStream(t)

		mux := newPeerThrottleMux(t.Context())
		pid := peer.ID("test-peer")

		// Create and use throttle
		ts, err := mux.throttle(context.Background(), pid, stream)
		require.NoError(t, err)
		_ = ts // keep stream active

		// Prune should not remove it
		mux.mu.Lock()
		pt := mux.throttles[pid]
		mux.prunePeer(pt)
		mux.mu.Unlock()

		require.Equal(t, 1, len(mux.throttles))
		require.Equal(t, false, pt.isPruned())
	})
}

func TestPeerThrottleMux_SpawnPruner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		mux := newPeerThrottleMux(ctx)

		// Create idle throttle
		_ = mux.getOrCreate(peer.ID("test-peer"))
		require.Equal(t, 1, len(mux.throttles))

		mux.spawnPruner()

		// Advance time past prune interval
		time.Sleep(peerThrottlePruneInterval + time.Second)
		synctest.Wait()

		// Throttle should be pruned
		require.Equal(t, 0, len(mux.throttles))
	})
}

func TestPeerThrottleMux_SpawnPruner_StopsOnCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		mux := newPeerThrottleMux(ctx)

		mux.spawnPruner()

		// Cancel immediately
		cancel()
		synctest.Wait()

		// Create throttle after cancel
		_ = mux.getOrCreate(peer.ID("test-peer"))

		// Advance time - should not prune since pruner stopped
		time.Sleep(peerThrottlePruneInterval + time.Second)
		synctest.Wait()

		require.Equal(t, 1, len(mux.throttles))
	})
}

func TestNewService_InitializesPeerThrottle(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	s := NewService(t.Context(), WithP2P(p2p), WithThrottleMuxOptions(WithThrottleBPS(123), WithThrottleStreamsPerPeer(7)))
	require.NotNil(t, s)
	require.NotNil(t, s.peerThrottleMux)
	require.Equal(t, rate.Limit(123), s.peerThrottleMux.bps)
	require.Equal(t, 7, s.peerThrottleMux.streamsPerPeer)
	require.Equal(t, s.ctx, s.peerThrottleMux.ctx)
}
