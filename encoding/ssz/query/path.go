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

// ParsePath parses a raw path string into a slice of PathElements.
// note: field names are stored in snake_case format. rawPath is enforced to be snake_case.
// 1. Supports dot notation for field access (e.g., "field1.field2").
// 2. Supports array indexing using square brackets (e.g., "array_field[0]").
// 3. Supports length access using len() notation (e.g., "len(array_field)").
// 4. Handles leading dots and validates path format.
func ParsePath(rawPath string) ([]PathElement, error) {
	// Enforce snake_case for field names
	rawElements := strings.Split(rawPath, ".")

	if rawElements[0] == "" {
		// Remove leading dot if present
		rawElements = rawElements[1:]
	}

	var path []PathElement
	for _, elem := range rawElements {
		if elem == "" {
			return nil, errors.New("invalid path: consecutive dots or trailing dot")
		}
		snakeCasePath := toSnakeCase(elem)

		var pathElement PathElement
		// Processing element string
		processingField := snakeCasePath

		re := regexp.MustCompile(`^\s*len\s*\(\s*([^)]+?)\s*\)\s*$`)
		matches := re.FindStringSubmatch(processingField)
		if len(matches) == 2 {
			pathElement.Length = true
			// Extract the inner expression between len( and ) and continue parsing on that
			processingField = matches[1]
		}

		// Default name is the full working string (may be updated below if it contains indices)
		pathElement.Name = processingField

		if strings.Contains(processingField, "[") {
			// Split into field and indices, e.g., "array[0][1]" -> name:"array", indices:{0,1}
			pathElement.Name = extractFieldName(processingField)
			indices, err := extractArrayIndices(processingField)
			if err != nil {
				return []PathElement{}, err
			}
			// Although extractArrayIndices supports multiple indices,
			// only a single index is supported per PathElement, e.g., "transactions[0]" is valid
			// while "transactions[0][0]" is rejected explicitly.
			if len(indices) != 1 {
				return []PathElement{}, fmt.Errorf("multiple indices not supported in token %q", processingField)
			}
			pathElement.Index = &indices[0]

		}

		path = append(path, pathElement)
	}

	return path, nil
}

// extractFieldName extracts the field name from a path element name (removes array indices)
// For example: "field_name[5]" returns "field_name"
func extractFieldName(name string) string {
	if idx := strings.Index(name, "["); idx != -1 {
		return name[:idx]
	}
	return name
}

var arrayIndexRegex = regexp.MustCompile(`\[\s*([^\]]+)\s*\]`)

// extractArrayIndices returns every bracketed, non-negative index in the name,
// e.g. "array[0][1]" -> []uint64{0, 1}. Errors if none are found or if any index is invalid.
func extractArrayIndices(name string) ([]uint64, error) {
	// Match all bracketed content, then we'll parse as unsigned to catch negatives explicitly
	re := arrayIndexRegex
	matches := re.FindAllStringSubmatch(name, -1)

	if len(matches) == 0 {
		return nil, errors.New("no array indices found")
	}

	indices := make([]uint64, 0, len(matches))
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		// Forbid signs explicitly; we want a clear error similar to ParseUint's message
		if strings.HasPrefix(raw, "-") {
			return nil, fmt.Errorf("cannot process negative indices %q", raw)
		}
		idx, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid array index: %w", err)
		}
		indices = append(indices, idx)
	}
	return indices, nil
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
