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

// Path represents the entire path structure for SSZ-QL queries. It consists of multiple PathElements
// and a flag indicating if the path is querying for length.
type Path struct {
	// If true, the path is querying for the length of the final element in Elements field
	Length bool
	// Sequence of path elements representing the navigation through the SSZ structure
	Elements []PathElement
}

// ParsePath parses a raw path string into a slice of PathElements.
// note: field names are stored in snake case format. rawPath has to be provided in snake case.
// 1. Supports dot notation for field access (e.g., "field1.field2").
// 2. Supports array indexing using square brackets (e.g., "array_field[0]").
// 3. Supports length access using len() notation (e.g., "len(array_field)").
// 4. Handles leading dots and validates path format.
func ParsePath(rawPath string) (Path, error) {
	if rawPath == "" {
		return Path{}, nil
	}

	// 1. Identify and remove len() wrapper
	inner, isLen, err := checkLength(rawPath)
	if err != nil {
		return Path{}, err
	}

	// 2. Clean up starting separators
	inner = strings.TrimPrefix(inner, ".")
	if inner == "" {
		return Path{}, errors.New("invalid path: consecutive dots or trailing dot")
	}

	// 3. Break the string into structured elements
	elements, err := parseSegments(inner)
	if err != nil {
		return Path{}, err
	}

	return Path{
		Length:   isLen,
		Elements: elements,
	}, nil
}

// checkLength determines if the path is wrapped in len()
func checkLength(path string) (string, bool, error) {
	if strings.HasPrefix(path, "len(") {
		if !strings.HasSuffix(path, ")") {
			return "", false, fmt.Errorf("unmatched parentheses in path: %s", path)
		}
		inner := path[4 : len(path)-1]
		if inner == "" {
			return "", false, errors.New("len() call must not be empty")
		}
		return inner, true, nil
	}

	if strings.Contains(path, "len(") {
		return "", false, fmt.Errorf("len() call must be at the start of the path: %s", path)
	}

	return path, false, nil
}

// parseSegments loops through the string to extract fields and indices
func parseSegments(path string) ([]PathElement, error) {
	elements := make([]PathElement, 0, 4)
	n := len(path)

	for i := 0; i < n; {
		// Get field name (e.g., "data")
		name, next, err := readName(path, i)
		if err != nil {
			return nil, err
		}

		element := PathElement{Name: name}
		i = next

		// Get index if '[' follows (e.g., "[0]")
		if i < n && path[i] == '[' {
			index, next, err := readIndex(path, i)
			if err != nil {
				return nil, err
			}
			element.Index = &index
			i = next

			// Ensure valid transition after a bracket
			if i < n && path[i] != '.' {
				return nil, fmt.Errorf("invalid path format near brackets")
			}
		}

		elements = append(elements, element)

		// Move past the dot separator
		if i < n && path[i] == '.' {
			i++
			if i == n { // Path ends in a dot
				return nil, errors.New("invalid path: consecutive dots or trailing dot")
			}
		}
	}
	return elements, nil
}

// readName extracts the alphanumeric field name
func readName(path string, start int) (string, int, error) {
	curr := start
	for curr < len(path) && path[curr] != '.' && path[curr] != '[' {
		c := path[curr]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return "", 0, fmt.Errorf("invalid character in path")
		}
		curr++
	}
	if curr == start {
		return "", 0, errors.New("invalid path: consecutive dots or trailing dot")
	}
	return path[start:curr], curr, nil
}

// readIndex extracts the number between brackets
func readIndex(path string, start int) (uint64, int, error) {
	i := start + 1 // skip '['
	mark := i
	for i < len(path) && path[i] >= '0' && path[i] <= '9' {
		i++
	}

	if i == mark || i == len(path) || path[i] != ']' {
		return 0, 0, fmt.Errorf("invalid bracket format in path")
	}

	val, err := strconv.ParseUint(path[mark:i], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return val, i + 1, nil // return value and position after ']'
}
