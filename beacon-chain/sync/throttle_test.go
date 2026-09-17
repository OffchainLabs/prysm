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

	prysmP2P "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
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
			name:       "exact bps size",
			bps:        1000,
			burst:      2000,
			writeSize:  1000,
			wantChunks: 1,
			wantTotal:  1000,
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
			name:       "exactly two chunks",
			bps:        1000,
			burst:      2000,
			writeSize:  2000,
			wantChunks: 2,
			wantTotal:  2000,
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

func TestThrottledStream_Write_BurstSmallerThanBPS(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stream := newTestStream(t)
		counter := &streamWriteCounter{Stream: stream}

		// A burst smaller than bps must not make writes larger than the burst fail:
		// chunks are clamped to the burst.
		limiter := testBpsLimiter(1000, 500)
		require.Equal(t, 500, limiter.chunk)
		ts := newThrottledStream(t.Context(), "", counter, limiter, func() {})

		n, err := ts.Write(make([]byte, 1200))
		require.NoError(t, err)
		synctest.Wait()

		require.Equal(t, 1200, n)
		chunks, totalBytes := counter.stats()
		require.Equal(t, 3, chunks)
		require.Equal(t, 1200, totalBytes)
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

		// Clearing the deadline makes writes wait for tokens again.
		require.NoError(t, ts.SetWriteDeadline(time.Time{}))
		start = time.Now()
		n, err = ts.Write(make([]byte, 100))
		synctest.Wait()
		require.NoError(t, err)
		require.Equal(t, 100, n)
		if elapsed := time.Since(start); elapsed < time.Second {
			t.Fatalf("expected elapsed >= 1s, got %v", elapsed)
		}
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

func TestThrottledStream_Close(t *testing.T) {
	t.Run("close succeeds", func(t *testing.T) {
		stream := newTestStream(t)

		limiter := testBpsLimiter(1000, 2000)
		cleanupCalled := atomic.Int32{}
		ts := newThrottledStream(context.Background(), "", stream, limiter, func() {
			cleanupCalled.Add(1)
		})

		err := ts.Close()

		require.NoError(t, err)
		require.Equal(t, int32(1), cleanupCalled.Load())

		// Second close should not call cleanup again (idempotent)
		_ = ts.Close()
		require.Equal(t, int32(1), cleanupCalled.Load())
	})

	t.Run("close error propagates", func(t *testing.T) {
		stream := newTestStream(t)

		errStream := &errorInjectingStream{
			Stream:   stream,
			closeErr: errors.New("close failed"),
		}
		limiter := testBpsLimiter(1000, 2000)
		cleanupCalled := atomic.Int32{}
		ts := newThrottledStream(context.Background(), "", errStream, limiter, func() {
			cleanupCalled.Add(1)
		})

		err := ts.Close()

		require.NotNil(t, err)
		require.Equal(t, int32(1), cleanupCalled.Load())
	})
}

func TestThrottledStream_CloseWrite(t *testing.T) {
	stream := newTestStream(t)

	limiter := testBpsLimiter(1000, 2000)
	cleanupCalled := atomic.Int32{}
	ts := newThrottledStream(context.Background(), "", stream, limiter, func() {
		cleanupCalled.Add(1)
	})

	err := ts.CloseWrite()

	require.NoError(t, err)
	require.Equal(t, int32(1), cleanupCalled.Load())

	// Second call should not call cleanup again
	_ = ts.CloseWrite()
	require.Equal(t, int32(1), cleanupCalled.Load())
}

func TestNewBpsLimiter_ChunkClampedToBurst(t *testing.T) {
	require.Equal(t, 1000, newBpsLimiter(1000, 2000).chunk)
	require.Equal(t, 500, newBpsLimiter(1000, 500).chunk)
	require.Equal(t, 1, newBpsLimiter(0, 0).chunk)
}

func TestPeerThrottle_Wait(t *testing.T) {
	t.Run("returns stream when slot available", func(t *testing.T) {
		stream := newTestStream(t)

		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)
		ts, err := pt.wait(context.Background(), stream)

		require.NoError(t, err)
		require.NotNil(t, ts)
	})

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
	t.Run("succeeds with context", func(t *testing.T) {
		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)

		release, err := pt.acquire(context.Background())

		require.NoError(t, err)
		require.NotNil(t, release)
		release()
	})

	t.Run("would block with nil context", func(t *testing.T) {
		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)

		// Acquire first
		release1, err := pt.acquire(context.Background())
		require.NoError(t, err)

		// Try non-blocking acquire
		_, err = pt.acquireFast()

		require.ErrorIs(t, err, errAcquireWouldBlock)
		release1()
	})

	t.Run("fails when pruned", func(t *testing.T) {
		pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)
		pt.markPruned()

		_, err := pt.acquire(context.Background())

		require.Equal(t, errPeerThrottlesPruned, err)
	})

	t.Run("fails when pruned while waiting for exclusive access", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			pt := newPeerThrottle(t.Context(), "", 100, 200, ThrottleStreamsPerPeer)

			// The pruner holds exclusive access...
			releasePruner, err := pt.acquireFast()
			require.NoError(t, err)

			// ...while a request is waiting for it.
			result := make(chan error, 1)
			go func() {
				_, err := pt.acquire(t.Context())
				result <- err
			}()
			synctest.Wait()

			// The pruner prunes the throttle and releases exclusive access.
			pt.markPruned()
			releasePruner()

			// The waiter must notice the prune rather than use the orphaned throttle.
			require.ErrorIs(t, <-result, errPeerThrottlesPruned)

			// And it must have handed exclusive access back.
			select {
			case <-pt.exclusive:
				pt.exclusive <- struct{}{}
			default:
				t.Fatal("exclusive access was not released")
			}
		})
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
	t.Run("defaults", func(t *testing.T) {
		mux := newPeerThrottleMux(t.Context())
		require.Equal(t, ThrottleBPS, mux.bps)
		require.Equal(t, ThrottleBurst, mux.burst)
		require.Equal(t, ThrottleStreamsPerPeer, mux.streamsPerPeer)
		require.Equal(t, true, mux.enabled())
	})

	t.Run("options applied", func(t *testing.T) {
		mux := newPeerThrottleMux(t.Context(), WithThrottleBPS(10), WithThrottleBurst(30), WithThrottleStreamsPerPeer(3))
		require.Equal(t, rate.Limit(10), mux.bps)
		require.Equal(t, 30, mux.burst)
		require.Equal(t, 3, mux.streamsPerPeer)
	})

	t.Run("bps of zero disables throttling", func(t *testing.T) {
		require.Equal(t, false, newPeerThrottleMux(t.Context(), WithThrottleBPS(0)).enabled())
		require.Equal(t, false, newPeerThrottleMux(t.Context(), WithThrottleBPS(-1)).enabled())
	})

	t.Run("nil mux is disabled", func(t *testing.T) {
		var mux *peerThrottleMux
		require.Equal(t, false, mux.enabled())
		mux.spawnPruner() // must not panic
	})

	t.Run("burst is raised to at least bps", func(t *testing.T) {
		mux := newPeerThrottleMux(t.Context(), WithThrottleBPS(1000), WithThrottleBurst(10))
		require.Equal(t, 1000, mux.burst)
	})

	t.Run("streams per peer is at least one", func(t *testing.T) {
		mux := newPeerThrottleMux(t.Context(), WithThrottleStreamsPerPeer(0))
		require.Equal(t, 1, mux.streamsPerPeer)
	})
}

func TestPeerThrottleMux_Get(t *testing.T) {
	t.Run("creates new throttle", func(t *testing.T) {
		mux := newPeerThrottleMux(t.Context())
		pid := peer.ID("test-peer")

		pt := mux.getOrCreate(pid)

		require.NotNil(t, pt)
		require.Equal(t, 1, len(mux.throttles))
	})

	t.Run("returns same throttle", func(t *testing.T) {
		mux := newPeerThrottleMux(t.Context())
		pid := peer.ID("test-peer")

		pt1 := mux.getOrCreate(pid)
		pt2 := mux.getOrCreate(pid)

		require.Equal(t, pt1, pt2)
		require.Equal(t, 1, len(mux.throttles))
	})

	t.Run("different peers get different throttles", func(t *testing.T) {
		mux := newPeerThrottleMux(t.Context())

		pt1 := mux.getOrCreate(peer.ID("peer-1"))
		pt2 := mux.getOrCreate(peer.ID("peer-2"))

		require.NotEqual(t, pt1, pt2)
		require.Equal(t, 2, len(mux.throttles))
	})

	t.Run("replaces a pruned throttle", func(t *testing.T) {
		mux := newPeerThrottleMux(t.Context())
		pid := peer.ID("test-peer")

		pt1 := mux.getOrCreate(pid)
		pt1.markPruned()
		pt2 := mux.getOrCreate(pid)

		require.NotEqual(t, pt1, pt2)
		require.Equal(t, 1, len(mux.throttles))
	})
}

func TestPeerThrottleMux_WaitForWriter(t *testing.T) {
	stream := newTestStream(t)

	mux := newPeerThrottleMux(t.Context())
	pid := peer.ID("test-peer")

	ts, err := mux.throttle(context.Background(), pid, stream)

	require.NoError(t, err)
	require.NotNil(t, ts)
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

func TestPeerThrottleMux_Prune(t *testing.T) {
	mux := newPeerThrottleMux(t.Context())

	// Create multiple peers
	for i := range 5 {
		pid := peer.ID(string(rune('a' + i)))
		_ = mux.getOrCreate(pid)
	}
	require.Equal(t, 5, len(mux.throttles))

	// Prune all idle throttles
	mux.prune()

	require.Equal(t, 0, len(mux.throttles))
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

func TestSafeDone(t *testing.T) {
	t.Run("done blocks until close", func(t *testing.T) {
		sd := newSafeDone()

		select {
		case <-sd.Done():
			t.Fatal("Done() should block before Close()")
		default:
			// expected
		}

		sd.Close()

		select {
		case <-sd.Done():
			// expected
		default:
			t.Fatal("Done() should not block after Close()")
		}
	})

	t.Run("close is idempotent", func(t *testing.T) {
		sd := newSafeDone()

		// Multiple closes should not panic
		sd.Close()
		sd.Close()
		sd.Close()

		select {
		case <-sd.Done():
			// expected
		default:
			t.Fatal("Done() should not block after Close()")
		}
	})

	t.Run("unblocks multiple waiters", func(t *testing.T) {
		sd := newSafeDone()
		const numWaiters = 5
		unblocked := atomic.Int32{}

		for range numWaiters {
			go func() {
				<-sd.Done()
				unblocked.Add(1)
			}()
		}

		// Give goroutines time to start
		time.Sleep(10 * time.Millisecond)
		require.Equal(t, int32(0), unblocked.Load())

		sd.Close()

		// Give goroutines time to unblock
		time.Sleep(10 * time.Millisecond)
		require.Equal(t, int32(numWaiters), unblocked.Load())
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

// waitFor polls cond until it is true or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// throttleTestTopic registers handler on a throttled (bounded) topic and returns a function that sends a
// request on that topic from remotePeer, mirroring how p2p.Send closes the write side after the request.
func throttleTestTopic(t *testing.T, r *Service, p2p, remotePeer *p2ptest.TestP2P, handler rpcHandler) func() network.Stream {
	t.Helper()
	topic := "/testing/throttle/1"
	prysmP2P.RPCTopicMappings[topic] = new(ethpb.Fork)
	t.Cleanup(func() { delete(prysmP2P.RPCTopicMappings, topic) })
	r.registerRPC(topic, handler)

	return func() network.Stream {
		t.Helper()
		stream, err := remotePeer.Host().NewStream(t.Context(), p2p.BHost.ID(), protocol.ID(topic+p2p.Encoding().ProtocolSuffix()))
		require.NoError(t, err)
		_, err = p2p.Encoding().EncodeWithMaxLength(stream, &ethpb.Fork{CurrentVersion: []byte("fooo"), PreviousVersion: []byte("barr")})
		require.NoError(t, err)
		require.NoError(t, stream.CloseWrite())
		return stream
	}
}

func TestRegisterRPC_ThrottledResponseIsCompleteAndNotPenalized(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	remotePeer := p2ptest.NewTestP2P(t)
	remotePeer.Connect(p2p)

	const burst = 8 * 1024
	mux := newPeerThrottleMux(t.Context(), WithThrottleBPS(burst), WithThrottleBurst(burst), WithThrottleStreamsPerPeer(1))
	r := &Service{
		ctx:             t.Context(),
		cfg:             &config{p2p: p2p},
		rateLimiter:     newRateLimiter(p2p),
		peerThrottleMux: mux,
	}

	// The response is three bursts: one goes out immediately, the rest take two seconds at bps.
	payload := make([]byte, 3*burst)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	var wrapped atomic.Bool
	handler := func(_ context.Context, _ any, stream libp2pcore.Stream) error {
		_, ok := stream.(*throttledStream)
		wrapped.Store(ok)
		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if _, err := stream.Write(payload); err != nil {
			return err
		}
		closeStream(stream, log)
		return nil
	}
	send := throttleTestTopic(t, r, p2p, remotePeer, handler)

	start := time.Now()
	stream := send()
	got, err := io.ReadAll(stream)
	require.NoError(t, err)
	elapsed := time.Since(start)

	require.DeepEqual(t, payload, got)
	require.Equal(t, true, wrapped.Load(), "handler did not receive a throttled stream")
	if elapsed < 1500*time.Millisecond {
		t.Fatalf("response was not throttled: took %v, expected at least 2s", elapsed)
	}
	require.Equal(t, 0, p2p.PeerScoring().BadResponseCount(remotePeer.BHost.ID()), "throttled peer was penalized")
}

func TestRegisterRPC_ThrottleSlotContentionIsNotPenalized(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	remotePeer := p2ptest.NewTestP2P(t)
	remotePeer.Connect(p2p)

	mux := newPeerThrottleMux(t.Context(), WithThrottleStreamsPerPeer(1))
	r := &Service{
		ctx:             t.Context(),
		cfg:             &config{p2p: p2p},
		rateLimiter:     newRateLimiter(p2p),
		peerThrottleMux: mux,
	}

	release := make(chan struct{})
	var handled atomic.Int32
	handler := func(_ context.Context, _ any, stream libp2pcore.Stream) error {
		if handled.Add(1) == 1 {
			// The first request holds the peer's only stream slot until released.
			<-release
		}
		if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
			return err
		}
		closeStream(stream, log)
		return nil
	}
	send := throttleTestTopic(t, r, p2p, remotePeer, handler)

	first := send()
	waitFor(t, 5*time.Second, func() bool { return handled.Load() == 1 }, "first request to reach the handler")

	// The second request must wait for the slot: while it waits it holds the throttle's exclusive token.
	second := send()
	pt := mux.getOrCreate(remotePeer.BHost.ID())
	waitFor(t, 5*time.Second, func() bool {
		rel, err := pt.acquireFast()
		if err == nil {
			rel()
			return false
		}
		return errors.Is(err, errAcquireWouldBlock)
	}, "second request to wait for a stream slot")
	require.Equal(t, int32(1), handled.Load(), "second request was served while the slot was busy")

	// Releasing the first request hands the slot to the second one; both complete.
	close(release)
	expectSuccess(t, first)
	expectSuccess(t, second)
	waitFor(t, 5*time.Second, func() bool { return handled.Load() == 2 }, "second request to reach the handler")

	require.Equal(t, 0, p2p.PeerScoring().BadResponseCount(remotePeer.BHost.ID()), "waiting peer was penalized")
}

func TestRegisterRPC_ThrottleWaitAbortedIsNotPenalized(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	remotePeer := p2ptest.NewTestP2P(t)
	remotePeer.Connect(p2p)

	// The service context bounds the slot wait; the mux context (which outlives it here) bounds the writes.
	svcCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	mux := newPeerThrottleMux(t.Context(), WithThrottleStreamsPerPeer(1))
	r := &Service{
		ctx:             svcCtx,
		cfg:             &config{p2p: p2p},
		rateLimiter:     newRateLimiter(p2p),
		peerThrottleMux: mux,
	}

	release := make(chan struct{})
	var handled atomic.Int32
	handler := func(_ context.Context, _ any, stream libp2pcore.Stream) error {
		handled.Add(1)
		<-release
		if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil {
			return err
		}
		closeStream(stream, log)
		return nil
	}
	send := throttleTestTopic(t, r, p2p, remotePeer, handler)

	first := send()
	waitFor(t, 5*time.Second, func() bool { return handled.Load() == 1 }, "first request to reach the handler")

	second := send()
	pt := mux.getOrCreate(remotePeer.BHost.ID())
	waitFor(t, 5*time.Second, func() bool {
		rel, err := pt.acquireFast()
		if err == nil {
			rel()
			return false
		}
		return errors.Is(err, errAcquireWouldBlock)
	}, "second request to wait for a stream slot")

	// Giving up on the slot wait resets the second stream without ever running its handler...
	cancel()
	expectResetStream(t, second)
	require.Equal(t, int32(1), handled.Load())

	// ...and without penalizing the peer, whose first response still completes.
	close(release)
	expectSuccess(t, first)
	require.Equal(t, 0, p2p.PeerScoring().BadResponseCount(remotePeer.BHost.ID()), "throttled peer was penalized")
}

func TestRegisterRPC_UnboundedTopicsSkipThrottle(t *testing.T) {
	for _, topic := range []string{
		prysmP2P.RPCStatusTopicV1, prysmP2P.RPCStatusTopicV2, prysmP2P.RPCGoodByeTopicV1, prysmP2P.RPCPingTopicV1,
		prysmP2P.RPCMetaDataTopicV1, prysmP2P.RPCMetaDataTopicV2, prysmP2P.RPCMetaDataTopicV3,
	} {
		_, ok := unboundedTopics[topic]
		require.Equal(t, true, ok, "%s should not be throttled", topic)
	}
	for _, topic := range []string{
		prysmP2P.RPCBlocksByRangeTopicV2, prysmP2P.RPCBlocksByRootTopicV2, prysmP2P.RPCBlobSidecarsByRangeTopicV1,
		prysmP2P.RPCDataColumnSidecarsByRangeTopicV1, prysmP2P.RPCExecutionPayloadEnvelopesByRangeTopicV1,
	} {
		_, ok := unboundedTopics[topic]
		require.Equal(t, false, ok, "%s should be throttled", topic)
	}
}
