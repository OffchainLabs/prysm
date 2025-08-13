package query_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
)

func TestParsePath(t *testing.T) {
	path := ".data.target.root"

	parsedPath, err := query.ParsePath(path)
	assert.NoError(t, err)

	expectedPath := []query.PathElement{
		{Name: "data"},
		{Name: "target"},
		{Name: "root"},
	}

	assert.Equal(t, 3, len(parsedPath), "Expected 3 path elements, got %d", len(parsedPath))
	assert.DeepEqual(t, expectedPath, parsedPath, "Parsed path does not match expected path")
}
