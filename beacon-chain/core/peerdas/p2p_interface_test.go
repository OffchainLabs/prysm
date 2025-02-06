package peerdas_test

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestCustodyGroupCount(t *testing.T) {
	testCases := []struct {
		name                  string
		subscribeToAllSubnets bool
		expected              uint64
	}{
		{
			name:                  "subscribeToAllSubnets=false",
			subscribeToAllSubnets: false,
			expected:              params.BeaconConfig().CustodyRequirement,
		},
		{
			name:                  "subscribeToAllSubnets=true",
			subscribeToAllSubnets: true,
			expected:              params.BeaconConfig().DataColumnSidecarSubnetCount,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set flags.
			resetFlags := flags.Get()
			defer func() {
				flags.Init(resetFlags)
			}()

			params.SetupTestConfigCleanup(t)
			gFlags := new(flags.GlobalFlags)
			gFlags.SubscribeToAllSubnets = tc.subscribeToAllSubnets
			flags.Init(gFlags)

			// Get the custody subnet count.
			actual := peerdas.CustodyGroupCount()
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestVerifyDataColumnSidecarKZGProofs(t *testing.T) {
	dbBlock := util.NewBeaconBlockDeneb()
	require.NoError(t, kzg.Start())

	var (
		comms [][]byte
		blobs []kzg.Blob
	)
	for i := int64(0); i < 6; i++ {
		blob := getRandBlob(i)
		commitment, _, err := generateCommitmentAndProof(&blob)
		require.NoError(t, err)
		comms = append(comms, commitment[:])
		blobs = append(blobs, blob)
	}

	dbBlock.Block.Body.BlobKzgCommitments = comms
	sBlock, err := blocks.NewSignedBeaconBlock(dbBlock)
	require.NoError(t, err)
	sCars, err := peerdas.DataColumnSidecars(sBlock, blobs)
	require.NoError(t, err)

	for i, sidecar := range sCars {
		roCol, err := blocks.NewRODataColumn(sidecar)
		require.NoError(t, err)
		verified, err := peerdas.VerifyDataColumnsSidecarKZGProofs([]blocks.RODataColumn{roCol})
		require.NoError(t, err)
		require.Equal(t, true, verified, fmt.Sprintf("sidecar %d failed", i))
	}
}

func TestCustodyGroupCountFromRecord(t *testing.T) {
	const expected uint64 = 7

	// Create an Ethereum record.
	record := &enr.Record{}
	record.Set(peerdas.Cgc(expected))

	actual, err := peerdas.CustodyGroupCountFromRecord(record)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
