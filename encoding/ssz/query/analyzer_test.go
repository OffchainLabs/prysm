package query_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	sszquerypb "github.com/OffchainLabs/prysm/v6/proto/ssz_query"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestAnalyzeSSZInfo(t *testing.T) {
	info, err := query.AnalyzeObject(&sszquerypb.FixedTestContainer{})
	require.NoError(t, err)

	require.NotNil(t, info, "Expected non-nil SSZ info")
	require.Equal(t, uint64(565), info.FixedSize())
}
