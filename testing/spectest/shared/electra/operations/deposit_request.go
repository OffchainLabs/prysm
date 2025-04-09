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

func blockWithDepositRequest(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	dr := &enginev1.DepositRequest{}
	if err := dr.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	er := &enginev1.ExecutionRequests{
		Deposits: []*enginev1.DepositRequest{dr},
	}
	b := util.NewBeaconBlockElectra()
	b.Block.Body = &ethpb.BeaconBlockBodyElectra{ExecutionRequests: er}
	return blocks.NewSignedBeaconBlock(b)
}

func RunDepositRequestsTest(t *testing.T, config string) {
	common.RunDepositRequestsTest(t, config, version.String(version.Electra), blockWithDepositRequest, sszToState)
}
