package query_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
)

func TestCalculateOffsetAndLength(t *testing.T) {
	path, err := query.ParsePath(".data.target.root")
	assert.NoError(t, err)

	info, err := query.AnalyzeSSZInfo(&ethpb.IndexedAttestationElectra{})
	assert.NoError(t, err)

	_, offset, length, err := query.CalculateOffsetAndLength(info, path)
	assert.NoError(t, err)

	assert.Equal(t, uint64(100), offset, "Expected offset to be 100")
	assert.Equal(t, uint64(32), length, "Expected length to be 32")
}
