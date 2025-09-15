package beacon

import (
	"encoding/json"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	rpctesting "github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/shared/testing"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

// TestBlocks_NewSignedBeaconBlock_EquivocationFix tests that blocks.NewSignedBeaconBlock
// correctly handles the fixed case where genericBlock.Block is passed instead of genericBlock
func TestBlocks_NewSignedBeaconBlock_EquivocationFix(t *testing.T) {
	// Parse the Phase0 JSON block
	var block structs.SignedBeaconBlock
	err := json.Unmarshal([]byte(rpctesting.Phase0Block), &block)
	require.NoError(t, err)

	// Convert to generic format
	genericBlock, err := block.ToGeneric()
	require.NoError(t, err)

	// Test the FIX: pass genericBlock.Block instead of genericBlock
	// This is what our fix changed in handlers.go line 704 and 858
	_, err = blocks.NewSignedBeaconBlock(genericBlock.Block)
	require.NoError(t, err, "NewSignedBeaconBlock should work with genericBlock.Block")

	// Test the BROKEN version: pass genericBlock directly (this should fail)
	_, err = blocks.NewSignedBeaconBlock(genericBlock)
	if err == nil {
		t.Errorf("NewSignedBeaconBlock should fail with whole genericBlock but succeeded")
	}
}
