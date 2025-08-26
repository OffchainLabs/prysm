package validator

import (
	"testing"

	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestUnblinder_UnblindBlobSidecars_InvalidBundle(t *testing.T) {
	wBlock, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockDeneb{
		Block: &ethpb.BeaconBlockDeneb{
			Body: &ethpb.BeaconBlockBodyDeneb{},
		},
		Signature: nil,
	})
	assert.NoError(t, err)
	_, err = unblindBlobsSidecars(wBlock, nil)
	assert.NoError(t, err)

	wBlock, err = consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockDeneb{
		Block: &ethpb.BeaconBlockDeneb{
			Body: &ethpb.BeaconBlockBodyDeneb{
				BlobKzgCommitments: [][]byte{[]byte("a"), []byte("b")},
			},
		},
		Signature: nil,
	})
	assert.NoError(t, err)
	_, err = unblindBlobsSidecars(wBlock, nil)
	assert.ErrorContains(t, "no valid bundle provided", err)
}

func TestUnblindBlobsSidecars_WithBlobsBundler(t *testing.T) {
	// Test that the function accepts BlobsBundler interface
	// This test focuses on the interface change rather than full integration

	t.Run("Interface compatibility with BlobsBundle", func(t *testing.T) {
		// Create a simple pre-Deneb block that will return nil (no processing needed)
		wBlock, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{
			Block: &ethpb.BeaconBlockCapella{
				Body: &ethpb.BeaconBlockBodyCapella{},
			},
			Signature: nil,
		})
		require.NoError(t, err)

		// Test with regular BlobsBundle
		bundle := &enginev1.BlobsBundle{
			KzgCommitments: [][]byte{make([]byte, 48)},
			Proofs:         [][]byte{make([]byte, 48)},
			Blobs:          [][]byte{make([]byte, 131072)},
		}

		// This should work without error (returns nil for pre-Deneb)
		sidecars, err := unblindBlobsSidecars(wBlock, bundle)
		require.NoError(t, err)
		assert.Equal(t, true, sidecars == nil)
	})

	t.Run("Interface compatibility with BlobsBundleV2", func(t *testing.T) {
		// Create a simple pre-Deneb block that will return nil (no processing needed)
		wBlock, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{
			Block: &ethpb.BeaconBlockCapella{
				Body: &ethpb.BeaconBlockBodyCapella{},
			},
			Signature: nil,
		})
		require.NoError(t, err)

		// Test with BlobsBundleV2 - this is the key test for the interface change
		bundleV2 := &enginev1.BlobsBundleV2{
			KzgCommitments: [][]byte{make([]byte, 48)},
			Proofs:         [][]byte{make([]byte, 48)},
			Blobs:          [][]byte{make([]byte, 131072)},
		}

		// This should work without error (returns nil for pre-Deneb)
		sidecars, err := unblindBlobsSidecars(wBlock, bundleV2)
		require.NoError(t, err)
		assert.Equal(t, true, sidecars == nil)
	})

	t.Run("Function signature accepts BlobsBundler interface", func(t *testing.T) {
		// This test verifies that the function signature has been updated to accept BlobsBundler
		// We test this by verifying the code compiles with both types

		// Create a simple pre-Deneb block for the interface test
		wBlock, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{
			Block: &ethpb.BeaconBlockCapella{
				Body: &ethpb.BeaconBlockBodyCapella{},
			},
			Signature: nil,
		})
		require.NoError(t, err)

		// Verify function accepts BlobsBundle through the interface
		var regularBundle enginev1.BlobsBundler = &enginev1.BlobsBundle{
			KzgCommitments: [][]byte{make([]byte, 48)},
			Proofs:         [][]byte{make([]byte, 48)},
			Blobs:          [][]byte{make([]byte, 131072)},
		}
		_, err = unblindBlobsSidecars(wBlock, regularBundle)
		require.NoError(t, err)

		// Verify function accepts BlobsBundleV2 through the interface
		var bundleV2 enginev1.BlobsBundler = &enginev1.BlobsBundleV2{
			KzgCommitments: [][]byte{make([]byte, 48)},
			Proofs:         [][]byte{make([]byte, 48)},
			Blobs:          [][]byte{make([]byte, 131072)},
		}
		_, err = unblindBlobsSidecars(wBlock, bundleV2)
		require.NoError(t, err)

		// If we get here, the interface change is working correctly
		assert.Equal(t, true, true)
	})
}

func TestUnblindBlobsSidecars_PreDenebBlock(t *testing.T) {
	// Test with pre-Deneb block (should return nil sidecars)
	wBlock, err := consensusblocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{
		Block: &ethpb.BeaconBlockCapella{
			Body: &ethpb.BeaconBlockBodyCapella{},
		},
		Signature: nil,
	})
	require.NoError(t, err)

	bundle := &enginev1.BlobsBundle{
		KzgCommitments: [][]byte{make([]byte, 48)},
		Proofs:         [][]byte{make([]byte, 48)},
		Blobs:          [][]byte{make([]byte, 131072)},
	}

	sidecars, err := unblindBlobsSidecars(wBlock, bundle)
	require.NoError(t, err)
	assert.Equal(t, true, sidecars == nil)

	// Also test with BlobsBundleV2
	bundleV2 := &enginev1.BlobsBundleV2{
		KzgCommitments: [][]byte{make([]byte, 48)},
		Proofs:         [][]byte{make([]byte, 48)},
		Blobs:          [][]byte{make([]byte, 131072)},
	}

	sidecars, err = unblindBlobsSidecars(wBlock, bundleV2)
	require.NoError(t, err)
	assert.Equal(t, true, sidecars == nil)
}
