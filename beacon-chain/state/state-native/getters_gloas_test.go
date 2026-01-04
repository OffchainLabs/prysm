package state_native

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBuilderPendingPayment_ReturnsCopy(t *testing.T) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	payments := make([]*ethpb.BuilderPendingPayment, 2*slotsPerEpoch)
	target := uint64(slotsPerEpoch + 1)
	payments[target] = &ethpb.BuilderPendingPayment{Weight: 10}

	st, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		BuilderPendingPayments: payments,
	})
	require.NoError(t, err)

	payment, err := st.BuilderPendingPayment(target)
	require.NoError(t, err)

	// mutate returned copy
	payment.Weight = 99

	original, err := st.BuilderPendingPayment(target)
	require.NoError(t, err)
	require.Equal(t, uint64(10), uint64(original.Weight))
}

func TestBuilderPendingPayment_UnsupportedVersion(t *testing.T) {
	st := &BeaconState{version: version.Electra}
	_, err := st.BuilderPendingPayment(0)
	require.ErrorContains(t, "BuilderPendingPayment", err)
}
