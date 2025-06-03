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

func blockWithProposerSlashing(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	ps := &ethpb.ProposerSlashing{}
	if err := ps.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockBellatrix()
	b.Block.Body = &ethpb.BeaconBlockBodyBellatrix{ProposerSlashings: []*ethpb.ProposerSlashing{ps}}
	return blocks.NewSignedBeaconBlock(b)
}

func RunProposerSlashingTest(t *testing.T, config string) {
	common.RunProposerSlashingTest(t, config, version.String(version.Bellatrix), blockWithProposerSlashing, sszToState)
}
