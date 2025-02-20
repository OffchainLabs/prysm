package attestations

import (
	"context"
	"testing"
	"time"

	"github.com/prysmaticlabs/go-bitfield"
	"github.com/prysmaticlabs/prysm/v5/async"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
)

func TestPruneExpired_Ticker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	s, err := NewService(ctx, &Config{
		Pool:          NewPool(),
		pruneInterval: 250 * time.Millisecond,
	})
	require.NoError(t, err)

	ad1 := util.HydrateAttestationData(&ethpb.AttestationData{})

	ad2 := util.HydrateAttestationData(&ethpb.AttestationData{Slot: 1})

	atts := []ethpb.Att{
		&ethpb.Attestation{Data: ad1, AggregationBits: bitfield.Bitlist{0b1000, 0b1}, Signature: make([]byte, fieldparams.BLSSignatureLength)},
		&ethpb.Attestation{Data: ad2, AggregationBits: bitfield.Bitlist{0b1000, 0b1}, Signature: make([]byte, fieldparams.BLSSignatureLength)},
	}
	require.NoError(t, s.cfg.Pool.SaveUnaggregatedAttestations(atts))
	require.Equal(t, 2, s.cfg.Pool.UnaggregatedAttestationCount(), "Unexpected number of attestations")
	atts = []ethpb.Att{
		&ethpb.Attestation{Data: ad1, AggregationBits: bitfield.Bitlist{0b1101, 0b1}, Signature: make([]byte, fieldparams.BLSSignatureLength)},
		&ethpb.Attestation{Data: ad2, AggregationBits: bitfield.Bitlist{0b1101, 0b1}, Signature: make([]byte, fieldparams.BLSSignatureLength)},
	}
	require.NoError(t, s.cfg.Pool.SaveAggregatedAttestations(atts))
	assert.Equal(t, 2, s.cfg.Pool.AggregatedAttestationCount())
	for _, att := range atts {
		require.NoError(t, s.cfg.Pool.SaveBlockAttestation(att))
	}

	// Rewind back one epoch worth of time.
	s.genesisTime = uint64(prysmTime.Now().Unix()) - uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))

	go s.pruneExpired()

	done := make(chan struct{}, 1)
	async.RunEvery(ctx, 500*time.Millisecond, func() {
		atts, err := s.cfg.Pool.UnaggregatedAttestations()
		require.NoError(t, err)
		for _, attestation := range atts {
			if attestation.GetData().Slot == 0 {
				return
			}
		}
		for _, attestation := range s.cfg.Pool.AggregatedAttestations() {
			if attestation.GetData().Slot == 0 {
				return
			}
		}
		for _, attestation := range s.cfg.Pool.BlockAttestations() {
			if attestation.GetData().Slot == 0 {
				return
			}
		}
		if s.cfg.Pool.UnaggregatedAttestationCount() != 1 || s.cfg.Pool.AggregatedAttestationCount() != 1 {
			return
		}
		done <- struct{}{}
	})
	select {
	case <-done:
		// All checks are passed.
	case <-ctx.Done():
		t.Error("Test case takes too long to complete")
	}
}

func TestPruneExpired_PruneExpiredAtts(t *testing.T) {
	s, err := NewService(context.Background(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	ad1 := util.HydrateAttestationData(&ethpb.AttestationData{})

	ad2 := util.HydrateAttestationData(&ethpb.AttestationData{})

	att1 := &ethpb.Attestation{Data: ad1, AggregationBits: bitfield.Bitlist{0b1101}}
	att2 := &ethpb.Attestation{Data: ad1, AggregationBits: bitfield.Bitlist{0b1111}}
	att3 := &ethpb.Attestation{Data: ad2, AggregationBits: bitfield.Bitlist{0b1101}}
	att4 := &ethpb.Attestation{Data: ad2, AggregationBits: bitfield.Bitlist{0b1110}}
	atts := []ethpb.Att{att1, att2, att3, att4}
	require.NoError(t, s.cfg.Pool.SaveAggregatedAttestations(atts))
	for _, att := range atts {
		require.NoError(t, s.cfg.Pool.SaveBlockAttestation(att))
	}

	// Rewind back one epoch worth of time.
	s.genesisTime = uint64(prysmTime.Now().Unix()) - uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))

	s.pruneExpiredAtts()
	// All the attestations on slot 0 should be pruned.
	for _, attestation := range s.cfg.Pool.AggregatedAttestations() {
		if attestation.GetData().Slot == 0 {
			t.Error("Should be pruned")
		}
	}
	for _, attestation := range s.cfg.Pool.BlockAttestations() {
		if attestation.GetData().Slot == 0 {
			t.Error("Should be pruned")
		}
	}
}

func TestPruneExpired_Expired(t *testing.T) {
	s, err := NewService(context.Background(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	// Rewind back one epoch worth of time.
	s.genesisTime = uint64(prysmTime.Now().Unix()) - uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	assert.Equal(t, true, s.expired(0), "Should be expired")
	assert.Equal(t, false, s.expired(1), "Should not be expired")
}

func TestPruneExpired_ExpiredDeneb(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.DenebForkEpoch = 3
	params.OverrideBeaconConfig(cfg)

	s, err := NewService(context.Background(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	// Rewind back 4 epochs + 10 slots worth of time.
	s.genesisTime = uint64(prysmTime.Now().Unix()) - (4*uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot)) + 10)
	secondEpochStart := primitives.Slot(2 * uint64(params.BeaconConfig().SlotsPerEpoch))
	thirdEpochStart := primitives.Slot(3 * uint64(params.BeaconConfig().SlotsPerEpoch))

	assert.Equal(t, true, s.expired(secondEpochStart), "Should be expired")
	assert.Equal(t, false, s.expired(thirdEpochStart), "Should not be expired")
}
