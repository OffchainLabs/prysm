package p2p

import (
	"math/rand"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/config/params"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestCompareForkENR(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	logrus.SetLevel(logrus.TraceLevel)

	db, err := enode.OpenDB("")
	assert.NoError(t, err)
	_, k := createAddrAndPrivKey(t)
	clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot)
	current := params.GetNetworkScheduleEntry(clock.CurrentEpoch())
	next := params.NextNetworkScheduleEntry(clock.CurrentEpoch())
	self := enode.NewLocalNode(db, k)
	require.NoError(t, updateENR(self, current, next))

	cases := []struct {
		name      string
		expectErr error
		expectLog string
		node      func(t *testing.T) *enode.Node
	}{
		{
			name: "match",
			node: func(t *testing.T) *enode.Node {
				// Create a peer with the same current fork digest and next fork version/epoch.
				peer := enode.NewLocalNode(db, k)
				require.NoError(t, updateENR(peer, current, next))
				return peer.Node()
			},
		},
		{
			name: "current digest mismatch",
			node: func(t *testing.T) *enode.Node {
				// Create a peer with the same current fork digest and next fork version/epoch.
				peer := enode.NewLocalNode(db, k)
				testDigest := [4]byte{0xFF, 0xFF, 0xFF, 0xFF}
				require.NotEqual(t, current.ForkDigest, testDigest, "ensure test fork digest is unique")
				currentCopy := current
				currentCopy.ForkDigest = testDigest
				require.NoError(t, updateENR(peer, currentCopy, next))
				return peer.Node()
			},
			expectErr: errEth2ENRDigestMismatch,
		},
		{
			name: "next fork version mismatch",
			node: func(t *testing.T) *enode.Node {
				// Create a peer with the same current fork digest and next fork version/epoch.
				peer := enode.NewLocalNode(db, k)
				testVersion := [4]byte{0xFF, 0xFF, 0xFF, 0xFF}
				require.NotEqual(t, next.ForkVersion, testVersion, "ensure test fork version is unique")
				nextCopy := next
				nextCopy.ForkVersion = testVersion
				require.NoError(t, updateENR(peer, current, nextCopy))
				return peer.Node()
			},
			expectLog: "Peer matches fork digest but has different next fork version",
		},
		{
			name: "next fork epoch mismatch",
			node: func(t *testing.T) *enode.Node {
				// Create a peer with the same current fork digest and next fork version/epoch.
				peer := enode.NewLocalNode(db, k)
				nextCopy := next
				nextCopy.Epoch = nextCopy.Epoch + 1
				require.NoError(t, updateENR(peer, current, nextCopy))
				return peer.Node()
			},
			expectLog: "Peer matches fork digest but has different next fork epoch",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			peer := c.node(t)
			err := compareForkENR(self.Node().Record(), peer.Record())
			if c.expectErr != nil {
				require.ErrorIs(t, err, c.expectErr, "Expected error to match")
			} else {
				require.NoError(t, err, "Expected no error comparing fork ENRs")
			}
			if c.expectLog != "" {
				require.LogsContain(t, hook, c.expectLog, "Expected log message not found")
			}
		})
	}
}

func TestDiscv5_AddRetrieveForkEntryENR(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()

	clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot)
	current := params.GetNetworkScheduleEntry(clock.CurrentEpoch())
	next := params.NextNetworkScheduleEntry(clock.CurrentEpoch())
	enrForkID := &pb.ENRForkID{
		CurrentForkDigest: current.ForkDigest[:],
		NextForkVersion:   next.ForkVersion[:],
		NextForkEpoch:     next.Epoch,
	}
	enc, err := enrForkID.MarshalSSZ()
	require.NoError(t, err)
	entry := enr.WithEntry(eth2ENRKey, enc)
	temp := t.TempDir()
	randNum := rand.Int()
	tempPath := path.Join(temp, strconv.Itoa(randNum))
	require.NoError(t, os.Mkdir(tempPath, 0700))
	pkey, err := privKey(&Config{DataDir: tempPath})
	require.NoError(t, err, "Could not get private key")
	db, err := enode.OpenDB("")
	require.NoError(t, err)
	localNode := enode.NewLocalNode(db, pkey)
	localNode.Set(entry)

	resp, err := forkEntry(localNode.Node().Record())
	require.NoError(t, err)
	assert.Equal(t, hexutil.Encode(current.ForkDigest[:]), hexutil.Encode(resp.CurrentForkDigest))
	assert.Equal(t, hexutil.Encode(next.ForkVersion[:]), hexutil.Encode(resp.NextForkVersion))
	assert.Equal(t, next.Epoch, resp.NextForkEpoch, "Unexpected next fork epoch")
}

func TestAddForkEntry_NextForkVersion(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.BeaconConfig().InitializeForkSchedule()
	temp := t.TempDir()
	randNum := rand.Int()
	tempPath := path.Join(temp, strconv.Itoa(randNum))
	require.NoError(t, os.Mkdir(tempPath, 0700))
	pkey, err := privKey(&Config{DataDir: tempPath})
	require.NoError(t, err, "Could not get private key")
	db, err := enode.OpenDB("")
	require.NoError(t, err)

	localNode := enode.NewLocalNode(db, pkey)
	clock := startup.NewClock(time.Now(), params.BeaconConfig().GenesisValidatorsRoot)
	current := params.GetNetworkScheduleEntry(clock.CurrentEpoch())
	next := params.NextNetworkScheduleEntry(clock.CurrentEpoch())
	// Add the fork entry to the local node's ENR.
	require.NoError(t, updateENR(localNode, current, next))
	fe, err := forkEntry(localNode.Node().Record())
	require.NoError(t, err)
	assert.Equal(t,
		hexutil.Encode(params.BeaconConfig().AltairForkVersion), hexutil.Encode(fe.NextForkVersion),
		"Wanted Next Fork Version to be equal to genesis fork version")

	last := params.LastForkEpoch()
	current = params.GetNetworkScheduleEntry(last)
	next = params.NextNetworkScheduleEntry(last)
	require.NoError(t, updateENR(localNode, current, next))
	entry := params.NextNetworkScheduleEntry(last)
	fe, err = forkEntry(localNode.Node().Record())
	require.NoError(t, err)
	assert.Equal(t,
		hexutil.Encode(entry.ForkVersion[:]), hexutil.Encode(fe.NextForkVersion),
		"Wanted Next Fork Version to be equal to last entry in schedule")

}
