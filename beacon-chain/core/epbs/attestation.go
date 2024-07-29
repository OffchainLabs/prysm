package epbs

import (
	"fmt"
)

// RemoveValidatorFlag removes validator flag from existing one.
func RemoveValidatorFlag(flag, flagPosition uint8) (uint8, error) {
	if flagPosition > 7 {
		return flag, fmt.Errorf("flag position %d exceeds length", flagPosition)
	}
	return flag & ^(1 << flagPosition), nil
}
