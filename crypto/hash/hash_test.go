package hash_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/crypto/hash"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	pb "github.com/OffchainLabs/prysm/v6/proto/testing"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	fuzz "github.com/google/gofuzz"
)

func TestHash(t *testing.T) {
	hashOf0 := [32]byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29}
	h := hash.Hash([]byte{0})
	assert.Equal(t, hashOf0, h)

	hashOf1 := [32]byte{75, 245, 18, 47, 52, 69, 84, 197, 59, 222, 46, 187, 140, 210, 183, 227, 209, 96, 10, 214, 49, 195, 133, 165, 215, 204, 226, 60, 119, 133, 69, 154}
	h = hash.Hash([]byte{1})
	assert.Equal(t, hashOf1, h)
	assert.Equal(t, false, hashOf0 == hashOf1)
}

func BenchmarkHash(b *testing.B) {
	for i := 0; i < b.N; i++ {
		hash.Hash([]byte("abc"))
	}
}

func TestHashKeccak256(t *testing.T) {
	hashOf0 := [32]byte{188, 54, 120, 158, 122, 30, 40, 20, 54, 70, 66, 41, 130, 143, 129, 125, 102, 18, 247, 180, 119, 214, 101, 145, 255, 150, 169, 224, 100, 188, 201, 138}
	h := hash.Keccak256([]byte{0})
	assert.Equal(t, hashOf0, h)

	hashOf1 := [32]byte{95, 231, 249, 119, 231, 29, 186, 46, 161, 166, 142, 33, 5, 123, 238, 187, 155, 226, 172, 48, 198, 65, 10, 163, 141, 79, 63, 190, 65, 220, 255, 210}
	h = hash.Keccak256([]byte{1})
	assert.Equal(t, hashOf1, h)
	assert.Equal(t, false, hashOf0 == hashOf1)

	// Same hashing test from go-ethereum for keccak256
	hashOfabc, err := hex.DecodeString("4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45")
	require.NoError(t, err)
	h = hash.Keccak256([]byte("abc"))
	h32 := bytesutil.ToBytes32(hashOfabc)
	assert.Equal(t, h32, h)
}

func BenchmarkHashKeccak256(b *testing.B) {
	for i := 0; i < b.N; i++ {
		hash.Keccak256([]byte("abc"))
	}
}

func TestHashProto(t *testing.T) {
	msg1 := &pb.Puzzle{
		Challenge: "hello",
	}
	msg2 := &pb.Puzzle{
		Challenge: "hello",
	}
	h1, err := hash.Proto(msg1)
	require.NoError(t, err)
	h2, err := hash.Proto(msg2)
	require.NoError(t, err)
	assert.Equal(t, h1, h2)
}

func TestHashProtoFuzz(t *testing.T) {
	f := fuzz.New().NilChance(.2)
	defer func(tt *testing.T) {
		if r := recover(); r != nil {
			t.Log("cannot hash nil proto")
		}
	}(t)

	for i := 0; i < 1000; i++ {
		msg := &pb.AddressBook{}
		f.Fuzz(msg)
		_, err := hash.Proto(msg)
		_ = err
	}
}

func BenchmarkHashProto(b *testing.B) {
	att := &ethpb.Attestation{
		AggregationBits: nil,
		Data: &ethpb.AttestationData{
			Slot:            5,
			CommitteeIndex:  3,
			BeaconBlockRoot: []byte{},
			Source:          nil,
			Target:          nil,
		},
		Signature: bls.NewAggregateSignature().Marshal(),
	}

	for i := 0; i < b.N; i++ {
		if _, err := hash.Proto(att); err != nil {
			b.Log(err)
		}
	}
}

// TestNewReusableSHA256Hasher tests the NewReusableSHA256Hasher function.
func TestNewReusableSHA256Hasher(t *testing.T) {
	hasher, cleanup := hash.NewReusableSHA256Hasher()
	defer cleanup()

	// Test multiple hashes with the same hasher
	h1 := hasher([]byte("test1"))
	h2 := hasher([]byte("test2"))

	// Verify results are different
	if h1 == h2 {
		t.Error("Expected different hashes for different inputs")
	}

	// Compare with standard Hash function
	stdH1 := hash.Hash([]byte("test1"))
	stdH2 := hash.Hash([]byte("test2"))

	if h1 != stdH1 {
		t.Errorf("Reusable hasher produced different result than standard Hash function: %x != %x", h1, stdH1)
	}

	if h2 != stdH2 {
		t.Errorf("Reusable hasher produced different result than standard Hash function: %x != %x", h2, stdH2)
	}
}

// TestCustomSHA256HasherPerformance tests the performance of CustomSHA256Hasher compared to Hash.
func TestCustomSHA256HasherPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	data := []byte("performance test data")
	iterations := 100000

	// Test standard Hash function
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_ = hash.Hash(data)
	}
	standardDuration := time.Since(start)

	// Test CustomSHA256Hasher
	start = time.Now()
	hasher := hash.CustomSHA256Hasher()
	for i := 0; i < iterations; i++ {
		_ = hasher(data)
	}
	customDuration := time.Since(start)

	// Test NewReusableSHA256Hasher
	start = time.Now()
	reusableHasher, cleanup := hash.NewReusableSHA256Hasher()
	for i := 0; i < iterations; i++ {
		_ = reusableHasher(data)
	}
	cleanup()
	reusableDuration := time.Since(start)

	t.Logf("Standard Hash: %v for %d iterations", standardDuration, iterations)
	t.Logf("CustomSHA256Hasher: %v for %d iterations", customDuration, iterations)
	t.Logf("NewReusableSHA256Hasher: %v for %d iterations", reusableDuration, iterations)
	
	// The custom hasher should be faster than the standard Hash function
	if customDuration >= standardDuration {
		t.Logf("Warning: CustomSHA256Hasher was not faster than standard Hash function")
	}
	
	// The reusable hasher should be similar in performance to the custom hasher
	if float64(reusableDuration) > float64(customDuration)*1.1 { // Allow 10% overhead
		t.Logf("Warning: NewReusableSHA256Hasher was significantly slower than CustomSHA256Hasher")
	}
}

// BenchmarkHashWithLargerData benchmarks the Hash function with larger data.
func BenchmarkHashWithLargerData(b *testing.B) {
	data := []byte("benchmark test data")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hash.Hash(data)
	}
}

// BenchmarkCustomSHA256Hasher benchmarks the CustomSHA256Hasher function.
func BenchmarkCustomSHA256Hasher(b *testing.B) {
	data := []byte("benchmark test data")
	hasher := hash.CustomSHA256Hasher()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hasher(data)
	}
}

// BenchmarkNewReusableSHA256Hasher benchmarks the NewReusableSHA256Hasher function.
func BenchmarkNewReusableSHA256Hasher(b *testing.B) {
	data := []byte("benchmark test data")
	hasher, cleanup := hash.NewReusableSHA256Hasher()
	defer cleanup()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hasher(data)
	}
}
