package operations

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	common "github.com/OffchainLabs/prysm/v6/testing/spectest/shared/common/operations"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func blockWithBlsChange(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	c := &ethpb.SignedBLSToExecutionChange{}
	if err := c.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockDeneb()
	b.Block.Body = &ethpb.BeaconBlockBodyDeneb{BlsToExecutionChanges: []*ethpb.SignedBLSToExecutionChange{c}}
	return blocks.NewSignedBeaconBlock(b)
}

func RunBLSToExecutionChangeTest(t *testing.T, config string) {
	common.RunBLSToExecutionChangeTest(t, config, version.String(version.Deneb), blockWithBlsChange, sszToState)
}
