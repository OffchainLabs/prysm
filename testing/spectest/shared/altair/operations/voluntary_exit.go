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

func blockWithVoluntaryExit(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	e := &ethpb.SignedVoluntaryExit{}
	if err := e.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockAltair()
	b.Block.Body = &ethpb.BeaconBlockBodyAltair{VoluntaryExits: []*ethpb.SignedVoluntaryExit{e}}
	return blocks.NewSignedBeaconBlock(b)
}

func RunVoluntaryExitTest(t *testing.T, config string) {
	common.RunVoluntaryExitTest(t, config, version.String(version.Altair), blockWithVoluntaryExit, sszToState)
}
