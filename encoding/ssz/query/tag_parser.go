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
		return nil, nil, errors.New("nil struct tag")
	}

	var newTagParts []string
	var sizeStr, maxStr string
	var isBitfield bool

	if castType := tag.Get(castTypeTag); strings.Contains(castType, "go-bitfield") {
		isBitfield = true
	}

	// Parse ssz-size tag
	if sszSize := tag.Get(sszSizeTag); sszSize != "" {
		dims := strings.Split(sszSize, ",")
		if len(dims) > 0 {
			sizeStr = dims[0]

			if len(dims) > 1 {
				remainingSize := strings.Join(dims[1:], ",")
				newTagParts = append(newTagParts, fmt.Sprintf(`%s:"%s"`, sszSizeTag, remainingSize))
			}
		}
	}

	// Parse ssz-max tag
	if sszMax := tag.Get(sszMaxTag); sszMax != "" {
		dims := strings.Split(sszMax, ",")
		if len(dims) > 0 {
			maxStr = dims[0]

			if len(dims) > 1 {
				remainingMax := strings.Join(dims[1:], ",")
				newTagParts = append(newTagParts, fmt.Sprintf(`%s:"%s"`, sszMaxTag, remainingMax))
			}
		}
	}

	// Create new tag with remaining dimensions only.
	// We don't have to preserve other tags like json, protobuf.
	var newTag *reflect.StructTag
	if len(newTagParts) > 0 {
		newTagStr := strings.Join(newTagParts, " ")
		t := reflect.StructTag(newTagStr)
		newTag = &t
	}

	// Parse the first dimension based on ssz-size and ssz-max rules.
	// 1. If ssz-size is not specified (wildcard or empty), it must be a list.
	if sizeStr == "?" || sizeStr == "" {
		if maxStr == "?" {
			return nil, nil, errors.New("ssz-size and ssz-max cannot both be '?'")
		}
		if maxStr == "" {
			return nil, nil, errors.New("list requires ssz-max value")
		}

		limit, err := strconv.ParseUint(maxStr, 10, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid ssz-max value: %w", err)
		}
		if limit == 0 {
			return nil, nil, errors.New("ssz-max must be greater than 0")
		}

		return &SSZDimension{listLimit: &limit, isBitfield: isBitfield}, newTag, nil
	}

	// 2. If ssz-size is specified, it must be a vector.
	length, err := strconv.ParseUint(sizeStr, 10, 64)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid ssz-size value: %w", err)
	}
	if length == 0 {
		return nil, nil, errors.New("ssz-size must be greater than 0")
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
		return 0, errors.New("not a vector dimension")
	}
	return *d.vectorLength, nil
}

// GetListLimit returns the limit for a list in current dimension
func (d *SSZDimension) GetListLimit() (uint64, error) {
	if !d.IsList() {
		return 0, errors.New("not a list dimension")
	}
	return *d.listLimit, nil
}
