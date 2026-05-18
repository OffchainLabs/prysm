package async

import (
	"errors"
	"runtime"
	"sync"
)

// WorkerResults are the results of a scatter worker.
type WorkerResults struct {
	Offset int
	Extent any
}

// Scatter scatters a computation across multiple goroutines.
// This breaks the task in to a number of chunks and executes those chunks in parallel with the function provided.
// Results returned are collected and presented as a set of WorkerResults, which can be reassembled by the calling function.
// Any error that occurs in the workers will be passed back to the calling function.
func Scatter(inputLen int, sFunc func(int, int, *sync.RWMutex) (any, error)) ([]*WorkerResults, error) {
	if inputLen <= 0 {
		return nil, errors.New("input length must be greater than 0")
	}

	chunkSize := calculateChunkSize(inputLen)
	workers := inputLen / chunkSize
	if inputLen%chunkSize != 0 {
		workers++
	}

	var (
		resultCh = make(chan *WorkerResults, workers)
		errorCh  = make(chan error, workers)
		mutex    = new(sync.RWMutex)
		wg       sync.WaitGroup
	)
	for worker := 0; worker < workers; worker++ {
		offset := worker * chunkSize
		entries := chunkSize
		if offset+entries > inputLen {
			entries = inputLen - offset
		}
		wg.Add(1)
		go func(offset int, entries int) {
			defer wg.Done()
			extent, err := sFunc(offset, entries, mutex)
			if err != nil {
				errorCh <- err
			} else {
				resultCh <- &WorkerResults{
					Offset: offset,
					Extent: extent,
				}
			}
		}(offset, entries)
	}

	// Wait for all goroutines to finish
	go func() {
		wg.Wait()
		close(resultCh)
		close(errorCh)
	}()

	// Collect results from workers
	results := make([]*WorkerResults, workers)
	i := 0
	for resultCh != nil || errorCh != nil {
		select {
		case result, ok := <-resultCh:
			if !ok {
				resultCh = nil
				continue
			}
			results[i] = result
			i++
		case err, ok := <-errorCh:
			if !ok {
				errorCh = nil
				continue
			}
			return nil, err
		}
	}
	return results, nil
}

// calculateChunkSize calculates a suitable chunk size for the purposes of parallelization.
func calculateChunkSize(items int) int {
	// Start with a simple even split
	chunkSize := items / runtime.GOMAXPROCS(0)

	// Add 1 if we have leftovers (or if we have fewer items than processors).
	if chunkSize == 0 || items%chunkSize != 0 {
		chunkSize++
	}

	return chunkSize
}
