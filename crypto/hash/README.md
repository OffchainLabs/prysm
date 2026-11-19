# Hash Package

This package provides optimized cryptographic hash functions for the Prysm Ethereum Consensus client.

## Features

- Optimized SHA-256 hashing using SIMD instructions via the `github.com/minio/sha256-simd` package
- Memory-efficient hashing with object pooling
- Keccak-256 (Ethereum) hashing
- Fast, non-cryptographic hashing via HighwayHash
- Protobuf message hashing

## Usage

### Basic Hashing

For simple one-off hashing, use the `Hash` function:

```go
import "github.com/OffchainLabs/prysm/v6/crypto/hash"

data := []byte("data to hash")
h := hash.Hash(data)  // Returns a [32]byte hash
```

### Performance Optimizations

For performance-critical code that performs multiple hash operations in sequence, use one of the following approaches:

#### CustomSHA256Hasher

Use `CustomSHA256Hasher` when you need to hash multiple items in a row and want to avoid allocating a new hasher for each operation:

```go
hasher := hash.CustomSHA256Hasher()
hash1 := hasher(data1)
hash2 := hasher(data2)
// The hasher will be automatically returned to the pool when garbage collected
```

#### NewReusableSHA256Hasher

Use `NewReusableSHA256Hasher` when you need explicit control over the hasher's lifecycle:

```go
hasher, cleanup := hash.NewReusableSHA256Hasher()
defer cleanup()  // Important: always call cleanup to return the hasher to the pool

// Use the hasher multiple times
hash1 := hasher(data1)
hash2 := hasher(data2)
```

This approach is particularly useful in high-performance scenarios where you want to ensure the hasher is returned to the pool as soon as possible.

### Other Hash Functions

#### Keccak-256

For Ethereum-compatible Keccak-256 hashing:

```go
h := hash.Keccak256(data)
```

#### Fast Non-Cryptographic Hashing

For high-performance, non-cryptographic hashing:

```go
sum64 := hash.FastSum64(data)  // 64-bit hash
sum256 := hash.FastSum256(data)  // 256-bit hash
```

#### Protobuf Message Hashing

For hashing protobuf messages:

```go
hash, err := hash.Proto(protoMsg)
if err != nil {
    // Handle error
}
```

## Performance Considerations

- `Hash` is suitable for most use cases and is safe for concurrent use
- `CustomSHA256Hasher` is more efficient when hashing multiple items (5+ times) in sequence
- `NewReusableSHA256Hasher` provides the best performance for batch operations with explicit control over the hasher lifecycle 