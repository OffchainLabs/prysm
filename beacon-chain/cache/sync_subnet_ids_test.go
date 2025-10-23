package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestSyncSubnetIDsCache_Roundtrip(t *testing.T) {
	c := newSyncSubnetIDs()

	for i := 0; i < 20; i++ {
		pubkey := [fieldparams.BLSPubkeyLength]byte{byte(i)}
		c.AddSyncCommitteeSubnets(pubkey[:], 100, []uint64{uint64(i)}, 0)
	}

	for i := uint64(0); i < 20; i++ {
		pubkey := [fieldparams.BLSPubkeyLength]byte{byte(i)}

		idxs, _, ok, _ := c.GetSyncCommitteeSubnets(pubkey[:], 100)
		if !ok {
			t.Errorf("Couldn't find entry in cache for pubkey %#x", pubkey)
			continue
		}
		require.Equal(t, i, idxs[0])
	}
	coms := c.GetAllSubnets(100)
	assert.Equal(t, 20, len(coms))
}

func TestSyncSubnetIDsCache_ValidateCurrentEpoch(t *testing.T) {
	c := newSyncSubnetIDs()

	for i := 0; i < 20; i++ {
		pubkey := [fieldparams.BLSPubkeyLength]byte{byte(i)}
		c.AddSyncCommitteeSubnets(pubkey[:], 100, []uint64{uint64(i)}, 0)
	}

	coms := c.GetAllSubnets(50)
	assert.Equal(t, 0, len(coms))

	for i := uint64(0); i < 20; i++ {
		pubkey := [fieldparams.BLSPubkeyLength]byte{byte(i)}

		_, jEpoch, ok, _ := c.GetSyncCommitteeSubnets(pubkey[:], 100)
		if !ok {
			t.Errorf("Couldn't find entry in cache for pubkey %#x", pubkey)
			continue
		}
		require.Equal(t, true, uint64(jEpoch) >= 100-params.BeaconConfig().SyncCommitteeSubnetCount)
	}

	coms = c.GetAllSubnets(99)
	assert.Equal(t, 20, len(coms))
}

func TestSyncSubnetIDsCache_AllSubnetsFlag(t *testing.T) {
	c := newSyncSubnetIDs()

	gFlags := new(flags.GlobalFlags)
	gFlags.SubscribeToAllSubnets = true
	flags.Init(gFlags)
	defer flags.Init(new(flags.GlobalFlags))

	subnets := c.GetAllSubnets(0)

	total := params.BeaconConfig().SyncCommitteeSubnetCount
	require.Equal(t, total, uint64(len(subnets)))

	expected := make(map[uint64]struct{}, total)
	for i := uint64(0); i < total; i++ {
		expected[i] = struct{}{}
	}

	for _, subnet := range subnets {
		delete(expected, subnet)
	}

	require.Equal(t, 0, len(expected))
}
