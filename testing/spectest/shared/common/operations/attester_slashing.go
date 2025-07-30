package operations

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	v "github.com/OffchainLabs/prysm/v6/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
)

func RunAttesterSlashingTest(t *testing.T, config string, fork string, block blockWithSSZObject, sszToState SSZToState) {
	runSlashingTest(t, config, fork, "attester_slashing", block, sszToState, func(ctx context.Context, s state.BeaconState, b interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
		return blocks.ProcessAttesterSlashings(ctx, s, b.Block().Body().AttesterSlashings(), v.ExitInformation(s))
	})
}
