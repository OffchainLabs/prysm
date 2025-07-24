package p2p

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/wrapper"
	"github.com/OffchainLabs/prysm/v6/io/file"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/metadata"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prysmaticlabs/go-bitfield"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"google.golang.org/protobuf/proto"
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

func TestMetaDataFromFile(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	tests := []struct {
		name     string
		metaData metadata.Metadata
	}{
		{
			name: "V0",
			metaData: func() metadata.Metadata {
				md := &pb.MetaDataV0{
					SeqNumber: rand.Uint64(),
					Attnets:   bitfield.NewBitvector64(),
				}
				return wrapper.WrappedMetadataV0(md)
			}(),
		},
		{
			name: "V1",
			metaData: func() metadata.Metadata {
				md := &pb.MetaDataV1{
					SeqNumber: rand.Uint64(),
					Attnets:   bitfield.NewBitvector64(),
					Syncnets:  bitfield.NewBitvector4(),
				}
				return wrapper.WrappedMetadataV1(md)
			}(),
		},
		{
			name: "V2",
			metaData: func() metadata.Metadata {
				md := &pb.MetaDataV2{
					SeqNumber:         rand.Uint64(),
					Attnets:           bitfield.NewBitvector64(),
					Syncnets:          bitfield.NewBitvector4(),
					CustodyGroupCount: rand.Uint64(),
				}
				return wrapper.WrappedMetadataV2(md)
			}(),
		},
	}

	for _, tt := range tests {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, metaDataPath)

		// Save to file
		err := saveMetaDataToFile(path, tt.metaData.Copy())
		require.NoError(t, err)

		// Load file, and compare
		mdFromFile, err := metaDataFromFile(path)
		require.NoError(t, err)
		require.DeepEqual(t, tt.metaData.Copy(), mdFromFile.Copy())
	}
}

func TestMetaDataMigrationFromProtoToSsz(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, metaDataPath)

	// Generate metadata V0 and save with proto-encoded
	seqNum := rand.Uint64()
	md := &pb.MetaDataV0{
		SeqNumber: seqNum,
		Attnets:   bitfield.NewBitvector64(),
	}
	wmd := wrapper.WrappedMetadataV0(md)
	dst, err := proto.Marshal(md)
	require.NoError(t, err)

	err = file.WriteFile(path, dst)
	require.NoError(t, err)

	migratedMd, err := migrateFromProtoToSsz(path)
	require.NoError(t, err)

	// Check if sequence number is incremented
	require.Equal(t, wmd.SequenceNumber()+1, migratedMd.SequenceNumber())
}
