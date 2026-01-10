package transition

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	v "github.com/OffchainLabs/prysm/v7/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/pkg/errors"
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

func electraOperations(ctx context.Context, st state.BeaconState, block interfaces.ReadOnlyBeaconBlock) (state.BeaconState, error) {
	var err error

	// 6110 validations are in VerifyOperationLengths
	bb := block.Body()
	// Electra extends the altair operations.
	var exitInfo *v.ExitInfo
	hasSlashings := len(bb.ProposerSlashings()) > 0 || len(bb.AttesterSlashings()) > 0
	hasExits := len(bb.VoluntaryExits()) > 0
	if hasSlashings || hasExits {
		// ExitInformation is expensive to compute, only do it if we need it.
		exitInfo = v.ExitInformation(st)
		if err := helpers.UpdateTotalActiveBalanceCache(st, exitInfo.TotalActiveBalance); err != nil {
			return nil, errors.Wrap(err, "could not update total active balance cache")
		}
	}
	st, err = blocks.ProcessProposerSlashings(ctx, st, bb.ProposerSlashings(), exitInfo)
	if err != nil {
		return nil, errors.Wrap(ErrProcessProposerSlashingsFailed, err.Error())
	}
	st, err = blocks.ProcessAttesterSlashings(ctx, st, bb.AttesterSlashings(), exitInfo)
	if err != nil {
		return nil, errors.Wrap(ErrProcessAttesterSlashingsFailed, err.Error())
	}
	st, err = electra.ProcessAttestationsNoVerifySignature(ctx, st, block)
	if err != nil {
		return nil, errors.Wrap(ErrProcessAttestationsFailed, err.Error())
	}
	if _, err := electra.ProcessDeposits(ctx, st, bb.Deposits()); err != nil {
		return nil, errors.Wrap(ErrProcessDepositsFailed, err.Error())
	}
	st, err = blocks.ProcessVoluntaryExits(ctx, st, bb.VoluntaryExits(), exitInfo)
	if err != nil {
		return nil, errors.Wrap(ErrProcessVoluntaryExitsFailed, err.Error())
	}
	st, err = blocks.ProcessBLSToExecutionChanges(st, block)
	if err != nil {
		return nil, errors.Wrap(ErrProcessBLSChangesFailed, err.Error())
	}
	// new in electra
	requests, err := bb.ExecutionRequests()
	if err != nil {
		return nil, electra.NewExecReqError(errors.Wrap(err, "could not get execution requests").Error())
	}
	for _, d := range requests.Deposits {
		if d == nil {
			return nil, electra.NewExecReqError("nil deposit request")
		}
	}
	st, err = electra.ProcessDepositRequests(ctx, st, requests.Deposits)
	if err != nil {
		return nil, electra.NewExecReqError(errors.Wrap(err, "could not process deposit requests").Error())
	}

	for _, w := range requests.Withdrawals {
		if w == nil {
			return nil, electra.NewExecReqError("nil withdrawal request")
		}
	}
	st, err = electra.ProcessWithdrawalRequests(ctx, st, requests.Withdrawals)
	if err != nil {
		return nil, electra.NewExecReqError(errors.Wrap(err, "could not process withdrawal requests").Error())
	}
	for _, c := range requests.Consolidations {
		if c == nil {
			return nil, electra.NewExecReqError("nil consolidation request")
		}
	}
	if err := electra.ProcessConsolidationRequests(ctx, st, requests.Consolidations); err != nil {
		return nil, electra.NewExecReqError(errors.Wrap(err, "could not process consolidation requests").Error())
	}
	return st, nil
}
