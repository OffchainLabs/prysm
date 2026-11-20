package stateutil

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"sync"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const (
	// Number of fields in the validator object.
	validatorFieldRoots = 8

	// Depth of an individual validator tree.
	validatorTreeDepth = 3

	// Leaves per validator tree.
	validatorLeaves = 1 << validatorTreeDepth
)

// ValidatorRegistryRoot computes the HashTreeRoot Merkleization of
// a list of compact validator structs according to the Ethereum
// Simple Serialize specification.
func ValidatorRegistryRoot(stateVersion int, vals []CompactValidator) ([32]byte, error) {
	if stateVersion >= version.Gloas {
		return validatorRegistryRootProgressive(vals)
	}
	return validatorRegistryRoot(vals)
}

func validatorRegistryRootProgressive(vals []CompactValidator) ([32]byte, error) {
	body := ssz.MerkleizeProgressiveChunks(OptimizedValidatorRoots(vals))
	var length [32]byte
	binary.LittleEndian.PutUint64(length[:8], uint64(len(vals)))
	return ssz.MixInLength(body, length[:]), nil
}

func validatorRegistryRoot(validators []CompactValidator) ([32]byte, error) {
	roots := OptimizedValidatorRoots(validators)

	validatorsRootsRoot, err := ssz.BitwiseMerkleize(roots, uint64(len(roots)), fieldparams.ValidatorRegistryLimit)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute validator registry merkleization")
	}
	validatorsRootsBuf := new(bytes.Buffer)
	if err := binary.Write(validatorsRootsBuf, binary.LittleEndian, uint64(len(validators))); err != nil {
		return [32]byte{}, errors.Wrap(err, "could not marshal validator registry length")
	}
	// We need to mix in the length of the slice.
	var validatorsRootsBufRoot [32]byte
	copy(validatorsRootsBufRoot[:], validatorsRootsBuf.Bytes())
	res := ssz.MixInLength(validatorsRootsRoot, validatorsRootsBufRoot[:])

	return res, nil
}

// OptimizedValidatorRoots returns the hash tree root of each compact validator.
func OptimizedValidatorRoots(validators []CompactValidator) [][32]byte {
	return merkleizeInPlace(len(validators), validatorTreeDepth, func(i int, leaves [][32]byte) {
		// Cannot panic: len(leaves) >= validatorFieldRoots.
		validators[i].fieldRootsInto((*[validatorFieldRoots][32]byte)(leaves))
	})
}

// merkleizeInPlace returns the roots of n trees of the given depth. fill
// writes the leaves of tree i. Leaves that fill does not write stay zero.
//
// Each group hashes its trees by level in place. After all groups finish,
// the roots move to the front (n=5, 2 groups):
//
//	leaves:    | t0 | t1 | t2 | t3 | t4 |   each ti is 2^depth leaves
//	           \--- group 0 ---/\ grp 1 /
//	by level:  | r0 r1 r2 ...  | r3 r4 ...|
//	moved:     | r0 r1 r2 r3 r4 ...       |  -> roots[:n:n]
func merkleizeInPlace(n, depth int, fill func(i int, leaves [][32]byte)) [][32]byte {
	// Exit early if no validators are provided.
	if n == 0 {
		return [][32]byte{}
	}

	leaves := 1 << depth
	rootsSize := n * leaves
	roots := make([][32]byte, rootsSize)
	procs := runtime.GOMAXPROCS(0)
	groupSize := (n + procs - 1) / procs

	var wg sync.WaitGroup
	for start := 0; start < n; start += groupSize {
		end := min(start+groupSize, n)
		wg.Go(func() {
			for i := start; i < end; i++ {
				fill(i, roots[i*leaves:(i+1)*leaves])
			}
			// Hash each level into the front half of the previous one.
			// The group's roots end up at the front of r.
			r := roots[start*leaves : end*leaves]
			for range depth {
				htr.HashChunks(r[:len(r)/2], r)
				r = r[:len(r)/2]
			}
		})
	}
	wg.Wait()

	// Move each group's roots to roots[start:end]. Group 0's are already in place.
	for start := groupSize; start < n; start += groupSize {
		end := min(start+groupSize, n)
		copy(roots[start:end], roots[start*leaves:start*leaves+end-start])
	}
	return roots[:n:n]
}
