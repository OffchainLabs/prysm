package gloas

import (
	"context"
	"errors"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

const (
	// GloasPreverifyWindowEpochs is the number of epochs before Gloas used to warm the cache.
	GloasPreverifyWindowEpochs = 2
	// MaxBuilderDepositsPerSlot caps new signature checks performed by one slot task.
	MaxBuilderDepositsPerSlot = 10_000
	// BuilderDepositBatchSize bounds the individual fallback cost of a failed batch.
	BuilderDepositBatchSize = 32
)

// PreverifyBuilderDepositsResult summarizes one bounded cache-warming pass.
type PreverifyBuilderDepositsResult struct {
	ValidBuilderSignatures     int
	InvalidBuilderSignatures   int
	ValidValidatorSignatures   int
	InvalidValidatorSignatures int
	ScannedPendingDeposits     int
	CachedDeposits             int
	BuilderPubkeys             int
	PendingDeposits            int
}

type pendingDepositsPreverificationProvider interface {
	PendingDepositsForPreverification() ([]*ethpb.PendingDeposit, error)
}

// PreverifyBuilderDeposits warms the signature cache for deposits inspected by Gloas onboarding.
func PreverifyBuilderDeposits(ctx context.Context, st state.ReadOnlyBeaconState, maxDeposits int) (result PreverifyBuilderDepositsResult, err error) {
	provider, ok := st.(builderDepositSignatureCacheProvider)
	if !ok || provider.BuilderDepositSignatureCache() == nil {
		return result, errors.New("state does not provide a builder deposit signature cache")
	}
	cache := provider.BuilderDepositSignatureCache()
	defer func() {
		result.CachedDeposits = cache.Len()
		result.BuilderPubkeys = cache.BuilderPubkeyLen()
	}()
	pendingDeposits, err := pendingDepositsForPreverification(st)
	if err != nil {
		return result, err
	}
	result.PendingDeposits = len(pendingDeposits)
	if maxDeposits <= 0 {
		return result, nil
	}

	queue := make([]*ethpb.PendingDeposit, 0, min(maxDeposits, len(pendingDeposits)))
	for _, deposit := range pendingDeposits {
		if len(queue) >= maxDeposits {
			break
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.ScannedPendingDeposits++
		if deposit == nil {
			continue
		}
		builderDeposit := helpers.IsBuilderWithdrawalCredential(deposit.WithdrawalCredentials)
		if builderDeposit {
			cache.TrackBuilderPubkey(deposit.PublicKey)
		} else if !cache.ShouldVerifyValidator(deposit.PublicKey) {
			continue
		}
		if valid, ok := cache.Get(deposit); ok {
			if !builderDeposit && valid {
				cache.MarkValidValidator(deposit.PublicKey)
			}
			continue
		}
		queue = append(queue, deposit)
	}

	for start := 0; start < len(queue); start += BuilderDepositBatchSize {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		end := min(start+BuilderDepositBatchSize, len(queue))
		validity, err := cache.VerifyBatch(ctx, queue[start:end])
		if err != nil {
			return result, err
		}
		for i, valid := range validity {
			deposit := queue[start+i]
			builderDeposit := helpers.IsBuilderWithdrawalCredential(deposit.WithdrawalCredentials)
			switch {
			case builderDeposit && valid:
				result.ValidBuilderSignatures++
			case builderDeposit:
				result.InvalidBuilderSignatures++
			case valid:
				result.ValidValidatorSignatures++
				cache.MarkValidValidator(deposit.PublicKey)
			default:
				result.InvalidValidatorSignatures++
			}
		}
	}
	return result, nil
}

func pendingDepositsForPreverification(st state.ReadOnlyBeaconState) ([]*ethpb.PendingDeposit, error) {
	if provider, ok := st.(pendingDepositsPreverificationProvider); ok {
		return provider.PendingDepositsForPreverification()
	}
	return st.PendingDeposits()
}
