package query_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	ssz_query "github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestAnalyzeSSZInfo(t *testing.T) {
	info, err := query.AnalyzeObject(&ssz_query.FixedTestContainer{})
	require.NoError(t, err)

	require.NotNil(t, info, "Expected non-nil SSZ info")
	require.Equal(t, uint64(365), info.FixedSize(), "Expected fixed size to be 365")
}
