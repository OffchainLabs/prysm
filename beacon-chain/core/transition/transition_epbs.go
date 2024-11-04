package transition

import (
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/epbs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func processExecution(state state.BeaconState, body interfaces.ReadOnlyBeaconBlockBody) (err error) {
	if body.Version() >= version.EPBS {
		state, err = blocks.ProcessWithdrawals(state, nil)
		if err != nil {
			return errors.Wrap(err, "could not process withdrawals")
		}
		return processExecutionPayloadHeader(state, body)
	}
	enabled, err := blocks.IsExecutionEnabled(state, body)
	if err != nil {
		return errors.Wrap(err, "could not check if execution is enabled")
	}
	if !enabled {
		return nil
	}
	executionData, err := body.Execution()
	if err != nil {
		return err
	}
	if state.Version() >= version.Capella {
		state, err = blocks.ProcessWithdrawals(state, executionData)
		if err != nil {
			return errors.Wrap(err, "could not process withdrawals")
		}
	}
	if err := blocks.ProcessPayload(state, body); err != nil {
		return errors.Wrap(err, "could not process execution data")
	}
	return nil
}

// This function verifies the signature as it is not necessarily signed by the
// proposer
func processExecutionPayloadHeader(state state.BeaconState, body interfaces.ReadOnlyBeaconBlockBody) (err error) {
	sh, err := body.SignedExecutionPayloadHeader()
	if err != nil {
		return err
	}
	header, err := sh.Header()
	if err != nil {
		return err
	}
	if err := epbs.ValidatePayloadHeaderSignature(state, sh); err != nil {
		return err
	}
	builderIndex := header.BuilderIndex()
	builder, err := state.ValidatorAtIndex(builderIndex)
	if err != nil {
		return err
	}
	epoch := slots.ToEpoch(state.Slot())
	if builder.ActivationEpoch > epoch || epoch >= builder.ExitEpoch {
		return errors.New("builder is not active")
	}
	if builder.Slashed {
		return errors.New("builder is slashed")
	}
	amount := header.Value()
	builderBalance, err := state.BalanceAtIndex(builderIndex)
	if err != nil {
		return err
	}
	if amount > primitives.Gwei(builderBalance) {
		return errors.New("builder has insufficient balance")
	}
	// sate.Slot == block.Slot because of process_slot
	if header.Slot() != state.Slot() {
		return errors.New("incorrect header slot")
	}
	// the state latest block header has the parent root because of
	// process_block_header
	blockHeader := state.LatestBlockHeader()
	if header.ParentBlockRoot() != [32]byte(blockHeader.ParentRoot) {
		return errors.New("incorrect parent block root")
	}
	lbh, err := state.LatestBlockHash()
	if err != nil {
		return err
	}
	if header.ParentBlockHash() != [32]byte(lbh) {
		return errors.New("incorrect latest block hash")
	}
	if err := state.UpdateBalancesAtIndex(builderIndex, builderBalance-uint64(amount)); err != nil {
		return err
	}
	if err := helpers.IncreaseBalance(state, blockHeader.ProposerIndex, uint64(amount)); err != nil {
		return err
	}
	headerEPBS, ok := header.Proto().(*enginev1.ExecutionPayloadHeaderEPBS)
	if !ok {
		return errors.New("not an ePBS execution payload header")
	}
	return state.SetLatestExecutionPayloadHeaderEPBS(headerEPBS)
}
