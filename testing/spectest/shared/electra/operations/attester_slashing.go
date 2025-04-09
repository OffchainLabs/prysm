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

func blockWithAttesterSlashing(asSSZ []byte) (interfaces.SignedBeaconBlock, error) {
	as := &ethpb.AttesterSlashingElectra{}
	if err := as.UnmarshalSSZ(asSSZ); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockElectra()
	b.Block.Body = &ethpb.BeaconBlockBodyElectra{AttesterSlashings: []*ethpb.AttesterSlashingElectra{as}}
	return blocks.NewSignedBeaconBlock(b)
}

func RunAttesterSlashingTest(t *testing.T, config string) {
	common.RunAttesterSlashingTest(t, config, version.String(version.Electra), blockWithAttesterSlashing, sszToState)
}
