package state_native

import (
	"crypto/rand"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/types"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util/random"
)

func Test_LatestExecutionPayloadHeader(t *testing.T) {
	s := &BeaconState{version: version.EPBS}
	_, err := s.LatestExecutionPayloadHeader()
	require.ErrorContains(t, "unsupported version (epbs) for latest execution payload header", err)
}

func Test_SetLatestExecutionPayloadHeader(t *testing.T) {
	s := &BeaconState{version: version.EPBS}
	require.ErrorContains(t, "SetLatestExecutionPayloadHeader is not supported for epbs", s.SetLatestExecutionPayloadHeader(nil))
}

func Test_SetExecutionPayloadHeader(t *testing.T) {
	s := &BeaconState{version: version.EPBS, dirtyFields: make(map[types.FieldIndex]bool)}
	header := random.ExecutionPayloadHeader(t)
	s.SetExecutionPayloadHeader(header)
	require.Equal(t, true, s.dirtyFields[types.ExecutionPayloadHeader])

	got := s.ExecutionPayloadHeader()
	require.DeepEqual(t, got, header)
}

func Test_SetLatestBlockHash(t *testing.T) {
	s := &BeaconState{version: version.EPBS, dirtyFields: make(map[types.FieldIndex]bool)}
	b := make([]byte, fieldparams.RootLength)
	_, err := rand.Read(b)
	require.NoError(t, err)
	s.SetLatestBlockHash(b)
	require.Equal(t, true, s.dirtyFields[types.LatestBlockHash])

	got := s.LatestBlockHash()
	require.DeepEqual(t, got, b)
}

func Test_SetLatestFullSlot(t *testing.T) {
	s := &BeaconState{version: version.EPBS, dirtyFields: make(map[types.FieldIndex]bool)}
	s.SetLatestFullSlot(3)
	require.Equal(t, true, s.dirtyFields[types.LatestFullSlot])

	got := s.LatestFullSlot()
	require.Equal(t, primitives.Slot(3), got)
}

func Test_SetLastWithdrawalsRoot(t *testing.T) {
	s := &BeaconState{version: version.EPBS, dirtyFields: make(map[types.FieldIndex]bool)}
	b := make([]byte, fieldparams.RootLength)
	_, err := rand.Read(b)
	require.NoError(t, err)
	s.SetLastWithdrawalsRoot(b)
	require.Equal(t, true, s.dirtyFields[types.LastWithdrawalsRoot])

	got := s.LastWithdrawalsRoot()
	require.DeepEqual(t, got, b)
}
