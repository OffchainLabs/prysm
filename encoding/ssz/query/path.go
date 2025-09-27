package query

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// PathElement represents a single element in a path.
type PathElement struct {
	Name string
	// [Optional] Index for List/Vector elements
	Index *uint64
}

func ParsePath(rawPath string) ([]PathElement, error) {
	// We use dot notation, so we split the path by '.'.
	rawElements := strings.Split(rawPath, ".")
	if len(rawElements) == 0 {
		return nil, errors.New("empty path provided")
	}

	if rawElements[0] == "" {
		// Remove leading dot if present
		rawElements = rawElements[1:]
	}

	var path []PathElement
	for _, elem := range rawElements {
		if elem == "" {
			return nil, errors.New("invalid path: consecutive dots or trailing dot")
		}

		fieldName := elem
		var index *uint64

		// Check for index notation, e.g., "field[0]"
		if strings.Contains(elem, "[") {
			parts := strings.SplitN(elem, "[", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid index notation in path element %s", elem)
			}

			fieldName = parts[0]
			indexPart := strings.TrimSuffix(parts[1], "]")
			if indexPart == "" {
				return nil, errors.New("index cannot be empty")
			}

			indexValue, err := strconv.ParseUint(indexPart, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid index in path element %s: %w", elem, err)
			}
			index = &indexValue
		}

		path = append(path, PathElement{Name: fieldName, Index: index})
	}

	return path, nil
}
