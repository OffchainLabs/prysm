package electra

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/blocks"
	v "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/validators"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
)

var (
	ProcessBLSToExecutionChanges = blocks.ProcessBLSToExecutionChanges
	ProcessVoluntaryExits        = blocks.ProcessVoluntaryExits
	ProcessAttesterSlashings     = blocks.ProcessAttesterSlashings
	ProcessProposerSlashings     = blocks.ProcessProposerSlashings
)

// ProcessOperations
//
// Spec definition:
//
//  def process_operations(state: BeaconState, body: BeaconBlockBody) -> None:
//      # [Modified in Electra:EIP6110]
//      # Disable former deposit mechanism once all prior deposits are processed
//      eth1_deposit_index_limit = min(state.eth1_data.deposit_count, state.deposit_requests_start_index)
//      if state.eth1_deposit_index < eth1_deposit_index_limit:
//          assert len(body.deposits) == min(MAX_DEPOSITS, eth1_deposit_index_limit - state.eth1_deposit_index)
//      else:
//          assert len(body.deposits) == 0
//
//      def for_ops(operations: Sequence[Any], fn: Callable[[BeaconState, Any], None]) -> None:
//          for operation in operations:
//              fn(state, operation)
//
//      for_ops(body.proposer_slashings, process_proposer_slashing)
//      for_ops(body.attester_slashings, process_attester_slashing)
//      for_ops(body.attestations, process_attestation)  # [Modified in Electra:EIP7549]
//      for_ops(body.deposits, process_deposit)  # [Modified in Electra:EIP7251]
//      for_ops(body.voluntary_exits, process_voluntary_exit)  # [Modified in Electra:EIP7251]
//      for_ops(body.bls_to_execution_changes, process_bls_to_execution_change)
//      for_ops(body.execution_payload.deposit_requests, process_deposit_request)  # [New in Electra:EIP6110]
//      # [New in Electra:EIP7002:EIP7251]
//      for_ops(body.execution_payload.withdrawal_requests, process_withdrawal_request)
//      # [New in Electra:EIP7251]
//      for_ops(body.execution_payload.consolidation_requests, process_consolidation_request)

func ProcessOperations(
	ctx context.Context,
	st state.BeaconState,
	block interfaces.ReadOnlyBeaconBlock) (state.BeaconState, error) {
	// 6110 validations are in VerifyOperationLengths
	bb := block.Body()
	// Electra extends the altair operations.
	st, err := ProcessProposerSlashings(ctx, st, bb.ProposerSlashings(), v.SlashValidator)
	if err != nil {
		return nil, errors.Wrap(err, "could not process altair proposer slashing")
	}
	st, err = ProcessAttesterSlashings(ctx, st, bb.AttesterSlashings(), v.SlashValidator)
	if err != nil {
		return nil, errors.Wrap(err, "could not process altair attester slashing")
	}
	st, err = ProcessAttestationsNoVerifySignature(ctx, st, block)
	if err != nil {
		return nil, errors.Wrap(err, "could not process altair attestation")
	}
	if _, err := ProcessDeposits(ctx, st, bb.Deposits()); err != nil { // new in electra
		return nil, errors.Wrap(err, "could not process altair deposit")
	}
	st, err = ProcessVoluntaryExits(ctx, st, bb.VoluntaryExits())
	if err != nil {
		return nil, errors.Wrap(err, "could not process voluntary exits")
	}
	st, err = ProcessBLSToExecutionChanges(st, block)
	if err != nil {
		return nil, errors.Wrap(err, "could not process bls-to-execution changes")
	}
	// new in electra
	requests, err := bb.ExecutionRequests()
	if err != nil {
		return nil, errors.Wrap(err, "could not get execution requests")
	}
	st, err = ProcessDepositRequests(ctx, st, requests.Deposits)
	if err != nil {
		return nil, errors.Wrap(err, "could not process deposit requests")
	}
	st, err = ProcessWithdrawalRequests(ctx, st, requests.Withdrawals)
	if err != nil {
		return nil, errors.Wrap(err, "could not process withdrawal requests")
	}
	if err := ProcessConsolidationRequests(ctx, st, requests.Consolidations); err != nil {
		return nil, fmt.Errorf("could not process consolidation requests: %w", err)
	}
	return st, nil
}
