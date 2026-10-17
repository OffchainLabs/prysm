package doublylinkedtree

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// setupExecutionProofForkchoice returns a forkchoice where the block `full` has a revealed payload, and the block
// `empty` (built on top of it) has none.
func setupExecutionProofForkchoice(t *testing.T) (f *ForkChoice, full, empty [32]byte) {
	t.Helper()
	f = setupGloas(t, 0, 0)
	ctx := t.Context()

	full, empty = indexToHash(1), indexToHash(2)

	st, roblock, err := prepareGloasForkchoiceState(ctx, 1, full, params.BeaconConfig().ZeroHash, indexToHash(100), params.BeaconConfig().ZeroHash, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))
	pe, err := prepareGloasForkchoicePayload(full)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayload(pe))

	st, roblock, err = prepareGloasForkchoiceState(ctx, 2, empty, full, indexToHash(200), indexToHash(100), 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, roblock))

	return f, full, empty
}

func TestSetPayloadExecutionValid(t *testing.T) {
	t.Run("full node", func(t *testing.T) {
		f, full, _ := setupExecutionProofForkchoice(t)
		executionValid, _ := f.PayloadValidationStatus(full)
		require.Equal(t, false, executionValid)

		require.NoError(t, f.SetPayloadExecutionValid(full))
		executionValid, _ = f.PayloadValidationStatus(full)
		require.Equal(t, true, executionValid)

		// The EL validation alone does not make the payload valid.
		optimistic, err := f.IsOptimistic(full)
		require.NoError(t, err)
		require.Equal(t, true, optimistic)
	})

	t.Run("no full node", func(t *testing.T) {
		f, _, empty := setupExecutionProofForkchoice(t)
		require.ErrorIs(t, f.SetPayloadExecutionValid(empty), ErrNilNode)
		executionValid, _ := f.PayloadValidationStatus(empty)
		require.Equal(t, false, executionValid)
	})
}

func TestIsExecutionValid(t *testing.T) {
	t.Run("empty block uses the payload it builds on", func(t *testing.T) {
		f, full, empty := setupExecutionProofForkchoice(t)
		require.Equal(t, false, f.IsExecutionValid(empty))

		require.NoError(t, f.SetPayloadExecutionValid(full))
		require.Equal(t, true, f.IsExecutionValid(full))
		require.Equal(t, true, f.IsExecutionValid(empty))
	})

	t.Run("EL validity propagates to ancestors", func(t *testing.T) {
		f, full, child := setupExecutionProofForkchoice(t)
		pe, err := prepareGloasForkchoicePayload(child)
		require.NoError(t, err)
		require.NoError(t, f.InsertPayload(pe))

		require.NoError(t, f.SetPayloadExecutionValid(child))
		executionValid, _ := f.PayloadValidationStatus(full)
		require.Equal(t, true, executionValid)
	})

	t.Run("unknown block", func(t *testing.T) {
		f, _, _ := setupExecutionProofForkchoice(t)
		require.Equal(t, false, f.IsExecutionValid(indexToHash(99)))
	})
}

func TestPayloadInsertionTime(t *testing.T) {
	t.Run("full node", func(t *testing.T) {
		before := time.Now()
		f, full, _ := setupExecutionProofForkchoice(t)
		insertedAt, ok := f.PayloadInsertionTime(full)
		require.Equal(t, true, ok)
		require.Equal(t, false, insertedAt.Before(before))
		require.Equal(t, false, insertedAt.After(time.Now()))
	})

	t.Run("no full node", func(t *testing.T) {
		f, _, empty := setupExecutionProofForkchoice(t)
		_, ok := f.PayloadInsertionTime(empty)
		require.Equal(t, false, ok)
	})
}

func TestSetNewPayloadRequestRoot(t *testing.T) {
	t.Run("full node", func(t *testing.T) {
		f, full, _ := setupExecutionProofForkchoice(t)
		_, ok := f.NewPayloadRequestRoot(full)
		require.Equal(t, false, ok)

		require.NoError(t, f.SetNewPayloadRequestRoot(full, indexToHash(42)))
		root, ok := f.NewPayloadRequestRoot(full)
		require.Equal(t, true, ok)
		require.Equal(t, indexToHash(42), root)
	})

	t.Run("no full node", func(t *testing.T) {
		f, _, empty := setupExecutionProofForkchoice(t)
		require.ErrorIs(t, f.SetNewPayloadRequestRoot(empty, indexToHash(42)), ErrNilNode)
		_, ok := f.NewPayloadRequestRoot(empty)
		require.Equal(t, false, ok)
	})
}

func TestAddExecutionProofType(t *testing.T) {
	t.Run("full node", func(t *testing.T) {
		f, full, _ := setupExecutionProofForkchoice(t)
		requireProofTypes(t, f, full, 0)

		require.NoError(t, f.AddExecutionProofType(full, ethpb.ProofTypeEthrexSP1))
		require.Equal(t, true, f.HasExecutionProofType(full, ethpb.ProofTypeEthrexSP1))
		require.Equal(t, false, f.HasExecutionProofType(full, ethpb.ProofTypeEthrexZisk))
		requireProofTypes(t, f, full, 1)

		// A second proof of the same type does not count.
		require.ErrorIs(t, f.AddExecutionProofType(full, ethpb.ProofTypeEthrexSP1), forkchoice.ErrExecutionProofTypeKnown)
		requireProofTypes(t, f, full, 1)

		require.NoError(t, f.AddExecutionProofType(full, ethpb.ProofTypeEthrexZisk))
		requireProofTypes(t, f, full, 2)
	})

	t.Run("no full node", func(t *testing.T) {
		f, _, empty := setupExecutionProofForkchoice(t)
		require.ErrorIs(t, f.AddExecutionProofType(empty, ethpb.ProofTypeEthrexSP1), ErrNilNode)
		require.Equal(t, false, f.HasExecutionProofType(empty, ethpb.ProofTypeEthrexSP1))
		requireProofTypes(t, f, empty, 0)
	})
}

func requireProofTypes(t *testing.T, f *ForkChoice, root [32]byte, expected int) {
	t.Helper()
	_, proofTypes := f.PayloadValidationStatus(root)
	require.Equal(t, expected, proofTypes)
}
