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

func blockWithConsolidation(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	cr := &enginev1.ConsolidationRequest{}
	if err := cr.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	er := &enginev1.ExecutionRequests{
		Consolidations: []*enginev1.ConsolidationRequest{cr},
	}
	b := util.NewBeaconBlockElectra()
	b.Block.Body = &ethpb.BeaconBlockBodyElectra{ExecutionRequests: er}
	return blocks.NewSignedBeaconBlock(b)
}

func RunConsolidationTest(t *testing.T, config string) {
	common.RunConsolidationTest(t, config, version.String(version.Fulu), blockWithConsolidation, sszToState)
}
