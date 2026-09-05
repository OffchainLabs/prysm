package client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSlotReservations_reserve(t *testing.T) {
	t.Run("zero value reserves", func(t *testing.T) {
		var r slotReservations
		require.Equal(t, true, r.reserve(5))
	})

	t.Run("duplicate reservation is rejected", func(t *testing.T) {
		var r slotReservations
		require.Equal(t, true, r.reserve(5))
		require.Equal(t, false, r.reserve(5))
	})

	t.Run("distinct slots reserve independently", func(t *testing.T) {
		var r slotReservations
		require.Equal(t, true, r.reserve(5))
		require.Equal(t, true, r.reserve(6))
	})
}

func TestSlotReservations_release(t *testing.T) {
	t.Run("released slot can be reserved again", func(t *testing.T) {
		var r slotReservations
		require.Equal(t, true, r.reserve(5))
		r.release(5)
		require.Equal(t, true, r.reserve(5))
	})

	t.Run("releases multiple slots at once", func(t *testing.T) {
		var r slotReservations
		require.Equal(t, true, r.reserve(5))
		require.Equal(t, true, r.reserve(6))
		r.release(5, 6)
		require.Equal(t, 0, r.count())
	})

	t.Run("releasing an unreserved slot is a no-op", func(t *testing.T) {
		var r slotReservations
		r.release(5)
		require.Equal(t, 0, r.count())
	})
}

func TestSlotReservations_prune(t *testing.T) {
	t.Run("drops reservations before epochStart and keeps the rest", func(t *testing.T) {
		var r slotReservations
		require.Equal(t, true, r.reserve(9))
		require.Equal(t, true, r.reserve(10))
		require.Equal(t, true, r.reserve(11))
		r.prune(false, 10)
		require.Equal(t, 2, r.count())
		require.Equal(t, true, r.reserve(9))
		require.Equal(t, false, r.reserve(10))
		require.Equal(t, false, r.reserve(11))
	})

	t.Run("force clears everything", func(t *testing.T) {
		var r slotReservations
		require.Equal(t, true, r.reserve(10))
		require.Equal(t, true, r.reserve(11))
		r.prune(true, 0)
		require.Equal(t, 0, r.count())
		require.Equal(t, true, r.reserve(11))
	})

	t.Run("zero value prunes without panicking", func(t *testing.T) {
		var r slotReservations
		r.prune(false, 10)
		require.Equal(t, 0, r.count())
	})
}

func TestSlotReservations_count(t *testing.T) {
	t.Run("zero value counts zero", func(t *testing.T) {
		var r slotReservations
		require.Equal(t, 0, r.count())
	})

	t.Run("counts reserved slots", func(t *testing.T) {
		var r slotReservations
		require.Equal(t, true, r.reserve(1))
		require.Equal(t, true, r.reserve(2))
		require.Equal(t, 2, r.count())
	})
}
