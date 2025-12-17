package sync

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

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
		sidecar := dataColumns[0].DataColumnSidecar()
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

func TestVerifierRoutine(t *testing.T) {
	err := kzg.Start()
	require.NoError(t, err)

	t.Run("processes single request", func(t *testing.T) {
		ctx := t.Context()

		service := &Service{
			ctx:     ctx,
			kzgChan: make(chan *kzgVerifier, 100),
		}
		go service.kzgVerifierRoutine()

		dataColumns := createValidTestDataColumns(t, 1)
		resChan := make(chan errorWithSegment, 1)
		service.kzgChan <- &kzgVerifier{sizeHint: 1, cellProofs: blocks.RODataColumnsToCellProofBundles(dataColumns), resChan: resChan}

		select {
		case errWithSegment := <-resChan:
			require.NoError(t, errWithSegment.err)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for verification result")
		}
	})

	t.Run("batches multiple requests", func(t *testing.T) {
		ctx := t.Context()

		service := &Service{
			ctx:     ctx,
			kzgChan: make(chan *kzgVerifier, 100),
		}
		go service.kzgVerifierRoutine()

		const numRequests = 5
		resChans := make([]chan errorWithSegment, numRequests)

		for i := range numRequests {
			dataColumns := createValidTestDataColumns(t, 1)
			resChan := make(chan errorWithSegment, 1)
			resChans[i] = resChan
			service.kzgChan <- &kzgVerifier{sizeHint: 1, cellProofs: blocks.RODataColumnsToCellProofBundles(dataColumns), resChan: resChan}
		}

		for i := range numRequests {
			select {
			case errWithSegment := <-resChans[i]:
				require.NoError(t, errWithSegment.err)
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
