package operations

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	common "github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/common/operations"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func blockWithVoluntaryExit(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	e := &ethpb.SignedVoluntaryExit{}
	if err := e.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockCapella()
	b.Block.Body = &ethpb.BeaconBlockBodyCapella{VoluntaryExits: []*ethpb.SignedVoluntaryExit{e}}
	return blocks.NewSignedBeaconBlock(b)
}

func RunVoluntaryExitTest(t *testing.T, config string) {
	common.RunVoluntaryExitTest(t, config, version.String(version.Capella), blockWithVoluntaryExit, sszToState)
}
