package gloas

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// InitiateBuilderExit initiates the exit of a builder by setting its withdrawable epoch.
//
//	<spec fn="initiate_builder_exit" fork="gloas" hash="3da938d5">
//	def initiate_builder_exit(state: BeaconState, builder_index: BuilderIndex) -> None:
//	    """
//	    Initiate the exit of the builder with index ``index``.
//	    """
//	    # Set builder exit epoch
//	    builder = state.builders[builder_index]
//	    builder.withdrawable_epoch = get_current_epoch(state) + MIN_BUILDER_WITHDRAWABILITY_DELAY
//	</spec>
func InitiateBuilderExit(s state.BeaconState, builderIndex primitives.BuilderIndex) error {
	builder, err := s.Builder(builderIndex)
	if err != nil {
		return err
	}
	currentEpoch := slots.ToEpoch(s.Slot())
	builder.WithdrawableEpoch = currentEpoch + params.BeaconConfig().MinBuilderWithdrawabilityDelay
	return s.UpdateBuilderAtIndex(builderIndex, builder)
}
