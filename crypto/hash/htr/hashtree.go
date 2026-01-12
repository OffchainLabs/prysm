package htr

import (
	"runtime"
	"sync"

	"github.com/OffchainLabs/hashtree"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/prysmaticlabs/gohashtree"
)

const minSliceSizeToParallelize = 5000

func hashParallel(inputList [][32]byte, outputList [][32]byte, wg *sync.WaitGroup) {
	defer wg.Done()
	var err error
	if features.Get().EnableHashtree {
		err = hashtree.Hash(outputList, inputList)
	} else {
		err = gohashtree.Hash(outputList, inputList)
	}
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
		if features.Get().EnableHashtree {
			err := hashtree.Hash(outputList, inputList)
			if err != nil {
				panic(err) // lint:nopanic -- This should never panic.
			}
		} else {
			err := gohashtree.Hash(outputList, inputList)
			if err != nil {
				panic(err) // lint:nopanic -- This should never panic.
			}
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
	var err error
	if features.Get().EnableHashtree {
		err = hashtree.Hash(outputList[n*groupSize:], inputList[n*2*groupSize:])
	} else {
		err = gohashtree.Hash(outputList[n*groupSize:], inputList[n*2*groupSize:])
	}
	if err != nil {
		panic(err) // lint:nopanic -- This should never panic.
	}
	wg.Wait()
	return outputList
}
