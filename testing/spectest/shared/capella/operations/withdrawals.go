package operations

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	common "github.com/OffchainLabs/prysm/v6/testing/spectest/shared/common/operations"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func blockWithWithdrawals(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	e := &enginev1.ExecutionPayloadCapella{}
	if err := e.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockCapella()
	b.Block.Body = &ethpb.BeaconBlockBodyCapella{ExecutionPayload: e}
	return blocks.NewSignedBeaconBlock(b)
}

func RunWithdrawalsTest(t *testing.T, config string) {
	common.RunWithdrawalsTest(t, config, version.String(version.Capella), blockWithWithdrawals, sszToState)
}
