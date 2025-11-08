package zkvmexecutionlayer

import (
	"bytes"
	"testing"
	"time"

	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/ethereum/go-ethereum/common"
)

// newBlockHashOrRoot is a helper to create an ExecutionBlockHash or BlockRoot for tests.
func newBlockHashOrRoot(b byte) common.Hash {
	var hash common.Hash
	for i := range hash {
		hash[i] = b
	}
	return hash
}

// TestDummyGeneratorSuccess translates test_dummy_generator_success.
func TestDummyGeneratorSuccess(t *testing.T) {
	subnet, _ := executionproof.NewExecutionProofId(0)
	generator := NewDummyProofGenerator(subnet)
	blockHash := newBlockHashOrRoot(1)
	blockRoot := newBlockHashOrRoot(2)
	slot := primitives.Slot(0)

	result, err := generator.Generate(slot, blockHash, blockRoot)

	if err != nil {
		t.Fatalf("Expected result to be ok, but got error: %v", err)
	}

	if result.ProofId != subnet {
		t.Errorf("Expected subnet %s, got %s", subnet, result.ProofId)
	}
	if result.BlockHash != blockHash {
		t.Errorf("Expected block hash %s, got %s", blockHash, result.BlockHash)
	}
	if result.BlockRoot != blockRoot {
		t.Errorf("Expected block root %s, got %s", blockRoot, result.BlockRoot)
	}
	if result.ProofDataSize() <= 0 {
		t.Error("Expected proof data size to be > 0")
	}
}

// TestDummyGeneratorDeterministic translates test_dummy_generator_deterministic.
func TestDummyGeneratorDeterministic(t *testing.T) {
	subnet, _ := executionproof.NewExecutionProofId(1)
	generator := NewDummyProofGenerator(subnet)
	blockHash := newBlockHashOrRoot(42)
	blockRoot := newBlockHashOrRoot(99)
	slot := primitives.Slot(0)

	// Generate twice
	proof1, err1 := generator.Generate(slot, blockHash, blockRoot)
	if err1 != nil {
		t.Fatalf("First generation failed: %v", err1)
	}

	proof2, err2 := generator.Generate(slot, blockHash, blockRoot)
	if err2 != nil {
		t.Fatalf("Second generation failed: %v", err2)
	}

	// Should be identical
	if !bytes.Equal(proof1.ProofDataSlice(), proof2.ProofDataSlice()) {
		t.Error("Expected proof data to be identical, but it differed")
	}
}

// TestDummyGeneratorCustomDelay translates test_dummy_generator_custom_delay.
func TestDummyGeneratorCustomDelay(t *testing.T) {
	subnet, _ := executionproof.NewExecutionProofId(0)
	delay := 10 * time.Millisecond // Use a small but measurable delay
	generator := NewDummyProofGeneratorWithDelay(subnet, delay)
	blockHash := newBlockHashOrRoot(1)
	blockRoot := newBlockHashOrRoot(2)
	slot := primitives.Slot(0)

	start := time.Now()
	result, err := generator.Generate(slot, blockHash, blockRoot)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if result == nil {
		t.Fatal("Got nil proof")
	}

	if elapsed < delay {
		t.Errorf("Expected elapsed time to be >= %s, but got %s", delay, elapsed)
	}
}