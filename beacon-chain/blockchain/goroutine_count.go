package blockchain

import (
	"runtime"
	"sync"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

const goroutineSampleWindow = 10

type goroutineCounter struct {
	mu      sync.RWMutex
	samples [goroutineSampleWindow]int
	avg     int
}

func (g *goroutineCounter) sample(slot primitives.Slot) {
	count := runtime.NumGoroutine()
	goroutineCountGauge.WithLabelValues("instant").Set(float64(count))

	g.mu.Lock()
	defer g.mu.Unlock()
	g.samples[uint64(slot)%goroutineSampleWindow] = count

	total := 0
	for _, sample := range g.samples {
		total += sample
	}
	g.avg = total / goroutineSampleWindow

	goroutineCountGauge.WithLabelValues("average").Set(float64(g.avg))
}

func (g *goroutineCounter) average() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.avg
}
