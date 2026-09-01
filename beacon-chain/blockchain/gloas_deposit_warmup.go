package blockchain

import (
	"context"
	"math"
	"runtime"
	"sync/atomic"
	stdtime "time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sync/errgroup"
)

const (
	// Lead time before the fork, and the per-slot wall-clock budget spent pre-verifying.
	depositWarmupLeadEpochs = 2
	depositWarmupBudget     = 2 * stdtime.Second
)

var (
	gloasDepositWarmupCandidates = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gloas_deposit_warmup_candidates",
		Help: "Pending deposits whose signatures the Gloas fork upgrade will verify.",
	})
	gloasDepositWarmupWarmed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gloas_deposit_warmup_warmed",
		Help: "Gloas fork upgrade deposit signature candidates already pre-verified.",
	})
	gloasDepositWarmupRemaining = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gloas_deposit_warmup_remaining",
		Help: "Gloas fork upgrade deposit signature candidates not yet pre-verified. Must reach zero before the fork.",
	})
	gloasDepositWarmupVerifySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "gloas_deposit_warmup_verify_seconds",
		Help:    "Measured duration of a single pending deposit signature verification.",
		Buckets: []float64{0.0005, 0.001, 0.002, 0.004, 0.008, 0.016},
	})
)

func depositWarmupWorkers() int {
	return max(1, runtime.GOMAXPROCS(0)/2)
}

// runGloasDepositWarmup pre-verifies the deposit signatures the Gloas fork upgrade will consult.
func (s *Service) runGloasDepositWarmup() {
	if err := s.waitForSync(); err != nil {
		log.WithError(err).Error("Failed to wait for initial sync")
		return
	}
	cfg := params.BeaconConfig()
	if cfg.GloasForkEpoch == math.MaxUint64 || cfg.GloasForkEpoch < depositWarmupLeadEpochs {
		return
	}
	if slots.ToEpoch(s.CurrentSlot()) > cfg.GloasForkEpoch {
		return
	}
	if err := s.waitUntilEpoch(cfg.GloasForkEpoch-depositWarmupLeadEpochs, cfg.SlotDuration()); err != nil {
		return
	}
	// Starts after the aggregate deadline so the budget does not overlap an attestation or
	// aggregate deadline.
	ticker := slots.NewSlotTickerWithOffset(s.genesisTime, cfg.SlotComponentDuration(cfg.AggregateDueBPS), cfg.SlotDuration())
	defer ticker.Done()
	for {
		select {
		case slot := <-ticker.C():
			if slots.ToEpoch(slot) <= cfg.GloasForkEpoch {
				s.warmDepositSignatures()
				continue
			}
			// Warming is pointless once the fork epoch has passed, but a reorg across the
			// boundary re-runs the upgrade, so the results are only safe to drop once the fork
			// epoch can no longer be reverted.
			if s.FinalizedCheckpt().Epoch > cfg.GloasForkEpoch {
				cache.DepositSignature.Clear()
				return
			}
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting gloas deposit warmup routine")
			return
		}
	}
}

func (s *Service) warmDepositSignatures() {
	ctx, cancel := context.WithTimeout(s.ctx, depositWarmupBudget)
	defer cancel()

	st, err := s.HeadStateReadOnly(ctx)
	if err != nil {
		log.WithError(err).Debug("Could not get head state for gloas deposit warmup")
		return
	}
	candidates, err := pendingBuilderDepositCandidates(st)
	if err != nil {
		log.WithError(err).Debug("Could not collect gloas deposit warmup candidates")
		return
	}
	gloasDepositWarmupCandidates.Set(float64(len(candidates)))

	cold := coldDepositCandidates(candidates)
	gloasDepositWarmupWarmed.Set(float64(len(candidates) - len(cold)))
	gloasDepositWarmupRemaining.Set(float64(len(cold)))
	if len(cold) == 0 {
		return
	}

	start := stdtime.Now()
	verified := s.verifyDepositSignatures(ctx, cold)
	if verified == 0 {
		return
	}
	gloasDepositWarmupVerifySeconds.Observe(stdtime.Since(start).Seconds() / float64(verified))
	gloasDepositWarmupRemaining.Set(float64(len(cold) - verified))
}

// coldDepositCandidates drops candidates whose result is already cached. Byte-identical deposits
// share a key, so they also collapse to a single verification.
func coldDepositCandidates(candidates []*ethpb.Deposit_Data) []*ethpb.Deposit_Data {
	seen := make(map[[32]byte]bool, len(candidates))
	cold := make([]*ethpb.Deposit_Data, 0, len(candidates))
	for _, data := range candidates {
		key, err := data.HashTreeRoot()
		if err != nil {
			continue
		}
		if cache.DepositSignature.Has(key) || seen[key] {
			continue
		}
		seen[key] = true
		cold = append(cold, data)
	}
	return cold
}

func (s *Service) verifyDepositSignatures(ctx context.Context, deposits []*ethpb.Deposit_Data) int {
	var verified atomic.Int64
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(depositWarmupWorkers())
	for _, data := range deposits {
		if gctx.Err() != nil {
			break
		}
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}
			if _, err := helpers.IsValidDepositSignature(data); err != nil {
				return err
			}
			verified.Add(1)
			return nil
		})
	}
	if err := g.Wait(); err != nil && ctx.Err() == nil {
		log.WithError(err).Debug("Could not pre-verify pending deposit signature")
	}
	return int(verified.Load())
}

// The candidate set is per public key, not per deposit: is_pending_validator also verifies the
// non-builder-credential deposits sharing a queried public key.
func pendingBuilderDepositCandidates(st state.ReadOnlyBeaconState) ([]*ethpb.Deposit_Data, error) {
	pubkeys := make(map[[fieldparams.BLSPubkeyLength]byte]bool)
	if err := st.ForEachPendingDeposit(func(deposit *ethpb.PendingDeposit) error {
		if deposit == nil || !helpers.IsBuilderWithdrawalCredential(deposit.WithdrawalCredentials) {
			return nil
		}
		pubkeys[bytesutil.ToBytes48(deposit.PublicKey)] = true
		return nil
	}); err != nil {
		return nil, err
	}
	for pubkey := range pubkeys {
		if _, ok := st.ValidatorIndexByPubkey(pubkey); ok {
			delete(pubkeys, pubkey)
		}
	}
	if len(pubkeys) == 0 {
		return nil, nil
	}

	var candidates []*ethpb.Deposit_Data
	if err := st.ForEachPendingDeposit(func(deposit *ethpb.PendingDeposit) error {
		if deposit == nil || !pubkeys[bytesutil.ToBytes48(deposit.PublicKey)] {
			return nil
		}
		candidates = append(candidates, &ethpb.Deposit_Data{
			PublicKey:             bytesutil.SafeCopyBytes(deposit.PublicKey),
			WithdrawalCredentials: bytesutil.SafeCopyBytes(deposit.WithdrawalCredentials),
			Amount:                deposit.Amount,
			Signature:             bytesutil.SafeCopyBytes(deposit.Signature),
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return candidates, nil
}
