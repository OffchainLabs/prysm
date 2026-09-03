package driver

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestJsonList(t *testing.T) {
	path := "testdata/json-list.json"
	files, err := loadJsonListing(environment{inventoryIndexPath: path})
	require.NoError(t, err)
	require.Equal(t, 4, len(files))
}
