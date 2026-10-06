package gloas

import (
	"bytes"
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ProcessSetSweepThresholdRequests applies each set sweep threshold request in order (EIP-8148).
func ProcessSetSweepThresholdRequests(_ context.Context, st state.BeaconState, requests []*enginev1.SetSweepThresholdRequest) error {
	if len(requests) > 0 {
		log.WithFields(logrus.Fields{
			"count": len(requests),
			"slot":  st.Slot(),
		}).Info("Processing EIP-8148 set sweep threshold requests from the parent payload")
	}

	for _, request := range requests {
		if err := processSetSweepThresholdRequest(st, request); err != nil {
			return errors.Wrap(err, "could not process set sweep threshold request")
		}
	}
	return nil
}

// processSetSweepThresholdRequest records a validator's custom withdrawal sweep threshold.
// Invalid requests are silently dropped by the spec; each reason is logged at debug level.
//
// https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8148/beacon-chain.md#new-process_set_sweep_threshold_request
func processSetSweepThresholdRequest(st state.BeaconState, request *enginev1.SetSweepThresholdRequest) error {
	if request == nil {
		return errors.New("nil set sweep threshold request")
	}

	idx, ok := st.ValidatorIndexByPubkey(bytesutil.ToBytes48(request.ValidatorPubkey))
	if !ok {
		rejectSweepThreshold(request, "pubkey is not in the validator registry", nil)
		return nil
	}

	val, err := st.ValidatorAtIndexReadOnly(idx)
	if err != nil {
		return errors.Wrapf(err, "could not get validator at index %d", idx)
	}

	if !val.HasCompoundingWithdrawalCredentials() {
		rejectSweepThreshold(request, "validator does not have compounding withdrawal credentials", logrus.Fields{"validatorIndex": idx})
		return nil
	}

	// withdrawal_credentials[12:] is the validator's execution address.
	creds := val.GetWithdrawalCredentials()
	if !bytes.Equal(creds[12:], request.SourceAddress) {
		rejectSweepThreshold(request, "source address is not the validator's withdrawal address", logrus.Fields{"validatorIndex": idx})
		return nil
	}

	cfg := params.BeaconConfig()
	if val.ExitEpoch() != cfg.FarFutureEpoch {
		rejectSweepThreshold(request, "validator is exiting", logrus.Fields{"validatorIndex": idx})
		return nil
	}

	current, err := st.ValidatorSweepThreshold(idx)
	if err != nil {
		return errors.Wrapf(err, "could not get sweep threshold at index %d", idx)
	}
	if current == request.Threshold {
		rejectSweepThreshold(request, "threshold is already set to this value", logrus.Fields{"validatorIndex": idx})
		return nil
	}

	balance, err := st.BalanceAtIndex(idx)
	if err != nil {
		return errors.Wrapf(err, "could not get balance at index %d", idx)
	}
	// A threshold below the current balance would let the validator sweep out immediately,
	// bypassing the partial withdrawal queue.
	if request.Threshold < balance {
		rejectSweepThreshold(request, "threshold is below the validator's current balance", logrus.Fields{"validatorIndex": idx, "balance": balance})
		return nil
	}
	if request.Threshold%cfg.EffectiveBalanceIncrement != 0 {
		rejectSweepThreshold(request, "threshold is not a multiple of EFFECTIVE_BALANCE_INCREMENT", logrus.Fields{"validatorIndex": idx})
		return nil
	}
	if request.Threshold < cfg.MinActivationBalance {
		rejectSweepThreshold(request, "threshold is below MIN_ACTIVATION_BALANCE", logrus.Fields{"validatorIndex": idx})
		return nil
	}
	if request.Threshold > cfg.MaxEffectiveBalanceElectra {
		rejectSweepThreshold(request, "threshold is above MAX_EFFECTIVE_BALANCE_ELECTRA", logrus.Fields{"validatorIndex": idx})
		return nil
	}

	if err := st.SetValidatorSweepThresholdAtIndex(idx, request.Threshold); err != nil {
		return errors.Wrapf(err, "could not set sweep threshold at index %d", idx)
	}

	sweepThresholdsProcessedTotal.Inc()
	log.WithFields(logrus.Fields{
		"validatorIndex":  idx,
		"pubkey":          fmt.Sprintf("%#x", request.ValidatorPubkey),
		"previous":        current,
		"threshold":       request.Threshold,
		"balance":         balance,
		"effectiveSweeps": "will sweep once balance and effective balance both exceed the threshold",
	}).Info("Applied EIP-8148 set sweep threshold request")

	return nil
}

// rejectSweepThreshold logs why a set sweep threshold request was dropped.
func rejectSweepThreshold(request *enginev1.SetSweepThresholdRequest, reason string, fields logrus.Fields) {
	all := logrus.Fields{
		"pubkey":        fmt.Sprintf("%#x", request.ValidatorPubkey),
		"sourceAddress": fmt.Sprintf("%#x", request.SourceAddress),
		"threshold":     request.Threshold,
		"reason":        reason,
	}
	for k, v := range fields {
		all[k] = v
	}
	log.WithFields(all).Debug("Rejected set sweep threshold request")
}
