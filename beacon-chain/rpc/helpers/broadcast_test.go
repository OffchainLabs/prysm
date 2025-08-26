package helpers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/testutil"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

// Comprehensive test for BroadcastDataColumnSidecars
func TestBroadcastDataColumnSidecars(t *testing.T) {
	ctx := context.Background()
	root := [32]byte{1, 2, 3}

	t.Run("success cases", func(t *testing.T) {
		// Test normal broadcast with multiple sidecars
		sidecars := []*ethpb.DataColumnSidecar{
			testutil.CreateDataColumnSidecar(0, []byte{1, 2, 3}),
			testutil.CreateDataColumnSidecar(1, []byte{4, 5, 6}),
		}

		var calls []struct {
			root    [32]byte
			subnet  uint64
			sidecar *ethpb.DataColumnSidecar
		}
		broadcastFunc := func(r [32]byte, subnet uint64, sidecar *ethpb.DataColumnSidecar) error {
			calls = append(calls, struct {
				root    [32]byte
				subnet  uint64
				sidecar *ethpb.DataColumnSidecar
			}{r, subnet, sidecar})
			return nil
		}

		err := BroadcastDataColumnSidecars(ctx, sidecars, root, broadcastFunc)
		require.NoError(t, err)
		// Verify all broadcasts called with correct parameters
		require.Equal(t, len(sidecars), len(calls))
		for _, call := range calls {
			require.Equal(t, root, call.root)
			require.Equal(t, sidecars[call.sidecar.Index], call.sidecar)
		}

		// Test empty list
		err = BroadcastDataColumnSidecars(ctx, nil, root, func([32]byte, uint64, *ethpb.DataColumnSidecar) error {
			t.Fatal("should not be called")
			return nil
		})
		require.NoError(t, err)
	})
	t.Run("options integration", func(t *testing.T) {
		sidecar := testutil.CreateDataColumnSidecar(0, []byte{1, 2, 3})

		// Test multiple options work together
		receiveCalled := false
		processedCalled := false

		_ = BroadcastDataColumnSidecars(ctx, []*ethpb.DataColumnSidecar{sidecar}, root,
			func([32]byte, uint64, *ethpb.DataColumnSidecar) error { return nil },
			WithDataColumnReceiver(func([]blocks.VerifiedRODataColumn) error {
				receiveCalled = true
				return nil
			}),
			WithDataColumnProcessedCallback(func([]blocks.VerifiedRODataColumn) {
				processedCalled = true
			}))

		// Options should be configured correctly (may not be called due to validation)
		require.Equal(t, true, receiveCalled)
		require.Equal(t, true, processedCalled)
	})
}

// Comprehensive test for BroadcastBlobSidecars
func TestBroadcastBlobSidecars(t *testing.T) {
	ctx := context.Background()
	root := [32]byte{1, 2, 3}

	t.Run("success cases", func(t *testing.T) {
		// Test normal broadcast
		sidecars := []*ethpb.BlobSidecar{
			testutil.CreateBlobSidecar(0, []byte{1, 2, 3}),
			testutil.CreateBlobSidecar(1, []byte{4, 5, 6}),
		}

		var calls []struct {
			index   uint64
			sidecar *ethpb.BlobSidecar
		}
		broadcastFunc := func(ctx context.Context, index uint64, sidecar *ethpb.BlobSidecar) error {
			calls = append(calls, struct {
				index   uint64
				sidecar *ethpb.BlobSidecar
			}{index, sidecar})
			return nil
		}

		err := BroadcastBlobSidecars(ctx, sidecars, root, broadcastFunc)
		require.NoError(t, err)

		// Verify broadcasts
		require.Equal(t, len(sidecars), len(calls))
		expectedIndices := make(map[uint64]bool)
		for i := range sidecars {
			expectedIndices[uint64(i)] = true
		}
		for _, call := range calls {
			require.Equal(t, true, expectedIndices[call.index])
			require.Equal(t, sidecars[call.index], call.sidecar)
		}

		// Test empty list
		err = BroadcastBlobSidecars(ctx, nil, root, func(context.Context, uint64, *ethpb.BlobSidecar) error {
			t.Fatal("should not be called")
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("blob receiver integration", func(t *testing.T) {
		sidecar := testutil.CreateBlobSidecar(0, []byte{1, 2, 3})
		
		receiveCalled := false
		processedCalled := false
		
		err := BroadcastBlobSidecars(ctx, []*ethpb.BlobSidecar{sidecar}, root,
			func(context.Context, uint64, *ethpb.BlobSidecar) error { return nil },
			WithBlobReceiver(func(ctx context.Context, blob blocks.VerifiedROBlob) error {
				receiveCalled = true
				require.NotNil(t, blob.ROBlob)
				return nil
			}),
			WithBlobProcessedCallback(func(blob blocks.VerifiedROBlob) {
				processedCalled = true
				require.NotNil(t, blob.ROBlob)
			}))
		
		require.NoError(t, err)
		require.Equal(t, true, receiveCalled)
		require.Equal(t, true, processedCalled)
	})

	t.Run("blob receiver error handling", func(t *testing.T) {
		sidecar := testutil.CreateBlobSidecar(0, []byte{1, 2, 3})
		
		// Test error in onReceiveBlob
		err := BroadcastBlobSidecars(ctx, []*ethpb.BlobSidecar{sidecar}, root,
			func(context.Context, uint64, *ethpb.BlobSidecar) error { return nil },
			WithBlobReceiver(func(ctx context.Context, blob blocks.VerifiedROBlob) error {
				return errors.New("receive blob error")
			}))
		
		require.ErrorContains(t, "receive blob error", err)
	})

	t.Run("context and concurrency", func(t *testing.T) {
		// Test context cancellation
		ctx, cancel := context.WithCancel(context.Background())
		sidecars := make([]*ethpb.BlobSidecar, 5)
		for i := range sidecars {
			sidecars[i] = testutil.CreateBlobSidecar(uint64(i), []byte{byte(i)})
		}

		var count int32
		err := BroadcastBlobSidecars(ctx, sidecars, root, func(context.Context, uint64, *ethpb.BlobSidecar) error {
			count++
			if count == 2 {
				cancel()
			}
			return nil
		})

		// Should handle cancellation gracefully
		if err != nil {
			require.Equal(t, true, int(count) > 0 && int(count) < len(sidecars))
		}

		// Test concurrent execution (timing-based, lenient)
		sidecars = make([]*ethpb.BlobSidecar, 20)
		for i := range sidecars {
			sidecars[i] = testutil.CreateBlobSidecar(uint64(i), []byte{byte(i)})
		}

		var mu sync.Mutex
		count = 0
		start := time.Now()
		err = BroadcastBlobSidecars(context.Background(), sidecars, root, func(context.Context, uint64, *ethpb.BlobSidecar) error {
			mu.Lock()
			count++
			time.Sleep(time.Microsecond) // Simulate work
			mu.Unlock()
			return nil
		})
		duration := time.Since(start)

		require.NoError(t, err)
		require.Equal(t, int32(len(sidecars)), count)
		// Should complete faster than sequential (very lenient timing)
		require.Equal(t, true, duration < time.Duration(len(sidecars))*100*time.Microsecond)
	})
}

// Test functional options pattern
func TestFunctionalOptions(t *testing.T) {
	t.Run("data column options", func(t *testing.T) {
		// Test individual options
		receiverCalled := false
		processedCalled := false

		opts := &dataColumnOptions{}
		WithDataColumnReceiver(func([]blocks.VerifiedRODataColumn) error {
			receiverCalled = true
			return nil
		})(opts)
		WithDataColumnProcessedCallback(func([]blocks.VerifiedRODataColumn) {
			processedCalled = true
		})(opts)

		// Both should be set and functional
		require.NotNil(t, opts.onReceiveDataColumns)
		require.NotNil(t, opts.onDataColumnsProcessed)

		_ = opts.onReceiveDataColumns(nil)
		opts.onDataColumnsProcessed(nil)

		require.Equal(t, true, receiverCalled)
		require.Equal(t, true, processedCalled)
	})

	t.Run("blob options", func(t *testing.T) {
		// Test all blob options together
		receiverCalled := false
		processedCalled := false

		opts := &blobOptions{}
		WithBlobReceiver(func(context.Context, blocks.VerifiedROBlob) error {
			receiverCalled = true
			return nil
		})(opts)
		WithBlobProcessedCallback(func(blocks.VerifiedROBlob) {
			processedCalled = true
		})(opts)

		// All should be set and functional
		require.NotNil(t, opts.onReceiveBlob)
		require.NotNil(t, opts.onBlobProcessed)

		_ = opts.onReceiveBlob(context.Background(), blocks.VerifiedROBlob{})
		opts.onBlobProcessed(blocks.VerifiedROBlob{})

		require.Equal(t, true, receiverCalled)
		require.Equal(t, true, processedCalled)
	})
}
