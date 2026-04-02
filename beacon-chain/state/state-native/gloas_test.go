package state_native

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestBuildersVal(t *testing.T) {
	st := &BeaconState{}

	require.Nil(t, st.buildersVal())

	st.builders = []*ethpb.Builder{
		{Pubkey: []byte{0x01}, ExecutionAddress: []byte{0x02}, Balance: 3},
		nil,
	}

	got := st.buildersVal()
	require.Len(t, got, 2)
	require.Nil(t, got[1])
	require.Equal(t, st.builders[0], got[0])
	require.NotSame(t, st.builders[0], got[0])
}

func TestPayloadExpectedWithdrawalsVal(t *testing.T) {
	st := &BeaconState{}

	require.Nil(t, st.payloadExpectedWithdrawalsVal())

	st.payloadExpectedWithdrawals = []*enginev1.Withdrawal{
		{Index: 1, ValidatorIndex: 2, Address: []byte{0x03}, Amount: 4},
		nil,
	}

	got := st.payloadExpectedWithdrawalsVal()
	require.Len(t, got, 2)
	require.Nil(t, got[1])
	require.Equal(t, st.payloadExpectedWithdrawals[0], got[0])
	require.NotSame(t, st.payloadExpectedWithdrawals[0], got[0])
}

func TestPTCWindowVal(t *testing.T) {
	st := &BeaconState{}

	require.Nil(t, st.ptcWindowVal())

	st.ptcWindow = []*ethpb.PTCs{
		{ValidatorIndices: []primitives.ValidatorIndex{1, 2}},
		nil,
	}

	got := st.ptcWindowVal()
	require.Len(t, got, 2)
	require.Nil(t, got[1])
	require.Equal(t, st.ptcWindow[0], got[0])
	require.NotSame(t, st.ptcWindow[0], got[0])

	got[0].ValidatorIndices[0] = 99
	require.Equal(t, uint64(1), st.ptcWindow[0].ValidatorIndices[0])
}
