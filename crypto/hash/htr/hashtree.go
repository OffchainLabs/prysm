package htr

import (
	"runtime"
	"sync"

	"github.com/OffchainLabs/hashtree"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/prysmaticlabs/gohashtree"
)

const minSliceSizeToParallelize = 5000

// Hash hashes chunks pairwise into digests using the configured hashing library.
// It performs input validation (odd chunks, digest length).
func Hash(digests, chunks [][32]byte) error {
	if features.Get().EnableHashtree {
		return hashtree.Hash(digests, chunks)
	}
	return gohashtree.Hash(digests, chunks)
}

// HashChunks hashes chunks pairwise into digests without error checking.
// The caller must ensure inputs are valid (even chunks, sufficient digest space).
func HashChunks(digests, chunks [][32]byte) {
	if features.Get().EnableHashtree {
		if err := hashtree.Hash(digests, chunks); err != nil {
			log.WithError(err).Error("Could not hash chunks")
		}
	} else {
		gohashtree.HashChunks(digests, chunks)
	}
}

func hashParallel(inputList [][32]byte, outputList [][32]byte, wg *sync.WaitGroup) {
	defer wg.Done()
	err := Hash(outputList, inputList)
	if err != nil {
		panic(err) // lint:nopanic -- This should never panic.
	}
}

// VectorizedSha256 takes a list of roots and hashes them using CPU
// specific vector instructions. Depending on host machine's specific
// hardware configuration, using this routine can lead to a significant
// performance improvement compared to the default method of hashing
// lists.
func VectorizedSha256(inputList [][32]byte) [][32]byte {
	outputList := make([][32]byte, len(inputList)/2)
	if len(inputList) < minSliceSizeToParallelize {
		err := Hash(outputList, inputList)
		if err != nil {
			panic(err) // lint:nopanic -- This should never panic.
		}
		return outputList
	}
	n := runtime.GOMAXPROCS(0) - 1
	wg := sync.WaitGroup{}
	wg.Add(n)
	groupSize := len(inputList) / (2 * (n + 1))
	for j := range n {
		go hashParallel(inputList[j*2*groupSize:(j+1)*2*groupSize], outputList[j*groupSize:], &wg)
	}
	err := Hash(outputList[n*groupSize:], inputList[n*2*groupSize:])
	if err != nil {
		panic(err) // lint:nopanic -- This should never panic.
	}
	wg.Wait()
	return outputList
}
