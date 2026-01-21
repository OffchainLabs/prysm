package operations

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/operations"
)

func blockWithSignedExecutionPayloadBid(blockSSZ []byte) (interfaces.SignedBeaconBlock, error) {
	var block ethpb.BeaconBlockGloas
	if err := block.UnmarshalSSZ(blockSSZ); err != nil {
		return nil, err
	}
	return blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockGloas{Block: &block})
}

func RunExecutionPayloadBidTest(t *testing.T, config string) {
	common.RunExecutionPayloadBidTest(t, config, version.String(version.Gloas), blockWithSignedExecutionPayloadBid, sszToState)
}
