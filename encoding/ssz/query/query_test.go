package query_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/query/testutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
)

func TestCalculateOffsetAndLength(t *testing.T) {
	path, err := query.ParsePath(".data.target.root")
	assert.NoError(t, err)

	info, err := query.AnalyzeObject(&ethpb.IndexedAttestationElectra{})
	assert.NoError(t, err)

	_, offset, length, err := query.CalculateOffsetAndLength(info, path)
	assert.NoError(t, err)

	assert.Equal(t, uint64(100), offset, "Expected offset to be 100")
	assert.Equal(t, uint64(32), length, "Expected length to be 32")
}

func TestRoundTripSszInfo(t *testing.T) {
	specs := []testutil.TestSpec{
		getIndexedAttestationElectraSpec(t),
	}

	for _, spec := range specs {
		testutil.RunStructTest(t, spec)
	}
}

func createIndexedAttestationElectra(t *testing.T) any {
	randomData := testutil.RandomDummyData(t)

	return &ethpb.IndexedAttestationElectra{
		AttestingIndices: []uint64{1, 2, 3},
		Data: &ethpb.AttestationData{
			Slot:            4,
			CommitteeIndex:  5,
			BeaconBlockRoot: randomData.Root,
			Source: &ethpb.Checkpoint{
				Epoch: 7,
				Root:  randomData.Root,
			},
			Target: &ethpb.Checkpoint{
				Epoch: 9,
				Root:  randomData.Root,
			},
		},
		Signature: randomData.Signature,
	}
}

func getIndexedAttestationElectraSpec(t *testing.T) testutil.TestSpec {
	indexedAtt := createIndexedAttestationElectra(t).(*ethpb.IndexedAttestationElectra)

	return testutil.TestSpec{
		Name:     "IndexedAttestationElectra",
		Type:     ethpb.IndexedAttestationElectra{},
		Instance: indexedAtt,
		PathTests: []testutil.PathTest{
			{
				Path:     ".data.target.root",
				Expected: indexedAtt.Data.Target.Root,
			},
			{
				Path:     ".data.target",
				Expected: indexedAtt.Data.Target,
			},
			{
				Path:     ".data",
				Expected: indexedAtt.Data,
			},
			{
				Path:     ".signature",
				Expected: indexedAtt.Signature,
			},
		},
	}
}
