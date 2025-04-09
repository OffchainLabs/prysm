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

func blockWithProposerSlashing(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	ps := &ethpb.ProposerSlashing{}
	if err := ps.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlock()
	b.Block.Body = &ethpb.BeaconBlockBody{ProposerSlashings: []*ethpb.ProposerSlashing{ps}}
	return blocks.NewSignedBeaconBlock(b)
}

func RunProposerSlashingTest(t *testing.T, config string) {
	common.RunProposerSlashingTest(t, config, version.String(version.Phase0), blockWithProposerSlashing, sszToState)
}
