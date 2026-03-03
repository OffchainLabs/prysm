package sync

import (
	"context"
	"fmt"
	"sync"
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

