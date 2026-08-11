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

// rejectSweepThreshold logs why a set sweep threshold request was dropped. The spec
// discards invalid requests silently, which makes a devnet impossible to debug, so
// every reason is surfaced at debug level.
func rejectSweepThreshold(request *enginev1.SetSweepThresholdRequest, reason string, fields logrus.Fields) error {
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

	return nil
}

// ProcessSetSweepThresholdRequests applies each set sweep threshold request in order.
func ProcessSetSweepThresholdRequests(_ context.Context, s state.BeaconState, requests []*enginev1.SetSweepThresholdRequest) error {
	if len(requests) > 0 {
		log.WithFields(logrus.Fields{
			"count": len(requests),
			"slot":  s.Slot(),
		}).Info("Processing EIP-8148 set sweep threshold requests from the parent payload")
	}

	for _, request := range requests {
		if err := processSetSweepThresholdRequest(s, request); err != nil {
			return errors.Wrap(err, "could not process set sweep threshold request")
		}
	}
	return nil
}

// processSetSweepThresholdRequest records a validator's custom withdrawal sweep threshold.
// https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8148/beacon-chain.md#new-process_set_sweep_threshold_request
func processSetSweepThresholdRequest(s state.BeaconState, request *enginev1.SetSweepThresholdRequest) error {
	if request == nil {
		return errors.New("nil set sweep threshold request")
	}

	idx, ok := s.ValidatorIndexByPubkey(bytesutil.ToBytes48(request.ValidatorPubkey))
	if !ok {
		return rejectSweepThreshold(request, "pubkey is not in the validator registry", nil)
	}

	val, err := s.ValidatorAtIndexReadOnly(idx)
	if err != nil {
		return fmt.Errorf("validator at index read only: %w", err)
	}

	if !val.HasCompoundingWithdrawalCredentials() {
		return rejectSweepThreshold(request, "validator does not have 0x02 compounding credentials", logrus.Fields{
			"validatorIndex": idx,
			"credentials":    fmt.Sprintf("%#x", val.GetWithdrawalCredentials()),
		})
	}

	// withdrawal_credentials[12:] is the validator's execution address.
	creds := val.GetWithdrawalCredentials()
	if !bytes.Equal(creds[12:], request.SourceAddress) {
		return rejectSweepThreshold(request, "source address is not the validator's withdrawal address", logrus.Fields{
			"validatorIndex":    idx,
			"withdrawalAddress": fmt.Sprintf("%#x", creds[12:]),
		})
	}

	cfg := params.BeaconConfig()
	if val.ExitEpoch() != cfg.FarFutureEpoch {
		return rejectSweepThreshold(request, "validator is exiting", logrus.Fields{
			"validatorIndex": idx,
			"exitEpoch":      val.ExitEpoch(),
		})
	}

	current, err := s.ValidatorSweepThreshold(idx)
	if err != nil {
		return errors.Wrapf(err, "could not get sweep threshold at index %d", idx)
	}
	if current == request.Threshold {
		return rejectSweepThreshold(request, "threshold is already set to this value", logrus.Fields{
			"validatorIndex": idx,
		})
	}

	balance, err := s.BalanceAtIndex(idx)
	if err != nil {
		return errors.Wrapf(err, "could not get balance at index %d", idx)
	}
	// A threshold below the current balance would let the validator sweep out immediately,
	// bypassing the partial withdrawal queue. Lower it by requesting a partial withdrawal
	// first, waiting for it to be processed, then setting the threshold.
	if request.Threshold < balance {
		return rejectSweepThreshold(request, "threshold is below the validator's current balance", logrus.Fields{
			"validatorIndex": idx,
			"balance":        balance,
			"shortfallGwei":  balance - request.Threshold,
		})
	}

	if request.Threshold%cfg.EffectiveBalanceIncrement != 0 {
		return rejectSweepThreshold(request, "threshold is not a multiple of EFFECTIVE_BALANCE_INCREMENT", logrus.Fields{
			"validatorIndex": idx,
			"increment":      cfg.EffectiveBalanceIncrement,
		})
	}
	if request.Threshold < cfg.MinSweepThreshold() {
		return rejectSweepThreshold(request, "threshold is below MIN_SWEEP_THRESHOLD", logrus.Fields{
			"validatorIndex":    idx,
			"minSweepThreshold": cfg.MinSweepThreshold(),
		})
	}
	if request.Threshold > cfg.MaxEffectiveBalanceElectra {
		return rejectSweepThreshold(request, "threshold is above MAX_EFFECTIVE_BALANCE_ELECTRA", logrus.Fields{
			"validatorIndex": idx,
			"maxThreshold":   cfg.MaxEffectiveBalanceElectra,
		})
	}

	if err := s.SetValidatorSweepThresholdAtIndex(idx, request.Threshold); err != nil {
		return fmt.Errorf("set validator sweep threshold at index: %w", err)
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
