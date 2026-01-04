package state_native

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSetBuilderPendingPayment_CopiesValue(t *testing.T) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	payments := make([]*ethpb.BuilderPendingPayment, 2*slotsPerEpoch)
	st, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		BuilderPendingPayments: payments,
	})
	require.NoError(t, err)

	payment := &ethpb.BuilderPendingPayment{Weight: 123}
	target := uint64(1)
	require.NoError(t, st.SetBuilderPendingPayment(target, payment))

	payment.Weight = 999

	got, err := st.BuilderPendingPayment(target)
	require.NoError(t, err)
	require.Equal(t, uint64(123), uint64(got.Weight))
}

func TestSetBuilderPendingPayment_UnsupportedVersion(t *testing.T) {
	st := &BeaconState{version: version.Electra}
	err := st.SetBuilderPendingPayment(0, &ethpb.BuilderPendingPayment{})
	require.ErrorContains(t, "SetBuilderPendingPayment", err)
}
