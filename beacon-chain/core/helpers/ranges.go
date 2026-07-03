package helpers

import "github.com/OffchainLabs/prysm/v7/container/slice"

// SortedSliceFromMap takes a map with uint64 keys and returns a sorted slice of the keys.
//
// Deprecated: Use container/slice.SortedSliceFromMap instead.
func SortedSliceFromMap(toSort map[uint64]bool) []uint64 {
	return slice.SortedSliceFromMap(toSort)
}

// PrettySlice returns a pretty string representation of a sorted slice of uint64.
// `sortedSlice` must be sorted in ascending order.
// Example: [1,2,3,5,6,7,8,10] -> "1-3,5-8,10"
//
// Deprecated: Use container/slice.PrettySlice instead.
func PrettySlice(sortedSlice []uint64) string {
	return slice.PrettySlice(sortedSlice)
}

// SortedPrettySliceFromMap combines SortedSliceFromMap and PrettySlice to return a pretty string representation of the keys in a map.
//
// Deprecated: Use container/slice.SortedPrettySliceFromMap instead.
func SortedPrettySliceFromMap(toSort map[uint64]bool) string {
	return slice.SortedPrettySliceFromMap(toSort)
}
