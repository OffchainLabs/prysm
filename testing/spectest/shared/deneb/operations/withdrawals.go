package operations

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	common "github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/common/operations"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func blockWithWithdrawals(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	e := &enginev1.ExecutionPayloadDeneb{}
	if err := e.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockDeneb()
	b.Block.Body = &ethpb.BeaconBlockBodyDeneb{ExecutionPayload: e}
	return blocks.NewSignedBeaconBlock(b)
}

func RunWithdrawalsTest(t *testing.T, config string) {
	common.RunWithdrawalsTest(t, config, version.String(version.Deneb), blockWithWithdrawals, sszToState)
}
