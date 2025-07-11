package fuzz

import "runtime/debug"

// FreeMemory calls debug.FreeOSMemory() every 10 loop iterations.
// This is very useful in tests that initialize a native state from a proto state inside a loop.
// Most commonly this happens in tests that make use of fuzzing.
// The reason is that fields of the native beacon state which are multi-value
// slices always create a slice of proper length for that field, even if
// the proto state's slice has a smaller length. Because the beacon state keeps
// a reference to the multi-value slice object, the multi-value slice is not garbage
// collected fast enough, leading to memory bloat.
// Freeing memory manually every 10 iterations keeps the in-use memory low.
// The tradeoff is longer test times.
func FreeMemory(iteration int) {
	if iteration%10 == 0 {
		debug.FreeOSMemory()
	}
}
