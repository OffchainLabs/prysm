package blockchain

import (
	"testing"

	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestReportEpochMetrics_BadAttestation(t *testing.T) {
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	h, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, h.AppendCurrentEpochAttestations(&eth.PendingAttestation{InclusionDelay: 0}))
	err = reportEpochMetrics(t.Context(), s, h)
	require.ErrorContains(t, "attestation with inclusion delay of 0", err)
}

func TestReportEpochMetrics_SlashedValidatorOutOfBound(t *testing.T) {
	h, _ := util.DeterministicGenesisState(t, 1)
	v, err := h.ValidatorAtIndex(0)
	require.NoError(t, err)
	v.Slashed = true
	require.NoError(t, h.UpdateValidatorAtIndex(0, v))
	require.NoError(t, h.AppendCurrentEpochAttestations(&eth.PendingAttestation{InclusionDelay: 1, Data: util.HydrateAttestationData(&eth.AttestationData{})}))
	err = reportEpochMetrics(t.Context(), h, h)
	require.ErrorContains(t, "slot 0 out of bounds", err)
}
