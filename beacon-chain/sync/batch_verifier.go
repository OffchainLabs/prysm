package sync

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

const signatureVerificationInterval = 5 * time.Millisecond

type signatureVerifier struct {
	set     *bls.SignatureBatch
	resChan chan error
}

type kzgVerifier struct {
	dataColumns []blocks.RODataColumn
	resChan     chan error
}

// A routine that runs in the background to perform batch
// verifications of incoming messages from gossip.
func (s *Service) verifierRoutine() {
	verifierBatch := make([]*signatureVerifier, 0)
	ticker := time.NewTicker(signatureVerificationInterval)
	for {
		select {
		case <-s.ctx.Done():
			// Clean up currently utilised resources.
			ticker.Stop()
			for i := 0; i < len(verifierBatch); i++ {
				verifierBatch[i].resChan <- s.ctx.Err()
			}
			return
		case sig := <-s.signatureChan:
			verifierBatch = append(verifierBatch, sig)
			if len(verifierBatch) >= s.cfg.batchVerifierLimit {
				verifyBatch(verifierBatch)
				verifierBatch = []*signatureVerifier{}
			}
		case <-ticker.C:
			if len(verifierBatch) > 0 {
				verifyBatch(verifierBatch)
				verifierBatch = []*signatureVerifier{}
			}
		}
	}
}

// A routine that runs in the background to perform batch
// KZG verifications by draining the channel and processing all pending requests.
func (s *Service) kzgVerifierRoutine() {
	for {
		kzgBatch := make([]*kzgVerifier, 0, 1)
		select {
		case <-s.ctx.Done():
			return
		case kzg := <-s.kzgChan:
			kzgBatch = append(kzgBatch, kzg)
		}
		for {
			select {
			case <-s.ctx.Done():
				return
			case kzg := <-s.kzgChan:
				kzgBatch = append(kzgBatch, kzg)
				continue
			default:
				verifyKzgBatch(kzgBatch)
			}
			break
		}
	}
}

func (s *Service) validateWithBatchVerifier(ctx context.Context, message string, set *bls.SignatureBatch) (pubsub.ValidationResult, error) {
	_, span := trace.StartSpan(ctx, "sync.validateWithBatchVerifier")
	defer span.End()

	resChan := make(chan error)
	verificationSet := &signatureVerifier{set: set.Copy(), resChan: resChan}
	s.signatureChan <- verificationSet

	resErr := <-resChan
	// If verification fails we fallback to individual verification
	// of each signature set.
	if resErr != nil {
		log.WithError(resErr).Tracef("Could not perform batch verification of %s", message)
		verified, err := set.Verify()
		if err != nil {
			verErr := errors.Wrapf(err, "Could not verify %s", message)
			tracing.AnnotateError(span, verErr)
			return pubsub.ValidationReject, verErr
		}
		if !verified {
			verErr := errors.Errorf("Verification of %s failed", message)
			tracing.AnnotateError(span, verErr)
			return pubsub.ValidationReject, verErr
		}
	}
	return pubsub.ValidationAccept, nil
}

func verifyBatch(verifierBatch []*signatureVerifier) {
	if len(verifierBatch) == 0 {
		return
	}
	aggSet := verifierBatch[0].set

	for i := 1; i < len(verifierBatch); i++ {
		aggSet = aggSet.Join(verifierBatch[i].set)
	}
	var verificationErr error

	aggSet, verificationErr = performBatchAggregation(aggSet)
	if verificationErr == nil {
		verified, err := aggSet.Verify()
		switch {
		case err != nil:
			verificationErr = err
		case !verified:
			verificationErr = errors.New("batch signature verification failed")
		}
	}
	for i := 0; i < len(verifierBatch); i++ {
		verifierBatch[i].resChan <- verificationErr
	}
}

func performBatchAggregation(aggSet *bls.SignatureBatch) (*bls.SignatureBatch, error) {
	currLen := len(aggSet.Signatures)
	num, aggSet, err := aggSet.RemoveDuplicates()
	if err != nil {
		return nil, err
	}
	duplicatesRemovedCounter.Add(float64(num))
	// Aggregate batches in the provided signature batch.
	aggSet, err = aggSet.AggregateBatch()
	if err != nil {
		return nil, err
	}
	// Record number of signature sets successfully batched.
	if currLen > len(aggSet.Signatures) {
		numberOfSetsAggregated.Observe(float64(currLen - len(aggSet.Signatures)))
	}
	return aggSet, nil
}

func (s *Service) validateWithKzgBatchVerifier(ctx context.Context, dataColumns []blocks.RODataColumn) (pubsub.ValidationResult, error) {
	_, span := trace.StartSpan(ctx, "sync.validateWithKzgBatchVerifier")
	defer span.End()

	timeout := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second

	resChan := make(chan error)
	verificationSet := &kzgVerifier{dataColumns: dataColumns, resChan: resChan}
	s.kzgChan <- verificationSet

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return pubsub.ValidationIgnore, ctx.Err() // parent context canceled, give up
	case err := <-resChan:
		if err != nil {
			log.WithError(err).Trace("Could not perform batch verification")
			tracing.AnnotateError(span, err)
			return s.validateUnbatchedColumnsKzg(ctx, dataColumns)
		}
	}
	return pubsub.ValidationAccept, nil
}

func (s *Service) validateUnbatchedColumnsKzg(ctx context.Context, columns []blocks.RODataColumn) (pubsub.ValidationResult, error) {
	_, span := trace.StartSpan(ctx, "sync.validateUnbatchedColumnsKzg")
	defer span.End()
	if err := peerdas.VerifyDataColumnsSidecarKZGProofs(columns); err != nil {
		err = errors.Wrap(err, "could not verify")
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}
	return pubsub.ValidationAccept, nil
}

func verifyKzgBatch(kzgBatch []*kzgVerifier) {
	if len(kzgBatch) == 0 {
		return
	}

	allDataColumns := make([]blocks.RODataColumn, 0, len(kzgBatch))
	for _, kzgVerifier := range kzgBatch {
		allDataColumns = append(allDataColumns, kzgVerifier.dataColumns...)
	}

	var verificationErr error
	err := peerdas.VerifyDataColumnsSidecarKZGProofs(allDataColumns)
	if err != nil {
		verificationErr = errors.Wrap(err, "batch KZG verification failed")
	}

	// Send the same result to all verifiers in the batch
	for _, verifier := range kzgBatch {
		verifier.resChan <- verificationErr
	}
}
