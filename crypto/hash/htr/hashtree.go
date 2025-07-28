package htr

import (
	"runtime"
	"sync"

	"github.com/prysmaticlabs/gohashtree"
	hashtreelib "github.com/prysmaticlabs/hashtree"
)

const minSliceSizeToParallelize = 5000

// HashFunc defines the interface for vectorized hash implementations
type HashFunc func(output [][32]byte, input [][32]byte) error

var (
	// currentHashFunc holds the active hash implementation
	currentHashFunc HashFunc = gohashtree.Hash

	// useHashtree flag determines which implementation to use
	useHashtree bool = false
)

// SetUseHashtree configures whether to use the hashtree library (true) or gohashtree (false)
func SetUseHashtree(use bool) {
	useHashtree = use
	if use {
		currentHashFunc = hashtreelib.Hash
	} else {
		currentHashFunc = gohashtree.Hash
	}
}

// GetUseHashtree returns the current hashtree usage setting
func GetUseHashtree() bool {
	return useHashtree
}

func hashParallel(inputList [][32]byte, outputList [][32]byte, wg *sync.WaitGroup) {
	defer wg.Done()
	err := currentHashFunc(outputList, inputList)
	if err != nil {
		panic(err) // lint:nopanic -- This should never panic.
	}
}

// VectorizedSha256 takes a list of roots and hashes them using CPU
// specific vector instructions. Uses either gohashtree or hashtree
// implementation based on the current configuration.
func VectorizedSha256(inputList [][32]byte) [][32]byte {
	outputList := make([][32]byte, len(inputList)/2)
	if len(inputList) < minSliceSizeToParallelize {
		err := currentHashFunc(outputList, inputList)
		if err != nil {
			panic(err) // lint:nopanic -- This should never panic.
		}
		return outputList
	}
	n := runtime.GOMAXPROCS(0) - 1
	wg := sync.WaitGroup{}
	wg.Add(n)
	groupSize := len(inputList) / (2 * (n + 1))
	for j := 0; j < n; j++ {
		go hashParallel(inputList[j*2*groupSize:(j+1)*2*groupSize], outputList[j*groupSize:], &wg)
	}
	err := currentHashFunc(outputList[n*groupSize:], inputList[n*2*groupSize:])
	if err != nil {
		panic(err) // lint:nopanic -- This should never panic.
	}
	wg.Wait()
	return outputList
}
