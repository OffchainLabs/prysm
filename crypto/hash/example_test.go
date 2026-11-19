package hash_test

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v6/crypto/hash"
)

func ExampleHash() {
	data := []byte("example data")
	h := hash.Hash(data)
	fmt.Printf("%x\n", h[:])
	// Output is a SHA-256 hash, will be different for each input
}

func ExampleCustomSHA256Hasher() {
	// Create a custom hasher for multiple operations
	hasher := hash.CustomSHA256Hasher()

	// Use the hasher multiple times
	data1 := []byte("data1")
	data2 := []byte("data2")

	h1 := hasher(data1)
	h2 := hasher(data2)

	fmt.Printf("Hash 1: %x\n", h1[:])
	fmt.Printf("Hash 2: %x\n", h2[:])
	// Output is two SHA-256 hashes
}

func ExampleNewReusableSHA256Hasher() {
	// Create a reusable hasher with explicit cleanup
	hasher, cleanup := hash.NewReusableSHA256Hasher()
	defer cleanup() // Important: always call cleanup to return the hasher to the pool

	// Use the hasher for multiple operations in a tight loop
	for i := 0; i < 3; i++ {
		data := []byte(fmt.Sprintf("data%d", i))
		h := hasher(data)
		fmt.Printf("Hash %d: %x\n", i, h[:])
	}
	// Output is three SHA-256 hashes
}

// Example showing how to use NewReusableSHA256Hasher in a high-performance scenario
func Example_highPerformanceHashing() {
	// Create a reusable hasher that will be used in a hot path
	hasher, cleanup := hash.NewReusableSHA256Hasher()
	defer cleanup()

	// Simulate processing a batch of data
	batchSize := 5
	results := make([][32]byte, batchSize)

	for i := 0; i < batchSize; i++ {
		// In a real application, this would be actual data to hash
		data := []byte(fmt.Sprintf("batch item %d", i))
		
		// Use the same hasher for all items in the batch
		results[i] = hasher(data)
	}

	// Print the first few bytes of each hash
	for i, result := range results {
		fmt.Printf("Item %d hash prefix: %x\n", i, result[:4])
	}
	// Output is hash prefixes for each batch item
} 