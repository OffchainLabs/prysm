package query

import (
	"fmt"
	"strings"
)

// PathElement represents a single element in a path. It only supports field names currently.
//
// TODO 1: Add feature for accessing by index of a list or vector.
// TODO 2: Add feature for getting the length of a list or vector.
type PathElement struct {
	Name string
}

func ParsePath(rawPath string) ([]PathElement, error) {
	// We use dot notation, so we split the path by '.'.
	rawElements := strings.Split(rawPath, ".")
	if len(rawElements) == 0 {
		return nil, fmt.Errorf("empty path provided")
	}

	if rawElements[0] == "" {
		// Remove leading dot if present
		rawElements = rawElements[1:]
	}

	var path []PathElement
	for _, elem := range rawElements {
		path = append(path, PathElement{Name: elem})
	}

	return path, nil
}
