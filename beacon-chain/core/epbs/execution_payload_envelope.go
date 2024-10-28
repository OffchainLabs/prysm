package epbs

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/electra"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

// ValidatePayloadStateTransition performs the process_execution_payload
// function.
func ValidatePayloadStateTransition(
	ctx context.Context,
	preState state.BeaconState,
	envelope interfaces.ROExecutionPayloadEnvelope,
) error {
	if err := validateAgainstHeader(ctx, preState, envelope); err != nil {
		return err
	}
	committedHeader, err := preState.LatestExecutionPayloadHeaderEPBS()
	if err != nil {
		return err
	}
	if err := validateAgainstCommittedBid(committedHeader, envelope); err != nil {
		return err
	}
	if err := ProcessPayloadStateTransition(ctx, preState, envelope); err != nil {
		return err
	}
	return checkPostStateRoot(ctx, preState, envelope)
}

func ProcessPayloadStateTransition(
	ctx context.Context,
	preState state.BeaconState,
	envelope interfaces.ROExecutionPayloadEnvelope,
) error {
	er := envelope.ExecutionRequests()
	preState, err := electra.ProcessDepositRequests(ctx, preState, er.Deposits)
	if err != nil {
		return errors.Wrap(err, "could not process deposit receipts")
	}
	preState, err = electra.ProcessWithdrawalRequests(ctx, preState, er.Withdrawals)
	if err != nil {
		return errors.Wrap(err, "could not process ercution layer withdrawal requests")
	}
	if err := electra.ProcessConsolidationRequests(ctx, preState, er.Consolidations); err != nil {
		return errors.Wrap(err, "could not process consolidation requests")
	}
	payload, err := envelope.Execution()
	if err != nil {
		return errors.Wrap(err, "could not get execution payload")
	}
	if err := preState.SetLatestBlockHash(payload.BlockHash()); err != nil {
		return err
	}
	return preState.SetLatestFullSlot(preState.Slot())
}

func validateAgainstHeader(
	ctx context.Context,
	preState state.BeaconState,
	envelope interfaces.ROExecutionPayloadEnvelope,
) error {
	blockHeader := preState.LatestBlockHeader()
	if blockHeader == nil {
		return errors.New("invalid nil latest block header")
	}
	if len(blockHeader.StateRoot) == 0 || [32]byte(blockHeader.StateRoot) == [32]byte{} {
		prevStateRoot, err := preState.HashTreeRoot(ctx)
		if err != nil {
			return errors.Wrap(err, "could not compute previous state root")
		}
		blockHeader.StateRoot = prevStateRoot[:]
		if err := preState.SetLatestBlockHeader(blockHeader); err != nil {
			return errors.Wrap(err, "could not set latest block header")
		}
	}
	blockHeaderRoot, err := blockHeader.HashTreeRoot()
	if err != nil {
		return err
	}
	beaconBlockRoot := envelope.BeaconBlockRoot()
	if blockHeaderRoot != beaconBlockRoot {
		return fmt.Errorf("beacon block root does not match previous header, got: %#x wanted: %#x", beaconBlockRoot, blockHeaderRoot)
	}
	return nil
}

func validateAgainstCommittedBid(
	committedHeader *enginev1.ExecutionPayloadHeaderEPBS,
	envelope interfaces.ROExecutionPayloadEnvelope,
) error {
	builderIndex := envelope.BuilderIndex()
	if committedHeader.BuilderIndex != builderIndex {
		return errors.New("builder index does not match committed header")
	}
	kzgRoot, err := envelope.BlobKzgCommitmentsRoot()
	if err != nil {
		return err
	}
	if [32]byte(committedHeader.BlobKzgCommitmentsRoot) != kzgRoot {
		return errors.New("blob KZG commitments root does not match committed header")
	}
	return nil
}

func checkPostStateRoot(
	ctx context.Context,
	preState state.BeaconState,
	envelope interfaces.ROExecutionPayloadEnvelope,
) error {
	stateRoot, err := preState.HashTreeRoot(ctx)
	if err != nil {
		return err
	}
	envelopeStateRoot := envelope.StateRoot()
	if stateRoot != envelopeStateRoot {
		return errors.New("state root mismatch")
	}
	return nil
}
