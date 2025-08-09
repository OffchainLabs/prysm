package p2p

import (
	"context"
	"testing"

	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

// Test `verifyConnectivity` function by trying to connect to google.com (successfully)
// and then by connecting to an unreachable IP and ensuring that a log is emitted
func TestVerifyConnectivity(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	hook := logTest.NewGlobal()
	cases := []struct {
		address              string
		port                 uint
		expectedConnectivity bool
		name                 string
	}{
		{"142.250.68.46", 80, true, "Dialing a reachable IP: 142.250.68.46:80"}, // google.com
		{"123.123.123.123", 19000, false, "Dialing an unreachable IP: 123.123.123.123:19000"},
	}
	for _, tc := range cases {
		t.Run(tc.name,
			func(t *testing.T) {
				verifyConnectivity(tc.address, tc.port, "tcp")
				logMessage := "IP address is not accessible"
				if tc.expectedConnectivity {
					require.LogsDoNotContain(t, hook, logMessage)
				} else {
					require.LogsContain(t, hook, logMessage)
				}
			})
	}
}

func TestSerializeENR(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	t.Run("Ok", func(t *testing.T) {
		key, err := crypto.GenerateKey()
		require.NoError(t, err)
		db, err := enode.OpenDB("")
		require.NoError(t, err)
		lNode := enode.NewLocalNode(db, key)
		record := lNode.Node().Record()
		s, err := SerializeENR(record)
		require.NoError(t, err)
		assert.NotEqual(t, "", s)
		s = "enr:" + s
		newRec, err := enode.Parse(enode.ValidSchemes, s)
		require.NoError(t, err)
		assert.Equal(t, s, newRec.String())
	})

	t.Run("Nil record", func(t *testing.T) {
		_, err := SerializeENR(nil)
		require.NotNil(t, err)
		assert.ErrorContains(t, "could not serialize nil record", err)
	})
}

func TestConvertPeerIDToNodeID(t *testing.T) {
	const (
		peerIDStr         = "16Uiu2HAmRrhnqEfybLYimCiAYer2AtZKDGamQrL1VwRCyeh2YiFc"
		expectedNodeIDStr = "eed26c5d2425ab95f57246a5dca87317c41cacee4bcafe8bbe57e5965527c290"
	)

	peerID, err := peer.Decode(peerIDStr)
	require.NoError(t, err)

	actualNodeID, err := ConvertPeerIDToNodeID(peerID)
	require.NoError(t, err)

	actualNodeIDStr := actualNodeID.String()
	require.Equal(t, expectedNodeIDStr, actualNodeIDStr)
}

func TestMetadataFromDB(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	t.Run("Metadata from DB", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)
		err := beaconDB.SaveMetadataSeqNum(t.Context(), 42)
		require.NoError(t, err)

		metaData, err := metaDataFromDB(context.Background(), beaconDB)
		require.NoError(t, err)

		assert.Equal(t, uint64(42), metaData.SequenceNumber())
	})

	t.Run("Use default sequence number (=0) as Metadata not found on DB", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)

		metaData, err := metaDataFromDB(context.Background(), beaconDB)
		require.NoError(t, err)

		assert.Equal(t, uint64(0), metaData.SequenceNumber())
	})
}
