package operations

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	common "github.com/OffchainLabs/prysm/v7/testing/spectest/shared/common/operations"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func blockWithSignedExecutionPayloadBid(bidSSZ []byte) (interfaces.SignedBeaconBlock, error) {
	signedBid := &ethpb.SignedExecutionPayloadBid{}
	if err := signedBid.UnmarshalSSZ(bidSSZ); err != nil {
		return nil, err
	}
	blk := util.NewBeaconBlockGloas()
	blk.Block.Slot = signedBid.Message.Slot
	blk.Block.ParentRoot = signedBid.Message.ParentBlockRoot
	blk.Block.Body.SignedExecutionPayloadBid = signedBid
	return blocks.NewSignedBeaconBlock(blk)
}

func RunExecutionPayloadBidTest(t *testing.T, config string) {
	common.RunExecutionPayloadBidTest(t, config, version.String(version.Gloas), blockWithSignedExecutionPayloadBid, sszToState)
}
