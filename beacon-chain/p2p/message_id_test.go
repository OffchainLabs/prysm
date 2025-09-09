package p2p_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/crypto/hash"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/golang/snappy"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
)

func TestMsgID_HashesCorrectly(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	clock := startup.NewClock(time.Now(), bytesutil.ToBytes32([]byte{'A'}))
	valRoot := clock.GenesisValidatorsRoot()
	d := params.ForkDigest(clock.CurrentEpoch())
	tpc := fmt.Sprintf(p2p.BlockSubnetTopicFormat, d)
	invalidSnappy := [32]byte{'J', 'U', 'N', 'K'}
	pMsg := &pubsubpb.Message{Data: invalidSnappy[:], Topic: &tpc}
	hashedData := hash.Hash(append(params.BeaconConfig().MessageDomainInvalidSnappy[:], pMsg.Data...))
	msgID := string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(valRoot[:], pMsg), "Got incorrect msg id")

	validObj := [32]byte{'v', 'a', 'l', 'i', 'd'}
	enc := snappy.Encode(nil, validObj[:])
	nMsg := &pubsubpb.Message{Data: enc, Topic: &tpc}
	hashedData = hash.Hash(append(params.BeaconConfig().MessageDomainValidSnappy[:], validObj[:]...))
	msgID = string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(valRoot[:], nMsg), "Got incorrect msg id")
}

func TestMessageIDFunction_HashesCorrectlyAltair(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	d, err := signing.ComputeForkDigest(params.BeaconConfig().AltairForkVersion, params.BeaconConfig().GenesisValidatorsRoot[:])
	assert.NoError(t, err)
	tpc := fmt.Sprintf(p2p.BlockSubnetTopicFormat, d)
	topicLen := uint64(len(tpc))
	topicLenBytes := bytesutil.Uint64ToBytesLittleEndian(topicLen)
	invalidSnappy := [32]byte{'J', 'U', 'N', 'K'}
	pMsg := &pubsubpb.Message{Data: invalidSnappy[:], Topic: &tpc}
	// Create object to hash
	combinedObj := append(params.BeaconConfig().MessageDomainInvalidSnappy[:], topicLenBytes...)
	combinedObj = append(combinedObj, tpc...)
	combinedObj = append(combinedObj, pMsg.Data...)
	hashedData := hash.Hash(combinedObj)
	msgID := string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(params.BeaconConfig().GenesisValidatorsRoot[:], pMsg), "Got incorrect msg id")

	validObj := [32]byte{'v', 'a', 'l', 'i', 'd'}
	enc := snappy.Encode(nil, validObj[:])
	nMsg := &pubsubpb.Message{Data: enc, Topic: &tpc}
	// Create object to hash
	combinedObj = append(params.BeaconConfig().MessageDomainValidSnappy[:], topicLenBytes...)
	combinedObj = append(combinedObj, tpc...)
	combinedObj = append(combinedObj, validObj[:]...)
	hashedData = hash.Hash(combinedObj)
	msgID = string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(params.BeaconConfig().GenesisValidatorsRoot[:], nMsg), "Got incorrect msg id")
}

func TestMessageIDFunction_HashesCorrectlyBellatrix(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	d, err := signing.ComputeForkDigest(params.BeaconConfig().BellatrixForkVersion, params.BeaconConfig().GenesisValidatorsRoot[:])
	assert.NoError(t, err)
	tpc := fmt.Sprintf(p2p.BlockSubnetTopicFormat, d)
	topicLen := uint64(len(tpc))
	topicLenBytes := bytesutil.Uint64ToBytesLittleEndian(topicLen)
	invalidSnappy := [32]byte{'J', 'U', 'N', 'K'}
	pMsg := &pubsubpb.Message{Data: invalidSnappy[:], Topic: &tpc}
	// Create object to hash
	combinedObj := append(params.BeaconConfig().MessageDomainInvalidSnappy[:], topicLenBytes...)
	combinedObj = append(combinedObj, tpc...)
	combinedObj = append(combinedObj, pMsg.Data...)
	hashedData := hash.Hash(combinedObj)
	msgID := string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(params.BeaconConfig().GenesisValidatorsRoot[:], pMsg), "Got incorrect msg id")

	validObj := [32]byte{'v', 'a', 'l', 'i', 'd'}
	enc := snappy.Encode(nil, validObj[:])
	nMsg := &pubsubpb.Message{Data: enc, Topic: &tpc}
	// Create object to hash
	combinedObj = append(params.BeaconConfig().MessageDomainValidSnappy[:], topicLenBytes...)
	combinedObj = append(combinedObj, tpc...)
	combinedObj = append(combinedObj, validObj[:]...)
	hashedData = hash.Hash(combinedObj)
	msgID = string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(params.BeaconConfig().GenesisValidatorsRoot[:], nMsg), "Got incorrect msg id")
}

func TestMsgID_WithNilTopic(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	msg := &pubsubpb.Message{
		Data:  make([]byte, 32),
		Topic: nil,
	}

	invalid := make([]byte, 20)
	copy(invalid, "invalid")

	res := p2p.MsgID([]byte{0x01}, msg)
	assert.Equal(t, res, string(invalid))
}
