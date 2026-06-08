package sync

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestCommitteeIndexBeaconAttestationSubscriber_CanSaveAggregatedAttestation(t *testing.T) {
	r := &Service{
		cfg: &config{
			attPool: attestations.NewPool(),
		},
	}

	committeeBits := primitives.NewAttestationCommitteeBits()
	committeeBits.SetBitAt(0, true)
	a := util.HydrateAttestationElectra(&ethpb.AttestationElectra{
		AggregationBits: bitfield.Bitlist{0x07},
		CommitteeBits:   committeeBits,
	})

	require.NoError(t, r.committeeIndexBeaconAttestationSubscriber(t.Context(), a))
	assert.DeepSSZEqual(t, []ethpb.Att{a}, r.cfg.attPool.AggregatedAttestations(), "Did not save aggregated attestation")
	assert.Equal(t, 0, len(r.cfg.attPool.UnaggregatedAttestations()), "Saved aggregated attestation as unaggregated")
}

func TestCommitteeIndexBeaconAttestationSubscriber_CanSaveUnaggregatedAttestation(t *testing.T) {
	r := &Service{
		cfg: &config{
			attPool: attestations.NewPool(),
		},
	}

	a := util.HydrateAttestation(&ethpb.Attestation{
		AggregationBits: bitfield.Bitlist{0x03},
	})

	require.NoError(t, r.committeeIndexBeaconAttestationSubscriber(t.Context(), a))
	assert.DeepSSZEqual(t, []ethpb.Att{a}, r.cfg.attPool.UnaggregatedAttestations(), "Did not save unaggregated attestation")
	assert.Equal(t, 0, len(r.cfg.attPool.AggregatedAttestations()), "Saved unaggregated attestation as aggregated")
}
