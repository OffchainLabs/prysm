package state_native

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
)

func Test_InitializeFromProtoEpbs(t *testing.T) {
	st := random.BeaconState(t)

	// Cache initial values to check against after initialization.
	latestBlockHash := st.LatestBlockHash
	latestFullSlot := st.LatestFullSlot
	header := st.LatestExecutionPayloadHeader
	lastWithdrawalsRoot := st.LastWithdrawalsRoot

	s, err := InitializeFromProtoEpbs(st)
	require.NoError(t, err)

	// Assert that initial values match those in the new state.
	gotLatestBlockHash := s.LatestBlockHash()
	require.DeepEqual(t, latestBlockHash, gotLatestBlockHash)
	gotLatestFullSlot := s.LatestFullSlot()
	require.Equal(t, latestFullSlot, gotLatestFullSlot)
	gotHeader := s.ExecutionPayloadHeader()
	require.DeepEqual(t, header, gotHeader)
	gotLastWithdrawalsRoot := s.LastWithdrawalsRoot()
	require.DeepEqual(t, lastWithdrawalsRoot, gotLastWithdrawalsRoot)
}

func Test_CopyEpbs(t *testing.T) {
	st := random.BeaconState(t)
	s, err := InitializeFromProtoUnsafeEpbs(st)
	require.NoError(t, err)

	// Test shallow copy.
	sNoCopy := s
	require.DeepEqual(t, s.executionPayloadHeader, sNoCopy.executionPayloadHeader)

	// Modify a field to check if it reflects in the shallow copy.
	s.executionPayloadHeader.Slot = 100
	require.Equal(t, s.executionPayloadHeader, sNoCopy.executionPayloadHeader)

	// Copy the state
	sCopy := s.Copy()
	require.NoError(t, err)
	header := sCopy.ExecutionPayloadHeader()
	require.DeepEqual(t, s.executionPayloadHeader, header)

	// Modify the original to check if the copied state is independent.
	s.executionPayloadHeader.Slot = 200
	require.DeepNotEqual(t, s.executionPayloadHeader, header)
}

func Test_HashTreeRootEpbs(t *testing.T) {
	st := random.BeaconState(t)
	s, err := InitializeFromProtoUnsafeEpbs(st)
	require.NoError(t, err)

	_, err = s.HashTreeRoot(context.Background())
	require.NoError(t, err)
}
