package sync

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	lruwrpr "github.com/OffchainLabs/prysm/v7/cache/lru"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// failThenSucceedReconstructor fails the first failCount calls to ReconstructBlobSidecars,
// then returns the configured BlobSidecars on subsequent calls.
type failThenSucceedReconstructor struct {
	mockExecution.EngineClient
	mu        sync.Mutex
	calls     int
	failCount int
}

func (m *failThenSucceedReconstructor) ReconstructBlobSidecars(
	_ context.Context,
	_ interfaces.ReadOnlySignedBeaconBlock,
	_ [fieldparams.RootLength]byte,
	_ func(uint64) bool,
) ([]blocks.VerifiedROBlob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls <= m.failCount {
		return nil, fmt.Errorf("EL temporarily unavailable")
	}
	return m.BlobSidecars, nil
}

// blockingReconstructor blocks ReconstructBlobSidecars until the release channel is closed.
// Tracks the number of actual EL calls via an atomic counter.
type blockingReconstructor struct {
	mockExecution.EngineClient
	calls   atomic.Int32
	release chan struct{}
}

func (m *blockingReconstructor) ReconstructBlobSidecars(
	ctx context.Context,
	_ interfaces.ReadOnlySignedBeaconBlock,
	_ [fieldparams.RootLength]byte,
	_ func(uint64) bool,
) ([]blocks.VerifiedROBlob, error) {
	m.calls.Add(1)
	select {
	case <-m.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return m.BlobSidecars, nil
}

func newTestBlobSidecar(t *testing.T) blocks.VerifiedROBlob {
	rob, err := blocks.NewROBlob(&ethpb.BlobSidecar{
		SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
			Header: &ethpb.BeaconBlockHeader{
				ParentRoot: make([]byte, 32),
				BodyRoot:   make([]byte, 32),
				StateRoot:  make([]byte, 32),
			},
			Signature: []byte("signature"),
		},
	})
	require.NoError(t, err)
	return blocks.VerifiedROBlob{ROBlob: rob}
}

func newDenebBlockWithCommitments(t *testing.T, n int) interfaces.ReadOnlySignedBeaconBlock {
	b := util.NewBeaconBlockDeneb()
	b.Block.Body.BlobKzgCommitments = make([][]byte, n)
	for i := range n {
		b.Block.Body.BlobKzgCommitments[i] = make([]byte, 48)
	}
	sb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	return sb
}

// TestProcessBlobSidecarsFromExecution_RetryOnELError verifies that transient EL failures
// are retried and the function eventually succeeds.
func TestProcessBlobSidecarsFromExecution_RetryOnELError(t *testing.T) {
	mock := &failThenSucceedReconstructor{
		failCount: 2,
		EngineClient: mockExecution.EngineClient{
			BlobSidecars: []blocks.VerifiedROBlob{newTestBlobSidecar(t)},
		},
	}

	s := &Service{
		cfg: &config{
			p2p:                    mockp2p.NewTestP2P(t),
			chain:                  &chainMock.ChainService{Genesis: time.Now()},
			clock:                  startup.NewClock(time.Now(), [32]byte{}),
			blobStorage:            filesystem.NewEphemeralBlobStorage(t),
			executionReconstructor: mock,
			operationNotifier:      &chainMock.MockOperationNotifier{},
		},
		seenBlobCache: lruwrpr.New(1),
	}

	// Bound with timeout so a retry regression can't hang CI.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	s.processBlobSidecarsFromExecution(ctx, newDenebBlockWithCommitments(t, 1))

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.Equal(t, 3, mock.calls) // 2 failures + 1 success
}

// TestProcessBlobSidecarsFromExecution_ContextCancelStopsRetry verifies that the retry loop
// exits cleanly when the context is cancelled, rather than looping indefinitely.
func TestProcessBlobSidecarsFromExecution_ContextCancelStopsRetry(t *testing.T) {
	s := &Service{
		cfg: &config{
			p2p:         mockp2p.NewTestP2P(t),
			chain:       &chainMock.ChainService{Genesis: time.Now()},
			clock:       startup.NewClock(time.Now(), [32]byte{}),
			blobStorage: filesystem.NewEphemeralBlobStorage(t),
			executionReconstructor: &mockExecution.EngineClient{
				ErrorBlobSidecars: fmt.Errorf("EL always fails"),
			},
			operationNotifier: &chainMock.MockOperationNotifier{},
		},
		seenBlobCache: lruwrpr.New(1),
	}

	ctx, cancel := context.WithTimeout(t.Context(), 400*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.processBlobSidecarsFromExecution(ctx, newDenebBlockWithCommitments(t, 1))
		close(done)
	}()

	select {
	case <-done:
		// Loop exited after context cancellation.
	case <-time.After(3 * time.Second):
		t.Fatal("processBlobSidecarsFromExecution did not return after context cancellation")
	}
}

// TestProcessBlobSidecarsFromExecution_SingleFlightDedup verifies that concurrent calls
// for the same block root are deduplicated via singleFlight, resulting in only one EL call.
func TestProcessBlobSidecarsFromExecution_SingleFlightDedup(t *testing.T) {
	mock := &blockingReconstructor{release: make(chan struct{})}

	s := &Service{
		cfg: &config{
			p2p:                    mockp2p.NewTestP2P(t),
			chain:                  &chainMock.ChainService{Genesis: time.Now()},
			clock:                  startup.NewClock(time.Now(), [32]byte{}),
			blobStorage:            filesystem.NewEphemeralBlobStorage(t),
			executionReconstructor: mock,
			operationNotifier:      &chainMock.MockOperationNotifier{},
		},
		seenBlobCache: lruwrpr.New(1),
	}

	// Use two separate-but-identical blocks to avoid sharing mutable state across goroutines.
	sb1 := newDenebBlockWithCommitments(t, 1)
	sb2 := newDenebBlockWithCommitments(t, 1)
	r1, err := sb1.Block().HashTreeRoot()
	require.NoError(t, err)
	r2, err := sb2.Block().HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, r1, r2, "blocks must have the same root for singleFlight dedup")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.processBlobSidecarsFromExecution(t.Context(), sb1) }()
	go func() { defer wg.Done(); s.processBlobSidecarsFromExecution(t.Context(), sb2) }()

	// Wait until at least one EL call is in flight before releasing.
	require.Eventually(t, func() bool { return mock.calls.Load() >= 1 }, time.Second, 5*time.Millisecond)
	close(mock.release)
	wg.Wait()

	require.Equal(t, int32(1), mock.calls.Load())
}
