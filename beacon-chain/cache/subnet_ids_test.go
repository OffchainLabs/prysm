package cache

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
)

func TestSubnetIDsCache_RoundTrip(t *testing.T) {
	c := newSubnetIDs()
	slot := primitives.Slot(100)
	committeeIDs := c.GetAggregatorSubnetIDs(slot)
	assert.Equal(t, 0, len(committeeIDs), "Empty cache returned an object")

	c.AddAggregatorSubnetID(slot, 1)
	res := c.GetAggregatorSubnetIDs(slot)
	assert.DeepEqual(t, []uint64{1}, res)

	c.AddAggregatorSubnetID(slot, 2)
	res = c.GetAggregatorSubnetIDs(slot)
	assert.DeepEqual(t, []uint64{1, 2}, res)

	c.AddAggregatorSubnetID(slot, 3)
	res = c.GetAggregatorSubnetIDs(slot)
	assert.DeepEqual(t, []uint64{1, 2, 3}, res)

	committeeIDs = c.GetAttesterSubnetIDs(slot)
	assert.Equal(t, 0, len(committeeIDs), "Empty cache returned an object")

	c.AddAttesterSubnetID(slot, 11)
	res = c.GetAttesterSubnetIDs(slot)
	assert.DeepEqual(t, []uint64{11}, res)

	c.AddAttesterSubnetID(slot, 22)
	res = c.GetAttesterSubnetIDs(slot)
	assert.DeepEqual(t, []uint64{11, 22}, res)

	c.AddAttesterSubnetID(slot, 33)
	res = c.GetAttesterSubnetIDs(slot)
	assert.DeepEqual(t, []uint64{11, 22, 33}, res)
}

func TestSubnetIDsCache_PersistentCommitteeRoundtrip(t *testing.T) {
	c := newSubnetIDs()

	c.AddPersistentCommittee([]uint64{0, 1, 2, 7, 8}, 0)

	coms := c.GetAllSubnets()
	assert.Equal(t, 5, len(coms))
}
