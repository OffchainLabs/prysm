package kv

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestStore_HotStateSnapshot_DefaultBehavior(t *testing.T) {
	db := setupDB(t)
	r1 := [32]byte{'A'}
	r2 := [32]byte{'B'}
	st1, _ := createState(t, 12, version.Phase0)
	st2, _ := createState(t, 13, version.Gloas)

	// save
	require.NoError(t, db.SaveHotStateSnapshot(t.Context(), st1, r1))
	require.NoError(t, db.SaveHotStateSnapshot(t.Context(), st2, r2))

	// has
	require.Equal(t, true, db.HasHotStateSnapshot(t.Context(), r1))
	require.Equal(t, true, db.HasHotStateSnapshot(t.Context(), r2))

	// read
	gotSt1, err := db.HotStateSnapshot(t.Context(), r1)
	require.NoError(t, err)
	gotSt2, err := db.HotStateSnapshot(t.Context(), r2)
	require.NoError(t, err)
	require.Equal(t, st1.Slot(), gotSt1.Slot())
	require.Equal(t, st2.Slot(), gotSt2.Slot())

	// clear
	require.NoError(t, db.ClearHotStateSnapshots(t.Context()))
	require.Equal(t, false, db.HasHotStateSnapshot(t.Context(), r1))
	require.Equal(t, false, db.HasHotStateSnapshot(t.Context(), r2))
	_, err = db.HotStateSnapshot(t.Context(), r1)
	require.ErrorIs(t, err, ErrNotFoundState)
}

func TestStore_StateUsingStateDiff_PreferHotStateSnapshots(t *testing.T) {
	resetCft := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCft()
	setDefaultStateDiffExponents()

	db := setupDB(t)
	r := [32]byte{'A'}
	st, _ := createState(t, 12, version.Phase0)
	require.NoError(t, db.SaveHotStateSnapshot(t.Context(), st, r))

	require.Equal(t, true, db.HasState(t.Context(), r))

	gotSt, err := db.State(t.Context(), r)
	require.NoError(t, err)
	require.NotNil(t, gotSt)
	require.Equal(t, st.Slot(), gotSt.Slot())
}
