package query

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PathElement represents a single element in a path.
type PathElement struct {
	Length bool
	Name   string
	// [Optional] Index for List/Vector elements
	Index *uint64
}

var (
	arrayIndexRegex = regexp.MustCompile(`\[\s*([^\]]+)\s*\]`)
	lengthRegex     = regexp.MustCompile(`^\s*len\s*\(\s*([^)]+?)\s*\)\s*$`)
)

// ParsePath parses a raw path string into a slice of PathElements.
// note: field names are stored in snake case format. rawPath has to be provided in snake case.
// 1. Supports dot notation for field access (e.g., "field1.field2").
// 2. Supports array indexing using square brackets (e.g., "array_field[0]").
// 3. Supports length access using len() notation (e.g., "len(array_field)").
// 4. Handles leading dots and validates path format.
func ParsePath(rawPath string) ([]PathElement, error) {
	// empty path returns empty slice (not error)
	if len(rawPath) == 0 {
		return nil, nil
	}

	// Skip leading dot without reallocation
	if rawPath[0] == '.' {
		// Remove leading dot if present
		rawPath = rawPath[1:]
		if len(rawPath) == 0 {
			return nil, errors.New("invalid path: trailing dot")
		}
	}

	// allocate slice for path elements based on dot count
	count := strings.Count(rawPath, ".") + 1
	path := make([]PathElement, 0, count)

	start := 0
	for {
		// Extract the next token (split by dot manually for fewer allocations)
		dot := strings.IndexByte(rawPath[start:], '.')
		var elem string
		if dot == -1 {
			elem = rawPath[start:]
		} else {
			elem = rawPath[start : start+dot]
		}
		// Reject empty tokens caused by consecutive or trailing dots
		if elem == "" {
			return nil, errors.New("invalid path: consecutive dots or trailing dot")
		}

		var pe PathElement
		field := elem

		// FindStringSubmatch matches a whole string like "len(field_name)" and its inner expression.
		// For a path element to be a length query, len(matches) should be 2:
		// 1. Full match: "len(field_name)"
		// 2. Inner expression: "field_name"
		if matches := lengthRegex.FindStringSubmatch(field); len(matches) == 2 {
			pe.Length = true
			field = matches[1]
		}

		// Detect array indices such as "array[0]"
		if idx := strings.IndexByte(field, '['); idx != -1 {
			// extractFieldName: get field name before '['
			pe.Name = field[:idx]

			// extractArrayIndices: parse one or more numeric indices inside brackets
			idxs, err := extractArrayIndices(field[idx:])
			if err != nil {
				return nil, err
			}

			// Allow only a single index per element: reject "array[0][1]"
			if len(idxs) != 1 {
				return nil, fmt.Errorf("multiple indices not supported in token %s", field)
			}
			// Store parsed index
			pe.Index = &idxs[0]
		} else {
			pe.Name = field
		}

		path = append(path, pe)

		if dot == -1 {
			break
		}
		start += dot + 1
		if start >= len(rawPath) {
			return nil, errors.New("invalid path: trailing dot at end of path")
		}
	}

	return path, nil
}

// extractArrayIndices returns every bracketed, non-negative index in the name,
// e.g. "array[0][1]" -> []uint64{0, 1}. Errors if none are found or if any index is invalid.
func extractArrayIndices(s string) ([]uint64, error) {
	matches := arrayIndexRegex.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil, errors.New("no array indices found")
	}

	indices := make([]uint64, len(matches))
	for i, m := range matches {
		numStr := strings.TrimSpace(m[1])
		idx, err := strconv.ParseUint(numStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid array index: %w", err)
		}
		indices[i] = idx
	}
	return indices, nil
}
