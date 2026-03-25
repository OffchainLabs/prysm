package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func testSigned(slot primitives.Slot, feeRecipient []byte, gasLimit uint64) *ethpb.SignedProposerPreferences {
	return &ethpb.SignedProposerPreferences{
		Message: &ethpb.ProposerPreferences{
			ProposalSlot: slot,
			FeeRecipient: feeRecipient,
			GasLimit:     gasLimit,
		},
	}
}

func TestProposerPreferencesCache_AddGetHas(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(123)
	feeRecipient := []byte{1, 2, 3, 4}

	require.Equal(t, false, c.Has(slot))
	added := c.Add(slot, testSigned(slot, feeRecipient, 42))
	require.Equal(t, true, added)
	require.Equal(t, true, c.Has(slot))

	pref, ok := c.Get(slot)
	require.Equal(t, true, ok)
	require.DeepEqual(t, feeRecipient, pref.FeeRecipient)
	require.Equal(t, uint64(42), pref.GasLimit)
}

func TestProposerPreferencesCache_AddDuplicateSlot(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(456)

	require.Equal(t, true, c.Add(slot, testSigned(slot, []byte{1}, 10)))
	require.Equal(t, false, c.Add(slot, testSigned(slot, []byte{2}, 20)))

	pref, ok := c.Get(slot)
	require.Equal(t, true, ok)
	require.DeepEqual(t, []byte{1}, pref.FeeRecipient)
	require.Equal(t, uint64(10), pref.GasLimit)
}

func TestProposerPreferencesCache_Clear(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(789)

	require.Equal(t, true, c.Add(slot, testSigned(slot, []byte{1}, 10)))
	c.Clear()

	require.Equal(t, false, c.Has(slot))
	_, ok := c.Get(slot)
	require.Equal(t, false, ok)
}

func TestProposerPreferencesCache_PruneBefore(t *testing.T) {
	c := NewProposerPreferencesCache()

	require.Equal(t, true, c.Add(10, testSigned(10, []byte{1}, 10)))
	require.Equal(t, true, c.Add(11, testSigned(11, []byte{2}, 11)))
	require.Equal(t, true, c.Add(12, testSigned(12, []byte{3}, 12)))

	c.PruneBefore(11)

	require.Equal(t, false, c.Has(10))
	require.Equal(t, true, c.Has(11))
	require.Equal(t, true, c.Has(12))
}

func TestProposerPreferencesCache_Pending(t *testing.T) {
	c := NewProposerPreferencesCache()

	c.Add(10, testSigned(10, []byte{1}, 10))
	c.Add(11, testSigned(11, []byte{2}, 11))
	c.Add(12, testSigned(12, []byte{3}, 12))

	all := c.Pending(0)
	require.Equal(t, 3, len(all))

	bySlot := c.Pending(11)
	require.Equal(t, 1, len(bySlot))
	require.Equal(t, primitives.Slot(11), bySlot[0].Message.ProposalSlot)

	empty := c.Pending(999)
	require.Equal(t, 0, len(empty))
}
