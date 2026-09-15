package hdiff

import (
	"fmt"

	"github.com/golang/snappy"
)

// snappyDecode rejects impossible decoded lengths before allocation.
func snappyDecode(input []byte) ([]byte, error) {
	n, err := snappy.DecodedLen(input)
	if err != nil {
		return nil, fmt.Errorf("get decoded length: %w", err)
	}

	// NOTE: 32 is a safety margin to prevent excessive memory allocation.
	// In theory, a valid snappy block can expand at most 64/3x its compressed size.
	// See 2.2.2. in:
	// https://github.com/google/snappy/blob/main/format_description.txt#L88-L111
	if n > 32*len(input) {
		return nil, snappy.ErrCorrupt
	}

	return snappy.Decode(nil, input)
}
