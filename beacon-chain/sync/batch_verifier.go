package sync

import (
	"context"
	"iter"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

const signatureVerificationInterval = 5 * time.Millisecond

type signatureVerifier struct {
	set     *bls.SignatureBatch
	resChan chan error
}

type errorWithSegment struct {
	err error
	// segment is only available if the batched verification failed
	segment *peerdas.CellProofBundleSegment
}

type kzgVerifier struct {
	sizeHint   int
	cellProofs iter.Seq[blocks.CellProofBundle]
	resChan    chan errorWithSegment
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
	kzgBatch := make([]*kzgVerifier, 0, 1)
	for {
		kzgBatch = kzgBatch[:0]
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
	verificationSet := &signatureVerifier{set: set, resChan: resChan}
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
	aggSet := bls.NewSet()
	for _, v := range verifierBatch {
		aggSet = aggSet.Join(v.set)
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
	for i := range verifierBatch {
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

func (s *Service) validateKZGProofs(ctx context.Context, sizeHint int, cellProofs iter.Seq[blocks.CellProofBundle]) error {
	_, span := trace.StartSpan(ctx, "sync.validateKZGProofs")
	defer span.End()

	timeout := time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second

	resChan := make(chan errorWithSegment, 1)
	verificationSet := &kzgVerifier{sizeHint: sizeHint, cellProofs: cellProofs, resChan: resChan}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case s.kzgChan <- verificationSet:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-ctx.Done():
		return ctx.Err() // parent context canceled, give up
	case errWithSegment := <-resChan:
		if errWithSegment.err != nil {
			err := errWithSegment.err
			log.WithError(err).Trace("Could not perform batch verification of cells")
			tracing.AnnotateError(span, err)
			if errWithSegment.segment == nil {
				return err
			}
			// We failed batch verification. Try again in this goroutine without batching
			return validateUnbatchedKZGProofs(ctx, *errWithSegment.segment)
		}
	}
	return nil
}

func validateUnbatchedKZGProofs(ctx context.Context, segment peerdas.CellProofBundleSegment) error {
	_, span := trace.StartSpan(ctx, "sync.validateUnbatchedColumnsKzg")
	defer span.End()
	start := time.Now()
	if err := segment.Verify(); err != nil {
		err = errors.Wrap(err, "could not verify")
		tracing.AnnotateError(span, err)
		return err
	}
	verification.DataColumnBatchKZGVerificationHistogram.WithLabelValues("fallback").Observe(float64(time.Since(start).Milliseconds()))
	return nil
}

func verifyKzgBatch(kzgBatch []*kzgVerifier) {
	if len(kzgBatch) == 0 {
		return
	}

	cellProofIters := make([]iter.Seq[blocks.CellProofBundle], 0, len(kzgBatch))
	var sizeHint int
	for _, kzgVerifier := range kzgBatch {
		sizeHint += kzgVerifier.sizeHint
		cellProofIters = append(cellProofIters, kzgVerifier.cellProofs)
	}

	var verificationErr error
	start := time.Now()
	segments, err := peerdas.BatchVerifyDataColumnsCellsKZGProofs(sizeHint, cellProofIters)
	if err != nil {
		verificationErr = errors.Wrap(err, "batch KZG verification failed")
	} else {
		verification.DataColumnBatchKZGVerificationHistogram.WithLabelValues("batch").Observe(float64(time.Since(start).Milliseconds()))
	}

	segmentAvailable := verificationErr != nil && len(segments) == len(kzgBatch)

	// Send the same result to all verifiers in the batch
	for i, verifier := range kzgBatch {
		var segment *peerdas.CellProofBundleSegment
		if segmentAvailable {
			failedSegment := segments[i]
			segment = &failedSegment
		}
		verifier.resChan <- errorWithSegment{
			err:     verificationErr,
			segment: segment,
		}
	}
}
