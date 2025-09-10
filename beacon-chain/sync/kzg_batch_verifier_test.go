package sync

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func TestValidateWithKzgBatchVerifier(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	tests := []struct {
		name           string
		dataColumns    []blocks.RODataColumn
		expectedResult pubsub.ValidationResult
		expectError    bool
	}{
		{
			name:           "single valid data column",
			dataColumns:    createValidTestDataColumns(t, 1),
			expectedResult: pubsub.ValidationAccept,
			expectError:    false,
		},
		{
			name:           "multiple valid data columns",
			dataColumns:    createValidTestDataColumns(t, 3),
			expectedResult: pubsub.ValidationAccept,
			expectError:    false,
		},
		{
			name:           "single invalid data column",
			dataColumns:    createInvalidTestDataColumns(t, 1),
			expectedResult: pubsub.ValidationReject,
			expectError:    true,
		},
		{
			name:           "empty data column slice",
			dataColumns:    []blocks.RODataColumn{},
			expectedResult: pubsub.ValidationAccept,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			service := &Service{
				ctx:     ctx,
				kzgChan: make(chan *kzgVerifier, 100),
			}
			go service.kzgVerifierRoutine()

			result, err := service.validateWithKzgBatchVerifier(ctx, tt.dataColumns)

			require.Equal(t, tt.expectedResult, result)
			if tt.expectError {
				assert.NotNil(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVerifierRoutine(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	t.Run("processes single request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		service := &Service{
			ctx:     ctx,
			kzgChan: make(chan *kzgVerifier, 100),
		}
		go service.kzgVerifierRoutine()

		dataColumns := createValidTestDataColumns(t, 1)
		resChan := make(chan error, 1)
		service.kzgChan <- &kzgVerifier{dataColumns: dataColumns, resChan: resChan}

		select {
		case err := <-resChan:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for verification result")
		}
	})

	t.Run("batches multiple requests", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		service := &Service{
			ctx:     ctx,
			kzgChan: make(chan *kzgVerifier, 100),
		}
		go service.kzgVerifierRoutine()

		const numRequests = 5
		resChans := make([]chan error, numRequests)

		for i := range numRequests {
			dataColumns := createValidTestDataColumns(t, 1)
			resChan := make(chan error, 1)
			resChans[i] = resChan
			service.kzgChan <- &kzgVerifier{dataColumns: dataColumns, resChan: resChan}
		}

		for i := range numRequests {
			select {
			case err := <-resChans[i]:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatalf("timeout waiting for verification result %d", i)
			}
		}
	})

	t.Run("context cancellation stops routine", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		service := &Service{
			ctx:     ctx,
			kzgChan: make(chan *kzgVerifier, 100),
		}

		routineDone := make(chan struct{})
		go func() {
			service.kzgVerifierRoutine()
			close(routineDone)
		}()

		cancel()

		select {
		case <-routineDone:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for routine to exit")
		}
	})
}

func TestVerifyKzgBatch(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	t.Run("all valid data columns succeed", func(t *testing.T) {
		dataColumns := createValidTestDataColumns(t, 3)
		resChan := make(chan error, 1)
		kzgVerifiers := []*kzgVerifier{{dataColumns: dataColumns, resChan: resChan}}

		verifyKzgBatch(kzgVerifiers)

		select {
		case err := <-resChan:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for batch verification")
		}
	})

	t.Run("invalid proofs fail entire batch", func(t *testing.T) {
		validColumns := createValidTestDataColumns(t, 1)
		invalidColumns := createInvalidTestDataColumns(t, 1)
		allColumns := append(validColumns, invalidColumns...)

		resChan := make(chan error, 1)
		kzgVerifiers := []*kzgVerifier{{dataColumns: allColumns, resChan: resChan}}

		verifyKzgBatch(kzgVerifiers)

		select {
		case err := <-resChan:
			assert.NotNil(t, err)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for batch verification")
		}
	})

	t.Run("empty batch handling", func(t *testing.T) {
		verifyKzgBatch([]*kzgVerifier{})
	})
}

func TestKzgBatchVerifierConcurrency(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	service := &Service{
		ctx:     ctx,
		kzgChan: make(chan *kzgVerifier, 100),
	}
	go service.kzgVerifierRoutine()

	const numGoroutines = 10
	const numRequestsPerGoroutine = 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Multiple goroutines sending verification requests simultaneously
	for i := range numGoroutines {
		go func(goroutineID int) {
			defer wg.Done()

			for range numRequestsPerGoroutine {
				dataColumns := createValidTestDataColumns(t, 1)
				result, err := service.validateWithKzgBatchVerifier(ctx, dataColumns)
				require.Equal(t, pubsub.ValidationAccept, result)
				require.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()
}

func TestKzgBatchVerifierFallback(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	t.Run("fallback handles mixed valid/invalid batch correctly", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		service := &Service{
			ctx:     ctx,
			kzgChan: make(chan *kzgVerifier, 100),
		}
		go service.kzgVerifierRoutine()

		validColumns := createValidTestDataColumns(t, 1)
		invalidColumns := createInvalidTestDataColumns(t, 1)

		result, err := service.validateWithKzgBatchVerifier(ctx, validColumns)
		require.Equal(t, pubsub.ValidationAccept, result)
		require.NoError(t, err)

		result, err = service.validateWithKzgBatchVerifier(ctx, invalidColumns)
		require.Equal(t, pubsub.ValidationReject, result)
		assert.NotNil(t, err)
	})

	t.Run("empty data columns fallback", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		service := &Service{
			ctx:     ctx,
			kzgChan: make(chan *kzgVerifier, 100),
		}
		go service.kzgVerifierRoutine()

		result, err := service.validateWithKzgBatchVerifier(ctx, []blocks.RODataColumn{})
		require.Equal(t, pubsub.ValidationAccept, result)
		require.NoError(t, err)
	})
}

func createValidTestDataColumns(t *testing.T, count int) []blocks.RODataColumn {
	_, roSidecars, _ := util.GenerateTestFuluBlockWithSidecars(t, count)
	if len(roSidecars) >= count {
		return roSidecars[:count]
	}
	return roSidecars
}

func createInvalidTestDataColumns(t *testing.T, count int) []blocks.RODataColumn {
	dataColumns := createValidTestDataColumns(t, count)

	if len(dataColumns) > 0 {
		sidecar := dataColumns[0].DataColumnSidecar
		if len(sidecar.Column) > 0 && len(sidecar.Column[0]) > 0 {
			corruptedSidecar := &ethpb.DataColumnSidecar{
				Index:                        sidecar.Index,
				KzgCommitments:               make([][]byte, len(sidecar.KzgCommitments)),
				KzgProofs:                    make([][]byte, len(sidecar.KzgProofs)),
				KzgCommitmentsInclusionProof: make([][]byte, len(sidecar.KzgCommitmentsInclusionProof)),
				SignedBlockHeader:            sidecar.SignedBlockHeader,
				Column:                       make([][]byte, len(sidecar.Column)),
			}

			for i, commitment := range sidecar.KzgCommitments {
				corruptedSidecar.KzgCommitments[i] = make([]byte, len(commitment))
				copy(corruptedSidecar.KzgCommitments[i], commitment)
			}

			for i, proof := range sidecar.KzgProofs {
				corruptedSidecar.KzgProofs[i] = make([]byte, len(proof))
				copy(corruptedSidecar.KzgProofs[i], proof)
			}

			for i, proof := range sidecar.KzgCommitmentsInclusionProof {
				corruptedSidecar.KzgCommitmentsInclusionProof[i] = make([]byte, len(proof))
				copy(corruptedSidecar.KzgCommitmentsInclusionProof[i], proof)
			}

			for i, col := range sidecar.Column {
				corruptedSidecar.Column[i] = make([]byte, len(col))
				copy(corruptedSidecar.Column[i], col)
			}
			corruptedSidecar.Column[0][0] ^= 0xFF // Flip bits to corrupt

			corruptedRO, err := blocks.NewRODataColumn(corruptedSidecar)
			require.NoError(t, err)
			dataColumns[0] = corruptedRO
		}
	}
	return dataColumns
}
