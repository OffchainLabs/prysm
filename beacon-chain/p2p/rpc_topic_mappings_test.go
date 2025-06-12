package p2p

import (
	"fmt"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestVerifyRPCMappings(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	assert.NoError(t, VerifyTopicMapping(RPCStatusTopicV1, &pb.Status{}), "Failed to verify status rpc topic")
	assert.NotNil(t, VerifyTopicMapping(RPCStatusTopicV1, new([]byte)), "Incorrect message type verified for status rpc topic")

	assert.NoError(t, VerifyTopicMapping(RPCMetaDataTopicV1, new(interface{})), "Failed to verify metadata rpc topic")
	assert.NotNil(t, VerifyTopicMapping(RPCStatusTopicV1, new([]byte)), "Incorrect message type verified for metadata rpc topic")

	assert.NoError(t, VerifyTopicMapping(RPCBlocksByRootTopicV1, new(types.BeaconBlockByRootsReq)), "Failed to verify blocks by root rpc topic")
}

func TestTopicDeconstructor(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	tt := []struct {
		name          string
		topic         string
		expectedError string
		output        []string
	}{
		{
			name:          "invalid topic",
			topic:         "/sjdksfks/dusidsdsd/ssz",
			expectedError: "unable to find a valid protocol prefix for /sjdksfks/dusidsdsd/ssz",
			output:        []string{"", "", ""},
		},
		{
			name:          "valid status topic",
			topic:         protocolPrefix + StatusMessageName + SchemaVersionV1,
			expectedError: "",
			output:        []string{protocolPrefix, StatusMessageName, SchemaVersionV1},
		},
		{
			name:          "malformed status topic",
			topic:         protocolPrefix + "/statis" + SchemaVersionV1,
			expectedError: "unable to find a valid message for /eth2/beacon_chain/req/statis/1",
			output:        []string{""},
		},
		{
			name:          "valid beacon block by range topic",
			topic:         protocolPrefix + BeaconBlocksByRangeMessageName + SchemaVersionV1 + "/ssz_snappy",
			expectedError: "",
			output:        []string{protocolPrefix, BeaconBlocksByRangeMessageName, SchemaVersionV1},
		},
		{
			name:          "beacon block by range topic with malformed version",
			topic:         protocolPrefix + BeaconBlocksByRangeMessageName + "/v" + "/ssz_snappy",
			expectedError: "unable to find a valid schema version for /eth2/beacon_chain/req/beacon_blocks_by_range/v/ssz_snappy",
			output:        []string{""},
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			protocolPref, message, version, err := TopicDeconstructor(test.topic)
			if test.expectedError != "" {
				require.NotNil(t, err)
				assert.Equal(t, test.expectedError, err.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.output[0], protocolPref)
				assert.Equal(t, test.output[1], message)
				assert.Equal(t, test.output[2], version)
			}
		})
	}
}

func TestTopicFromMessage_CorrectType(t *testing.T) {
	const (
		genesisEpoch    = primitives.Epoch(0)
		altairForkEpoch = primitives.Epoch(100)
		fuluForkEpoch   = primitives.Epoch(200)
	)

	params.SetupTestConfigCleanup(t)
	bCfg := params.BeaconConfig().Copy()

	bCfg.AltairForkEpoch = altairForkEpoch
	bCfg.ForkVersionSchedule[bytesutil.ToBytes4(bCfg.AltairForkVersion)] = altairForkEpoch

	bCfg.FuluForkEpoch = fuluForkEpoch
	bCfg.ForkVersionSchedule[bytesutil.ToBytes4(bCfg.FuluForkVersion)] = fuluForkEpoch

	params.OverrideBeaconConfig(bCfg)

	t.Run("garbage message", func(t *testing.T) {
		// Garbage Message
		const badMsg = "wljdjska"
		_, err := TopicFromMessage(badMsg, genesisEpoch)
		require.ErrorContains(t, fmt.Sprintf("%s: %s", invalidRPCMessageType, badMsg), err)
	})

	t.Run("before altair fork", func(t *testing.T) {
		for m := range messageMapping {
			topic, err := TopicFromMessage(m, genesisEpoch)
			require.NoError(t, err)

			require.Equal(t, true, strings.Contains(topic, SchemaVersionV1))
			_, _, version, err := TopicDeconstructor(topic)
			require.NoError(t, err)
			require.Equal(t, SchemaVersionV1, version)
		}
	})

	t.Run("after altair fork but before fulu fork", func(t *testing.T) {
		// Not modified in altair fork.
		topic, err := TopicFromMessage(GoodbyeMessageName, altairForkEpoch)
		require.NoError(t, err)
		require.Equal(t, "/eth2/beacon_chain/req/goodbye/1", topic)

		// Modified in altair fork.
		topic, err = TopicFromMessage(MetadataMessageName, altairForkEpoch)
		require.NoError(t, err)
		require.Equal(t, "/eth2/beacon_chain/req/metadata/2", topic)
	})

	t.Run("after fulu fork", func(t *testing.T) {
		// Not modified in any fork.
		topic, err := TopicFromMessage(GoodbyeMessageName, fuluForkEpoch)
		require.NoError(t, err)
		require.Equal(t, "/eth2/beacon_chain/req/goodbye/1", topic)

		// Modified in altair fork.
		topic, err = TopicFromMessage(BeaconBlocksByRangeMessageName, fuluForkEpoch)
		require.NoError(t, err)
		require.Equal(t, "/eth2/beacon_chain/req/beacon_blocks_by_range/2", topic)

		// Modified in fulu fork.
		topic, err = TopicFromMessage(StatusMessageName, fuluForkEpoch)
		require.NoError(t, err)
		require.Equal(t, "/eth2/beacon_chain/req/status/2", topic)

		// Modified both in altair and fulu fork.
		topic, err = TopicFromMessage(MetadataMessageName, fuluForkEpoch)
		require.NoError(t, err)
		require.Equal(t, "/eth2/beacon_chain/req/metadata/3", topic)
	})
}
