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

func blockWithSyncAggregate(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	sa := &ethpb.SyncAggregate{}
	if err := sa.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockDeneb()
	b.Block.Body = &ethpb.BeaconBlockBodyDeneb{SyncAggregate: sa}
	return blocks.NewSignedBeaconBlock(b)
}

func RunSyncCommitteeTest(t *testing.T, config string) {
	common.RunSyncCommitteeTest(t, config, version.String(version.Deneb), blockWithSyncAggregate, sszToState)
}
