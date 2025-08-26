package query

import (
	"errors"
	"strings"
)

// PathElement represents a single element in a path.
type PathElement struct {
	Name string
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
		path = append(path, PathElement{Name: elem})
	}

	return path, nil
}
