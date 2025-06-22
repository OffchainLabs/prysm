// Package hashutil includes all hash-function related helpers for Prysm.
package hash

import (
	"errors"
	"hash"
	"reflect"
	"runtime"
	"sync"

	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/minio/highwayhash"
	"github.com/minio/sha256-simd"
	fastssz "github.com/prysmaticlabs/fastssz"
	"golang.org/x/crypto/sha3"
	"google.golang.org/protobuf/proto"
)

// ErrNilProto can occur when attempting to hash a protobuf message that is nil
// or has nil objects within lists.
var ErrNilProto = errors.New("cannot hash a nil protobuf message")

var sha256Pool = sync.Pool{New: func() interface{} {
	return sha256.New()
}}

// Hash defines a function that returns the sha256 checksum of the data passed in.
// https://github.com/ethereum/consensus-specs/blob/v0.9.3/specs/core/0_beacon-chain.md#hash
func Hash(data []byte) [32]byte {
	h, ok := sha256Pool.Get().(hash.Hash)
	if !ok {
		h = sha256.New()
	}
	defer sha256Pool.Put(h)
	h.Reset()

	var b [32]byte

	// The hash interface never returns an error, for that reason
	// we are not handling the error below. For reference, it is
	// stated here https://golang.org/pkg/hash/#Hash

	// #nosec G104
	h.Write(data)
	h.Sum(b[:0])

	return b
}

// CustomSHA256Hasher returns a hash function that uses
// an enclosed hasher. This is not safe for concurrent
// use as the same hasher is being called throughout.
//
// Note: that this method is only more performant over
// hashutil.Hash if the callback is used more than 5 times.
// 
// The returned function automatically returns the hasher to the pool
// when it goes out of scope (via the finalizer).
func CustomSHA256Hasher() func([]byte) [32]byte {
	hasher, ok := sha256Pool.Get().(hash.Hash)
	if !ok {
		hasher = sha256.New()
	} else {
		hasher.Reset()
	}
	var h [32]byte

	// Create a reference counter to track usage
	refCount := new(int32)
	*refCount = 1

	// Create the hash function
	hashFunc := func(data []byte) [32]byte {
		// The hash interface never returns an error, for that reason
		// we are not handling the error below. For reference, it is
		// stated here https://golang.org/pkg/hash/#Hash

		// #nosec G104
		hasher.Write(data)
		hasher.Sum(h[:0])
		hasher.Reset()

		return h
	}

	// Create a finalizer that will return the hasher to the pool when garbage collected
	runtime.SetFinalizer(refCount, func(_ interface{}) {
		sha256Pool.Put(hasher)
	})

	return hashFunc
}

// NewReusableSHA256Hasher returns a reusable SHA256 hasher that must be manually
// returned to the pool using the returned cleanup function when no longer needed.
// This is useful for cases where you need more control over the lifecycle of the hasher.
//
// Usage example:
//
//	hasher, cleanup := hash.NewReusableSHA256Hasher()
//	defer cleanup()
//	
//	hash1 := hasher(data1)
//	hash2 := hasher(data2)
//	// ...
func NewReusableSHA256Hasher() (func([]byte) [32]byte, func()) {
	hasher, ok := sha256Pool.Get().(hash.Hash)
	if !ok {
		hasher = sha256.New()
	} else {
		hasher.Reset()
	}
	var h [32]byte

	hashFunc := func(data []byte) [32]byte {
		hasher.Reset()
		// #nosec G104
		hasher.Write(data)
		hasher.Sum(h[:0])
		return h
	}

	cleanup := func() {
		sha256Pool.Put(hasher)
	}

	return hashFunc, cleanup
}

var keccak256Pool = sync.Pool{New: func() interface{} {
	return sha3.NewLegacyKeccak256()
}}

// Keccak256 defines a function which returns the Keccak-256/SHA3
// hash of the data passed in.
func Keccak256(data []byte) [32]byte {
	var b [32]byte

	h, ok := keccak256Pool.Get().(hash.Hash)
	if !ok {
		h = sha3.NewLegacyKeccak256()
	}
	defer keccak256Pool.Put(h)
	h.Reset()

	// The hash interface never returns an error, for that reason
	// we are not handling the error below. For reference, it is
	// stated here https://golang.org/pkg/hash/#Hash

	// #nosec G104
	h.Write(data)
	h.Sum(b[:0])

	return b
}

// Proto hashes a protocol buffer message using sha256.
func Proto(msg proto.Message) (result [32]byte, err error) {
	if msg == nil || reflect.ValueOf(msg).IsNil() {
		return [32]byte{}, ErrNilProto
	}
	var data []byte
	if m, ok := msg.(fastssz.Marshaler); ok {
		data, err = m.MarshalSSZ()
	} else {
		data, err = proto.Marshal(msg)
	}
	if err != nil {
		return [32]byte{}, err
	}
	return Hash(data), nil
}

// Key used for FastSum64
var fastSumHashKey = bytesutil.ToBytes32([]byte("hash_fast_sum64_key"))

// FastSum64 returns a hash sum of the input data using highwayhash. This method is not secure, but
// may be used as a quick identifier for objects where collisions are acceptable.
func FastSum64(data []byte) uint64 {
	return highwayhash.Sum64(data, fastSumHashKey[:])
}

// FastSum256 returns a hash sum of the input data using highwayhash. This method is not secure, but
// may be used as a quick identifier for objects where collisions are acceptable.
func FastSum256(data []byte) [32]byte {
	return highwayhash.Sum(data, fastSumHashKey[:])
}
