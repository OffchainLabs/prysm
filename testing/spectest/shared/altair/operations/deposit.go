package operations

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	common "github.com/OffchainLabs/prysm/v6/testing/spectest/shared/common/operations"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func blockWithDeposit(ssz []byte) (interfaces.SignedBeaconBlock, error) {
	d := &ethpb.Deposit{}
	if err := d.UnmarshalSSZ(ssz); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockAltair()
	b.Block.Body = &ethpb.BeaconBlockBodyAltair{Deposits: []*ethpb.Deposit{d}}
	return blocks.NewSignedBeaconBlock(b)
}

func RunDepositTest(t *testing.T, config string) {
	common.RunDepositTest(t, config, version.String(version.Altair), blockWithDeposit, altair.ProcessDeposits, sszToState)
}
