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
)

// SSZDimension holds parsed SSZ tag information for current dimension.
//
// NOTE: This struct stores raw string without parsing to uint64,
// as it might have a wildcard ('?') indicating variable size.
type SSZDimension struct {
	SizeValue string // Current dimension for ssz-size
	MaxValue  string // Current dimension for ssz-max
	HasSize   bool
	HasMax    bool
}

// ParseSSZTag parses SSZ-specific tags (like `ssz-max` and `ssz-size`)
// and returns the first dimension and remaining tag.
func ParseSSZTag(tag *reflect.StructTag) (*SSZDimension, *reflect.StructTag, error) {
	if tag == nil {
		return nil, nil, nil
	}

	info := &SSZDimension{}
	var newTagParts []string

	// Parse ssz-size tag
	if sszSize := tag.Get(sszSizeTag); sszSize != "" {
		dims := strings.Split(sszSize, ",")
		if len(dims) > 0 && dims[0] != "" {
			info.SizeValue = dims[0]
			info.HasSize = true

			if len(dims) > 1 {
				remainingSize := strings.Join(dims[1:], ",")
				newTagParts = append(newTagParts, fmt.Sprintf(`%s:"%s"`, sszSizeTag, remainingSize))
			}
		}
	}

	// Parse ssz-max tag
	if sszMax := tag.Get(sszMaxTag); sszMax != "" {
		dims := strings.Split(sszMax, ",")
		if len(dims) > 0 && dims[0] != "" {
			info.MaxValue = dims[0]
			info.HasMax = true

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

	return info, newTag, nil
}

// ParseUint64 parses a string dimension value to uint64.
func (info *SSZDimension) ParseUint64(value string) (uint64, error) {
	if value == "" {
		return 0, errors.New("empty dimension value")
	}
	return strconv.ParseUint(value, 10, 64)
}

// IsVector returns true if this dimension represents a vector.
func (info *SSZDimension) IsVector() bool {
	return info.HasSize && info.SizeValue != "" && info.SizeValue != "?"
}

// IsList returns true if this dimension represents a list.
// `ssz-size` can be a wildcard ('?') indicating variable size.
func (info *SSZDimension) IsList() bool {
	return info.HasMax && (!info.HasSize || info.SizeValue == "" || info.SizeValue == "?")
}

// GetVectorLength returns the length for a vector in current dimension
func (info *SSZDimension) GetVectorLength() (uint64, error) {
	if !info.IsVector() {
		return 0, errors.New("not a vector dimension")
	}
	return info.ParseUint64(info.SizeValue)
}

// GetListLimit returns the limit for a list in current dimension
func (info *SSZDimension) GetListLimit() (uint64, error) {
	if !info.IsList() {
		return 0, errors.New("not a list dimension")
	}
	return info.ParseUint64(info.MaxValue)
}
