package apiutil

import (
	"fmt"
	neturl "net/url"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

// Uint64ToString is a util function that will convert uints to string
func Uint64ToString[T uint64 | primitives.Slot | primitives.ValidatorIndex | primitives.CommitteeIndex | primitives.Epoch](val T) string {
	return strconv.FormatUint(uint64(val), 10)
}

// BuildURL is a util function that assists with adding query parameters to the url
func BuildURL(path string, queryParams ...neturl.Values) string {
	if len(queryParams) == 0 {
		return path
	}

	return fmt.Sprintf("%s?%s", path, queryParams[0].Encode())
}
