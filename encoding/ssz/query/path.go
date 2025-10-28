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
	Name string
	// [Optional] Index for List/Vector elements
	Index *uint64
}

// Path represents the entire path structure for SSZ-QL queries. It consists of multiple PathElements
// and a flag indicating if the path is querying for length.
type Path struct {
	// If true, the path is querying for the length of the final element in Elements field
	Length bool
	// Sequence of path elements representing the navigation through the SSZ structure
	Elements []PathElement
}

// Matches an array index expression like [123] or [ foo ] and captures the inner content without the brackets.
var arrayIndexRegex = regexp.MustCompile(`\[\s*([^\]]+)\s*\]`)

// Matches an entire string that’s a len(<expr>) call (whitespace flexible), capturing the inner expression and disallowing any trailing characters.
var lengthRegex = regexp.MustCompile(`^\s*len\s*\(\s*([^)]+?)\s*\)\s*$`)

// ParsePath parses a raw path string into a slice of PathElements.
// note: field names are stored in snake case format. rawPath has to be provided in snake case.
// 1. Supports dot notation for field access (e.g., "field1.field2").
// 2. Supports array indexing using square brackets (e.g., "array_field[0]").
// 3. Supports length access using len() notation (e.g., "len(array_field)").
// 4. Handles leading dots and validates path format.
func ParsePath(rawPath string) (Path, error) {
	var rawElements []string
	var processedPath Path

	matches := lengthRegex.FindStringSubmatch(rawPath)

	// FindStringSubmatch matches a whole string like "len(field_name)" and its inner expression.
	// For a path element to be a length query, len(matches) should be 2:
	// 1. Full match: "len(field_name)"
	// 2. Inner expression: "field_name"
	if len(matches) == 2 {
		processedPath.Length = true
		// If we have found a len() expression, we only want to parse its inner expression.
		rawElements = strings.Split(matches[1], ".")
	} else {
		// Normal path parsing
		rawElements = strings.Split(rawPath, ".")
	}

	if rawElements[0] == "" {
		// Remove leading dot if present
		rawElements = rawElements[1:]
	}

	var pathElements []PathElement
	for _, elem := range rawElements {
		if elem == "" {
			return Path{}, errors.New("invalid pathElements: consecutive dots or trailing dot")
		}

		// Processing element string
		processingField := elem
		var pathElement PathElement

		// Default name is the full working string (may be updated below if it contains indices)
		pathElement.Name = processingField

		if strings.Contains(processingField, "[") {
			// Split into field and indices, e.g., "array[0][1]" -> name:"array", indices:{0,1}
			pathElement.Name = extractFieldName(processingField)
			indices, err := extractArrayIndices(processingField)
			if err != nil {
				return Path{}, err
			}
			// Although extractArrayIndices supports multiple indices,
			// only a single index is supported per PathElement, e.g., "transactions[0]" is valid
			// while "transactions[0][0]" is rejected explicitly.
			if len(indices) != 1 {
				return Path{}, fmt.Errorf("multiple indices not supported in token %s", processingField)
			}
			pathElement.Index = &indices[0]

		}

		pathElements = append(pathElements, pathElement)
	}

	processedPath.Elements = pathElements
	return processedPath, nil
}

// extractFieldName extracts the field name from a path element name (removes array indices)
// For example: "field_name[5]" returns "field_name"
func extractFieldName(name string) string {
	if idx := strings.Index(name, "["); idx != -1 {
		return name[:idx]
	}
	return name
}

// extractArrayIndices returns every bracketed, non-negative index in the name,
// e.g. "array[0][1]" -> []uint64{0, 1}. Errors if none are found or if any index is invalid.
func extractArrayIndices(name string) ([]uint64, error) {
	// Match all bracketed content, then we'll parse as unsigned to catch negatives explicitly
	matches := arrayIndexRegex.FindAllStringSubmatch(name, -1)

	if len(matches) == 0 {
		return nil, errors.New("no array indices found")
	}

	indices := make([]uint64, 0, len(matches))
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		idx, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid array index: %w", err)
		}
		indices = append(indices, idx)
	}
	return indices, nil
}
