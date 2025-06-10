package fuzz

import "runtime/debug"

// FreeMemory calls debug.FreeOSMemory() every 10 loop iterations.
// This is very useful in tests that fuzz a proto beacon state
// and initialize a native state from that proto state inside the loop.
// The reason is that fields of the native beacon state which are multi-value
// slices always create a slice of proper length for that field, even if
// the fuzzer creates a smaller slice. Because the beacon state keeps
// a reference to the multi-value slice, the multi-value slice is not garbage
// collected fast enough during fuzzing, leading to memory bloat.
// Freeing memory manually every 10 iterations keeps the in-use memory low.
// The tradeoff is longer test times.
func FreeMemory(iteration int) {
	if iteration%10 == 0 {
		debug.FreeOSMemory()
	}
}
