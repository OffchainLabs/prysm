package benchmark

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	"github.com/MariusVanDerWijden/FuzzyVM/generator"
)

// RunFullBench runs a full benchmark with N runs.
func RunFullBench(N int) {
	time, err := testGeneration(N)
	// Basic building blocks
	printResult("BenchmarkTestGeneration", time, err)
}

func printResult(name string, time time.Duration, err error) {
	if err != nil {
		fmt.Printf("Benchmark %v produced error: %v\n", name, err)
		return
	}
	fmt.Printf("Benchmark %v took %v \n", name, time.String())
}

func newFiller() (*filler.Filler, error) {
	rand.Seed(12345)
	rnd := make([]byte, 40)
	if _, err := rand.Read(rnd); err != nil {
		return nil, err
	}
	return filler.NewFiller(rnd), nil
}

// testGeneration generates N programs.
func testGeneration(N int) (time.Duration, error) {
	f, err := newFiller()
	if err != nil {
		return time.Nanosecond, err
	}
	start := time.Now()
	for i := 0; i < N; i++ {
		generator.GenerateProgram(f)
	}
	return time.Since(start), nil
}
