package p2p_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/golang/snappy"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
)

func TestMsgID_HashesCorrectly(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	clock := startup.NewClock(time.Now(), bytesutil.ToBytes32([]byte{'A'}))
	d := params.ForkDigest(clock.CurrentEpoch())
	tpc := fmt.Sprintf(p2p.BlockSubnetTopicFormat, d)
	invalidSnappy := [32]byte{'J', 'U', 'N', 'K'}
	pMsg := &pubsubpb.Message{Data: invalidSnappy[:], Topic: &tpc}
	hashedData := hash.Hash(append(params.BeaconConfig().MessageDomainInvalidSnappy[:], pMsg.Data...))
	msgID := string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(pMsg), "Got incorrect msg id")

	validObj := [32]byte{'v', 'a', 'l', 'i', 'd'}
	enc := snappy.Encode(nil, validObj[:])
	nMsg := &pubsubpb.Message{Data: enc, Topic: &tpc}
	hashedData = hash.Hash(append(params.BeaconConfig().MessageDomainValidSnappy[:], validObj[:]...))
	msgID = string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(nMsg), "Got incorrect msg id")
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
	assert.Equal(t, msgID, p2p.MsgID(pMsg), "Got incorrect msg id")

	validObj := [32]byte{'v', 'a', 'l', 'i', 'd'}
	enc := snappy.Encode(nil, validObj[:])
	nMsg := &pubsubpb.Message{Data: enc, Topic: &tpc}
	// Create object to hash
	combinedObj = append(params.BeaconConfig().MessageDomainValidSnappy[:], topicLenBytes...)
	combinedObj = append(combinedObj, tpc...)
	combinedObj = append(combinedObj, validObj[:]...)
	hashedData = hash.Hash(combinedObj)
	msgID = string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(nMsg), "Got incorrect msg id")
}

func TestMsgID_OverlongSnappy(t *testing.T) {
	prevMaxPayloadSize := encoder.MaxPayloadSize
	encoder.MaxPayloadSize = 1000
	t.Cleanup(func() { encoder.MaxPayloadSize = prevMaxPayloadSize })
	d, err := signing.ComputeForkDigest(params.BeaconConfig().AltairForkVersion, params.BeaconConfig().GenesisValidatorsRoot[:])
	assert.NoError(t, err)
	tpc := fmt.Sprintf(p2p.BlockSubnetTopicFormat, d)

	// Encoding every byte as its own literal is valid snappy, but longer than gossip allows here.
	data := bytes.Repeat([]byte{'x'}, 700)
	overlong := binary.AppendUvarint(nil, uint64(len(data)))
	for _, b := range data {
		overlong = append(overlong, 0x00, b)
	}
	pMsg := &pubsubpb.Message{Data: overlong, Topic: &tpc}

	// It must get the invalid snappy ID of its raw data, not the ID of the valid encoding.
	combinedObj := append(params.BeaconConfig().MessageDomainInvalidSnappy[:], bytesutil.Uint64ToBytesLittleEndian(uint64(len(tpc)))...)
	combinedObj = append(combinedObj, tpc...)
	combinedObj = append(combinedObj, overlong...)
	hashedData := hash.Hash(combinedObj)
	assert.Equal(t, string(hashedData[:20]), p2p.MsgID(pMsg), "Got incorrect msg id")
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
	assert.Equal(t, msgID, p2p.MsgID(pMsg), "Got incorrect msg id")

	validObj := [32]byte{'v', 'a', 'l', 'i', 'd'}
	enc := snappy.Encode(nil, validObj[:])
	nMsg := &pubsubpb.Message{Data: enc, Topic: &tpc}
	// Create object to hash
	combinedObj = append(params.BeaconConfig().MessageDomainValidSnappy[:], topicLenBytes...)
	combinedObj = append(combinedObj, tpc...)
	combinedObj = append(combinedObj, validObj[:]...)
	hashedData = hash.Hash(combinedObj)
	msgID = string(hashedData[:20])
	assert.Equal(t, msgID, p2p.MsgID(nMsg), "Got incorrect msg id")
}

func TestMsgID_WithNilTopic(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	msg := &pubsubpb.Message{
		Data:  make([]byte, 32),
		Topic: nil,
	}

	invalid := make([]byte, 20)
	copy(invalid, "invalid")

	res := p2p.MsgID(msg)
	assert.Equal(t, res, string(invalid))
}
