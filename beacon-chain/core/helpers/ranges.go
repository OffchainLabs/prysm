package helpers

import (
	"fmt"
	"slices"
)

// SortedSliceFromMap takes a map with uint64 keys and returns a sorted slice of the keys.
func SortedSliceFromMap(toSort map[uint64]bool) []uint64 {
	slice := make([]uint64, 0, len(toSort))
	for key := range toSort {
		slice = append(slice, key)
	}

	slices.Sort(slice)
	return slice
}

// PrettySlice returns a pretty string representation of a sorted slice of uint64.
// `sortedSlice` must be sorted in ascending order.
// Example: [1,2,3,5,6,7,8,10] -> "1-3,5-8,10"
func PrettySlice(sortedSlice []uint64) string {
	if len(sortedSlice) == 0 {
		return ""
	}

	var result string
	start := sortedSlice[0]
	end := sortedSlice[0]

	for i := 1; i < len(sortedSlice); i++ {
		if sortedSlice[i] == end+1 {
			end = sortedSlice[i]
			continue
		}

		if start == end {
			result += fmt.Sprintf("%d,", start)
			start = sortedSlice[i]
			end = sortedSlice[i]
			continue
		}

		result += fmt.Sprintf("%d-%d,", start, end)
		start = sortedSlice[i]
		end = sortedSlice[i]
	}

	if start == end {
		result += fmt.Sprintf("%d", start)
		return result
	}

	result += fmt.Sprintf("%d-%d", start, end)
	return result
}

// SortedPrettySliceFromMap combines SortedSliceFromMap and PrettySlice to return a pretty string representation of the keys in a map.
func SortedPrettySliceFromMap(toSort map[uint64]bool) string {
	sorted := SortedSliceFromMap(toSort)
	return PrettySlice(sorted)
}
