package operations

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/altair"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	common "github.com/prysmaticlabs/prysm/v5/testing/spectest/shared/common/operations"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func blockWithDeposit(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	d := &ethpb.Deposit{}
	if err := d.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockBellatrix()
	b.Block.Body = &ethpb.BeaconBlockBodyBellatrix{Deposits: []*ethpb.Deposit{d}}
	return blocks.NewSignedBeaconBlock(b)
}

func RunDepositTest(t *testing.T, config string) {
	common.RunDepositTest(t, config, version.String(version.Bellatrix), blockWithDeposit, altair.ProcessDeposits, sszToState)
}
