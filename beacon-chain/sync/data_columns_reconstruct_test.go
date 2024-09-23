package sync

import (
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestDataColumnsCache(t *testing.T) {
	var (
		root1 [fieldparams.RootLength]byte
		root2 [fieldparams.RootLength]byte
	)

	root1[0] = 1
	root2[0] = 2

	columnsCache := cache.New(1*time.Minute, 2*time.Minute)

	// Retrieve a non-existent entry
	res, err := dataColumnsCache(columnsCache, root1)
	require.NoError(t, err)
	require.Equal(t, 0, len(res))

	res, err = dataColumnsCache(columnsCache, root2)
	require.NoError(t, err)
	require.Equal(t, 0, len(res))

	// Set an entry in an empty cache for this root
	err = setDataColumnCache(columnsCache, root1, 1)
	require.NoError(t, err)

	err = setDataColumnCache(columnsCache, root2, 2)
	require.NoError(t, err)

	// Retrieve the entry
	res, err = dataColumnsCache(columnsCache, root1)
	require.NoError(t, err)
	require.Equal(t, 1, len(res))
	require.Equal(t, true, res[1])

	res, err = dataColumnsCache(columnsCache, root2)
	require.NoError(t, err)
	require.Equal(t, 1, len(res))
	require.Equal(t, true, res[2])

	// Set a new entry in the cache
	err = setDataColumnCache(columnsCache, root1, 11)
	require.NoError(t, err)

	err = setDataColumnCache(columnsCache, root2, 22)
	require.NoError(t, err)

	// Retrieve the entries
	res, err = dataColumnsCache(columnsCache, root1)
	require.NoError(t, err)
	require.Equal(t, 2, len(res))
	require.Equal(t, true, res[1])
	require.Equal(t, true, res[11])

	res, err = dataColumnsCache(columnsCache, root2)
	require.NoError(t, err)
	require.Equal(t, 2, len(res))
	require.Equal(t, true, res[2])
	require.Equal(t, true, res[22])
}

func TestColumnsArrayToMap(t *testing.T) {
	var input [fieldparams.NumberOfColumns]bool
	input[0] = true
	input[7] = true
	input[14] = true
	input[125] = true

	expected := map[uint64]bool{0: true, 7: true, 14: true, 125: true}

	actual := columnsArrayToMap(input)

	require.Equal(t, len(expected), len(actual))

	for k, v := range expected {
		require.Equal(t, v, actual[k])
	}
}
