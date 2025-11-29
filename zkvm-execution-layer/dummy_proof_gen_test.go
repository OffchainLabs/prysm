package zkvmexecutionlayer

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// newBlockHashOrRoot is a helper to create an ExecutionBlockHash or BlockRoot for tests.
func newBlockHashOrRoot(b byte) []byte {
	bytes := make([]byte, 32)
	for i := range bytes {
		bytes[i] = b
	}
	return bytes
}

// TestDummyGeneratorSuccess translates test_dummy_generator_success.
func TestDummyGeneratorSuccess(t *testing.T) {
	subnet := primitives.ExecutionProofId(0)
	generator := NewDummyProofGenerator(subnet)
	blockHash := newBlockHashOrRoot(1)
	blockRoot := newBlockHashOrRoot(2)
	slot := primitives.Slot(0)

	result, err := generator.Generate(slot, blockHash, blockRoot)
	assert.NoError(t, err)

	require.Equal(t, subnet, result.ProofId, "ProofId mismatch")
	require.DeepEqual(t, blockHash, result.BlockHash, "BlockHash mismatch")
	require.DeepEqual(t, blockRoot, result.BlockRoot, "BlockRoot mismatch")
	require.Equal(t, len(result.ProofData) > 0, true, "ProofData should be non-empty")
}

// TestDummyGeneratorDeterministic translates test_dummy_generator_deterministic.
func TestDummyGeneratorDeterministic(t *testing.T) {
	subnet := primitives.ExecutionProofId(1)
	generator := NewDummyProofGenerator(subnet)
	blockHash := newBlockHashOrRoot(42)
	blockRoot := newBlockHashOrRoot(99)
	slot := primitives.Slot(0)

	// Generate twice
	proof1, err := generator.Generate(slot, blockHash, blockRoot)
	assert.NoError(t, err)
	proof2, err := generator.Generate(slot, blockHash, blockRoot)
	assert.NoError(t, err)

	require.DeepEqual(t, proof1.ProofData, proof2.ProofData, "Proof data should be identical on repeated generation")
}

// TestDummyGeneratorCustomDelay translates test_dummy_generator_custom_delay.
func TestDummyGeneratorCustomDelay(t *testing.T) {
	subnet := primitives.ExecutionProofId(2)
	delay := 10 * time.Millisecond // Use a small but measurable delay
	generator := NewDummyProofGeneratorWithDelay(subnet, delay)
	blockHash := newBlockHashOrRoot(1)
	blockRoot := newBlockHashOrRoot(2)
	slot := primitives.Slot(0)

	start := time.Now()
	result, err := generator.Generate(slot, blockHash, blockRoot)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	elapsed := time.Since(start)
	if elapsed < delay {
		t.Errorf("Expected elapsed time to be >= %s, but got %s", delay, elapsed)
	}
}
