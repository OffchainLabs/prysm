package htr

import (
	"crypto/rand"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func Test_VectorizedSha256(t *testing.T) {
	largeSlice := make([][32]byte, 32*minSliceSizeToParallelize)
	secondLargeSlice := make([][32]byte, 32*minSliceSizeToParallelize)
	hash1 := make([][32]byte, 16*minSliceSizeToParallelize)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		tempHash := VectorizedSha256(largeSlice)
		copy(hash1, tempHash)
	}()
	wg.Wait()
	hash2 := VectorizedSha256(secondLargeSlice)
	require.Equal(t, len(hash1), len(hash2))
	for i, r := range hash1 {
		require.Equal(t, r, hash2[i])
	}
}

// generateTestData creates random test data for hashing tests
func generateTestData(size int) [][32]byte {
	data := make([][32]byte, size)
	for i := range data {
		_, err := rand.Read(data[i][:])
		if err != nil {
			panic(err) // This should never happen in tests
		}
	}
	return data
}

// Test_GohashtreeVsHashtree verifies both implementations produce identical results
func Test_GohashtreeVsHashtree(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"small", 100},
		{"medium", 1000},
		{"large", 10000},
		{"very_large", 50000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate test data (must be even number for hash pairs)
			input := generateTestData(tt.size * 2)

			// Test with gohashtree (default)
			SetUseHashtree(false)
			gohashtreeResult := VectorizedSha256(input)

			// Test with hashtree
			SetUseHashtree(true)
			hashtreeResult := VectorizedSha256(input)

			// Reset to default
			SetUseHashtree(false)

			// Results should be identical
			require.Equal(t, len(gohashtreeResult), len(hashtreeResult), "Result lengths should match")
			for i := range gohashtreeResult {
				require.Equal(t, gohashtreeResult[i], hashtreeResult[i], "Hash results should be identical at index %d", i)
			}
		})
	}
}

// Test_GohashtreeImplementation tests gohashtree specifically
func Test_GohashtreeImplementation(t *testing.T) {
	// Force gohashtree
	SetUseHashtree(false)
	defer SetUseHashtree(false) // Reset after test

	require.Equal(t, false, GetUseHashtree(), "Should be using gohashtree")

	// Test small input (non-parallel path)
	smallInput := generateTestData(10)
	smallResult := VectorizedSha256(smallInput)
	require.Equal(t, 5, len(smallResult), "Small input should produce correct number of hashes")

	// Test large input (parallel path)
	largeInput := generateTestData(minSliceSizeToParallelize + 100)
	largeResult := VectorizedSha256(largeInput)
	expectedLen := (minSliceSizeToParallelize + 100) / 2
	require.Equal(t, expectedLen, len(largeResult), "Large input should produce correct number of hashes")
}

// Test_HashtreeImplementation tests hashtree specifically
func Test_HashtreeImplementation(t *testing.T) {
	// Force hashtree
	SetUseHashtree(true)
	defer SetUseHashtree(false) // Reset after test

	require.Equal(t, true, GetUseHashtree(), "Should be using hashtree")

	// Test small input
	smallInput := generateTestData(10)
	smallResult := VectorizedSha256(smallInput)
	require.Equal(t, 5, len(smallResult), "Small input should produce correct number of hashes")

	// Test large input
	largeInput := generateTestData(minSliceSizeToParallelize + 100)
	largeResult := VectorizedSha256(largeInput)
	expectedLen := (minSliceSizeToParallelize + 100) / 2
	require.Equal(t, expectedLen, len(largeResult), "Large input should produce correct number of hashes")
}

// Test_ThreadSafety verifies both implementations work correctly with concurrent access
func Test_ThreadSafety(t *testing.T) {
	tests := []struct {
		name        string
		useHashtree bool
	}{
		{"gohashtree_concurrent", false},
		{"hashtree_concurrent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetUseHashtree(tt.useHashtree)
			defer SetUseHashtree(false)

			const numGoroutines = 10
			const inputSize = 1000

			results := make([][][32]byte, numGoroutines)
			wg := sync.WaitGroup{}

			// Run concurrent hashing
			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					input := generateTestData(inputSize)
					results[index] = VectorizedSha256(input)
				}(i)
			}
			wg.Wait()

			// Verify all results have correct length
			expectedLen := inputSize / 2
			for i, result := range results {
				require.Equal(t, expectedLen, len(result), "Result %d should have correct length", i)
			}
		})
	}
}

// Benchmark_GohashtreeSmall benchmarks gohashtree with small input
func Benchmark_GohashtreeSmall(b *testing.B) {
	SetUseHashtree(false)
	input := generateTestData(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VectorizedSha256(input)
	}
}

// Benchmark_HashtreeSmall benchmarks hashtree with small input
func Benchmark_HashtreeSmall(b *testing.B) {
	SetUseHashtree(true)
	defer SetUseHashtree(false)
	input := generateTestData(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VectorizedSha256(input)
	}
}

// Benchmark_GohashtreeMedium benchmarks gohashtree with medium input
func Benchmark_GohashtreeMedium(b *testing.B) {
	SetUseHashtree(false)
	input := generateTestData(2000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VectorizedSha256(input)
	}
}

// Benchmark_HashtreeMedium benchmarks hashtree with medium input
func Benchmark_HashtreeMedium(b *testing.B) {
	SetUseHashtree(true)
	defer SetUseHashtree(false)
	input := generateTestData(2000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VectorizedSha256(input)
	}
}

// Benchmark_GohashtreeLarge benchmarks gohashtree with large input (parallel path)
func Benchmark_GohashtreeLarge(b *testing.B) {
	SetUseHashtree(false)
	input := generateTestData(minSliceSizeToParallelize + 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VectorizedSha256(input)
	}
}

// Benchmark_HashtreeLarge benchmarks hashtree with large input (parallel path)
func Benchmark_HashtreeLarge(b *testing.B) {
	SetUseHashtree(true)
	defer SetUseHashtree(false)
	input := generateTestData(minSliceSizeToParallelize + 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VectorizedSha256(input)
	}
}

// Benchmark_GohashtreeVeryLarge benchmarks gohashtree with very large input
func Benchmark_GohashtreeVeryLarge(b *testing.B) {
	SetUseHashtree(false)
	input := generateTestData(50000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VectorizedSha256(input)
	}
}

// Benchmark_HashtreeVeryLarge benchmarks hashtree with very large input
func Benchmark_HashtreeVeryLarge(b *testing.B) {
	SetUseHashtree(true)
	defer SetUseHashtree(false)
	input := generateTestData(50000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VectorizedSha256(input)
	}
}

// Benchmark_Comparison runs both implementations side by side for direct comparison
func Benchmark_Comparison(b *testing.B) {
	input := generateTestData(10000)

	b.Run("gohashtree", func(b *testing.B) {
		SetUseHashtree(false)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = VectorizedSha256(input)
		}
	})

	b.Run("hashtree", func(b *testing.B) {
		SetUseHashtree(true)
		defer SetUseHashtree(false)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = VectorizedSha256(input)
		}
	})
}
