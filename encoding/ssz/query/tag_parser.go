package query

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	// sszMaxTag specifies the maximum capacity of a variable-sized collection, like an SSZ List.
	sszMaxTag = "ssz-max"

	// sszSizeTag specifies the length of a fixed-sized collection, like an SSZ Vector.
	// A wildcard ('?') indicates that the dimension is variable-sized (a List).
	sszSizeTag = "ssz-size"

	// castTypeTag specifies special custom casting instructions.
	// e.g., "github.com/prysmaticlabs/go-bitfield.Bitlist".
	castTypeTag = "cast-type"

	// Bitfield substring for fast checking
	bitfieldMarker = "go-bitfield"
)

var (
	errNilStructTag       = errors.New("nil struct tag")
	errBothWildcard       = errors.New("ssz-size and ssz-max cannot both be '?'")
	errListRequiresMax    = errors.New("list requires ssz-max value")
	errMaxMustBePositive  = errors.New("ssz-max must be greater than 0")
	errSizeMustBePositive = errors.New("ssz-size must be greater than 0")
	errNotVectorDimension = errors.New("not a vector dimension")
	errNotListDimension   = errors.New("not a list dimension")
)

// SSZDimension holds parsed SSZ tag information for current dimension.
// Mutually exclusive fields indicate whether the dimension is a vector or a list.
type SSZDimension struct {
	vectorLength *uint64
	listLimit    *uint64

	// isBitfield indicates if the dimension represents a bitfield type (Bitlist, Bitvector).
	isBitfield bool
}

// ParseSSZTag parses SSZ-specific tags (like `ssz-max` and `ssz-size`)
// and returns the first dimension and the remaining SSZ tags.
// This function validates the tags and returns an error if they are malformed.
func ParseSSZTag(tag *reflect.StructTag) (*SSZDimension, *reflect.StructTag, error) {
	if tag == nil {
		return nil, nil, errNilStructTag
	}

	isBitfield := strings.Contains(tag.Get(castTypeTag), bitfieldMarker)

	sszSize := tag.Get(sszSizeTag)
	sszMax := tag.Get(sszMaxTag)

	// Early return if both tags are empty
	if sszSize == "" && sszMax == "" {
		return nil, nil, errListRequiresMax
	}

	var sizeStr, maxStr string
	var newTagParts []string

	// Parse ssz-size tag
	if sszSize != "" {
		if idx := strings.IndexByte(sszSize, ','); idx != -1 {
			sizeStr = sszSize[:idx]
			// Only allocate if there are remaining dimensions
			if idx+1 < len(sszSize) {
				newTagParts = append(newTagParts, sszSizeTag+`:"`+sszSize[idx+1:]+`"`)
			}
		} else {
			sizeStr = sszSize
		}
	}

	// Parse ssz-max tag
	if sszMax != "" {
		if idx := strings.IndexByte(sszMax, ','); idx != -1 {
			maxStr = sszMax[:idx]
			// Only allocate if there are remaining dimensions
			if idx+1 < len(sszMax) {
				newTagParts = append(newTagParts, sszMaxTag+`:"`+sszMax[idx+1:]+`"`)
			}
		} else {
			maxStr = sszMax
		}
	}

	// Create new tag with remaining dimensions only.
	// We don't have to preserve other tags like json, protobuf.
	var newTag *reflect.StructTag
	if len(newTagParts) > 0 {
		totalLen := 0
		for i := range newTagParts {
			totalLen += len(newTagParts[i])
			if i > 0 {
				totalLen++ // space separator
			}
		}

		var sb strings.Builder
		sb.Grow(totalLen)
		for i, part := range newTagParts {
			if i > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(part)
		}

		t := reflect.StructTag(sb.String())
		newTag = &t
	}

	// Parse the first dimension based on ssz-size and ssz-max rules.
	// 1. If ssz-size is not specified (wildcard or empty), it must be a list.
	if sizeStr == "?" || sizeStr == "" {
		if maxStr == "?" {
			return nil, nil, errBothWildcard
		}
		if maxStr == "" {
			return nil, nil, errListRequiresMax
		}

		limit, err := strconv.ParseUint(maxStr, 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid ssz-max value: %w", err)
		}
		if limit == 0 {
			return nil, nil, errMaxMustBePositive
		}

		return &SSZDimension{listLimit: &limit, isBitfield: isBitfield}, newTag, nil
	}

	// 2. If ssz-size is specified, it must be a vector.
	length, err := strconv.ParseUint(sizeStr, 10, 64)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid ssz-size value: %w", err)
	}
	if length == 0 {
		return nil, nil, errSizeMustBePositive
	}

	return &SSZDimension{vectorLength: &length, isBitfield: isBitfield}, newTag, nil
}

// IsVector returns true if this dimension represents a vector.
func (d *SSZDimension) IsVector() bool {
	return d.vectorLength != nil
}

// IsList returns true if this dimension represents a list.
func (d *SSZDimension) IsList() bool {
	return d.listLimit != nil
}

// GetVectorLength returns the length for a vector in current dimension
func (d *SSZDimension) GetVectorLength() (uint64, error) {
	if !d.IsVector() {
		return 0, errNotVectorDimension
	}
	return *d.vectorLength, nil
}

// GetListLimit returns the limit for a list in current dimension
func (d *SSZDimension) GetListLimit() (uint64, error) {
	if !d.IsList() {
		return 0, errNotListDimension
	}
	return *d.listLimit, nil
}
