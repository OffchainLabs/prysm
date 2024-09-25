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

func Test_LatestExecutionPayloadHeaderEPBS(t *testing.T) {
	s := &BeaconState{version: version.EPBS}
	_, err := s.LatestExecutionPayloadHeader()
	require.ErrorContains(t, "unsupported version (epbs) for latest execution payload header", err)
}

func Test_SetLatestExecutionPayloadHeader(t *testing.T) {
	s := &BeaconState{version: version.EPBS}
	require.ErrorContains(t, "SetLatestExecutionPayloadHeader is not supported for epbs", s.SetLatestExecutionPayloadHeader(nil))
}

func Test_SetLatestExecutionPayloadHeaderEPBS(t *testing.T) {
	s := &BeaconState{version: version.EPBS, dirtyFields: make(map[types.FieldIndex]bool)}
	header := random.ExecutionPayloadHeader(t)
	require.NoError(t, s.SetLatestExecutionPayloadHeaderEPBS(header))
	require.Equal(t, true, s.dirtyFields[types.ExecutionPayloadHeader])

	got, err := s.LatestExecutionPayloadHeaderEPBS()
	require.NoError(t, err)
	require.DeepEqual(t, got, header)
}

func Test_SetLatestBlockHash(t *testing.T) {
	s := &BeaconState{version: version.EPBS, dirtyFields: make(map[types.FieldIndex]bool)}
	b := make([]byte, fieldparams.RootLength)
	_, err := rand.Read(b)
	require.NoError(t, err)
	require.NoError(t, s.SetLatestBlockHash(b))
	require.Equal(t, true, s.dirtyFields[types.LatestBlockHash])

	got, err := s.LatestBlockHash()
	require.NoError(t, err)
	require.DeepEqual(t, got, b)
}

func Test_SetLatestFullSlot(t *testing.T) {
	s := &BeaconState{version: version.EPBS, dirtyFields: make(map[types.FieldIndex]bool)}
	require.NoError(t, s.SetLatestFullSlot(primitives.Slot(3)))
	require.Equal(t, true, s.dirtyFields[types.LatestFullSlot])

	got, err := s.LatestFullSlot()
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(3), got)
}

func Test_SetLastWithdrawalsRoot(t *testing.T) {
	s := &BeaconState{version: version.EPBS, dirtyFields: make(map[types.FieldIndex]bool)}
	b := make([]byte, fieldparams.RootLength)
	_, err := rand.Read(b)
	require.NoError(t, err)
	require.NoError(t, s.SetLastWithdrawalsRoot(b))
	require.Equal(t, true, s.dirtyFields[types.LastWithdrawalsRoot])

	got, err := s.LastWithdrawalsRoot()
	require.NoError(t, err)
	require.DeepEqual(t, got, b)
}

func Test_UnsupportedStateVersionEpbs(t *testing.T) {
	s := &BeaconState{version: version.Electra}
	_, err := s.IsParentBlockFull()
	require.ErrorContains(t, "IsParentBlockFull is not supported for electra", err)
	_, err = s.LatestBlockHash()
	require.ErrorContains(t, "LatestBlockHash is not supported for electra", err)
	_, err = s.LatestFullSlot()
	require.ErrorContains(t, "LatestFullSlot is not supported for electra", err)
	_, err = s.LastWithdrawalsRoot()
	require.ErrorContains(t, "LastWithdrawalsRoot is not supported for electra", err)
	_, err = s.LatestExecutionPayloadHeaderEPBS()
	require.ErrorContains(t, "LatestExecutionPayloadHeaderEPBS is not supported for electra", err)

	require.ErrorContains(t, "LastWithdrawalsRoot is not supported for electra", s.SetLastWithdrawalsRoot(nil))
	require.ErrorContains(t, "SetLatestBlockHash is not supported for electra", s.SetLatestBlockHash(nil))
	require.ErrorContains(t, "SetLatestFullSlot is not supported for electra", s.SetLatestFullSlot(0))
	require.ErrorContains(t, "SetLatestExecutionPayloadHeaderEPBS is not supported for electra", s.SetLatestExecutionPayloadHeaderEPBS(nil))
}
