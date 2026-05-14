package cache

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestSubscribedValidatorsCache_AddHas(t *testing.T) {
	c := NewSubscribedValidatorsCache(time.Minute, time.Minute)
	require.Equal(t, false, c.Has(7))
	c.Add(7)
	require.Equal(t, true, c.Has(7))
}

func TestSubscribedValidatorsCache_Validating(t *testing.T) {
	c := NewSubscribedValidatorsCache(time.Minute, time.Minute)
	require.Equal(t, false, c.Validating())
	c.Add(7)
	require.Equal(t, true, c.Validating())
}

func TestSubscribedValidatorsCache_Indices(t *testing.T) {
	c := NewSubscribedValidatorsCache(time.Minute, time.Minute)
	require.Equal(t, 0, len(c.Indices()))
	c.Add(3)
	c.Add(11)
	c.Add(2026)
	got := c.Indices()
	require.Equal(t, 3, len(got))
	require.Equal(t, true, got[primitives.ValidatorIndex(3)])
	require.Equal(t, true, got[primitives.ValidatorIndex(11)])
	require.Equal(t, true, got[primitives.ValidatorIndex(2026)])
}

func TestSubscribedValidatorsCache_AddIdempotent(t *testing.T) {
	c := NewSubscribedValidatorsCache(time.Minute, time.Minute)
	c.Add(7)
	c.Add(7)
	require.Equal(t, 1, len(c.Indices()))
}

func TestSubscribedValidatorsCache_TTLExpiry(t *testing.T) {
	c := NewSubscribedValidatorsCache(10*time.Millisecond, time.Millisecond)
	c.Add(7)
	require.Equal(t, true, c.Has(7))
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, false, c.Has(7))
	require.Equal(t, false, c.Validating())
}
