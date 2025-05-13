package rlnc

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
	ristretto "github.com/gtank/ristretto255"
)

func TestPrepareMessage(t *testing.T) {
	numChunks := uint(10)
	chunkSize := uint(100)
	committer := newCommitter(chunkSize)
	block := make([]byte, numChunks*chunkSize*31)
	_, err := rand.Read(block)
	require.NoError(t, err)
	node, err := NewSource(committer, numChunks, block)
	require.NoError(t, err)
	message, err := node.PrepareMessage()
	require.NoError(t, err)
	require.NotNil(t, message)
	require.Equal(t, true, message.Verify(committer))

	emptyNode := NewNode(committer, numChunks)
	_, err = emptyNode.PrepareMessage()
	require.ErrorIs(t, ErrNoData, err)
}

func TestReceive(t *testing.T) {
	numChunks := uint(2)
	chunkSize := uint(100)
	committer := newCommitter(chunkSize)
	block := make([]byte, numChunks*chunkSize*31)
	_, err := rand.Read(block)
	require.NoError(t, err)
	node, err := NewSource(committer, numChunks, block)
	require.NoError(t, err)
	// Send one message
	message, err := node.PrepareMessage()
	require.NoError(t, err)
	require.NotNil(t, message)
	receiver := NewNode(committer, numChunks)
	require.NoError(t, receiver.receive(message))
	require.Equal(t, 1, len(receiver.chunks))

	// Send another message
	message, err = node.PrepareMessage()
	require.NoError(t, err)
	require.NotNil(t, message)
	require.NoError(t, receiver.receive(message))
	require.Equal(t, 2, len(receiver.chunks))

	// The third one should fail
	message, err = node.PrepareMessage()
	require.NoError(t, err)
	require.NotNil(t, message)
	require.ErrorIs(t, ErrLinearlyDependentMessage, receiver.receive(message))
	require.Equal(t, 2, len(receiver.chunks))
}

func TestDecode(t *testing.T) {
	numChunks := uint(3)
	chunkSize := uint(10)
	committer := newCommitter(chunkSize)
	block := make([]byte, numChunks*chunkSize*31)
	_, err := rand.Read(block)
	block[len(block)-1] = 0xfe // termination byte
	require.NoError(t, err)
	node, err := NewSource(committer, numChunks, block)
	require.NoError(t, err)
	emptyNode := NewNode(committer, numChunks)

	for i := 0; i < int(numChunks); i++ {
		_, err = emptyNode.decode()
		require.ErrorIs(t, ErrNoData, err)
		message, err := node.PrepareMessage()
		require.NoError(t, err)
		require.NoError(t, emptyNode.receive(message))
	}
	decoded, err := emptyNode.decode()
	require.NoError(t, err)
	require.DeepEqual(t, block[:len(block)-1], decoded)
}

func TestDecodeLarge(t *testing.T) {
	numChunks := uint(10)
	blockHex := "bc0e0401000901007b0907f0f500c1441f3332515f27b5371829016890ec25b7b6464ce1781f863a2d50bca20d13f464e4da6b4ee0f208dad1d890023d3bef4be7084148d9e709d06a727e5133e454000000a63c7a7a1e189e8b7ccb767bd808b97a6ef7e7bf445ea5b663f882102031d0f05cbdcf7cb691dd0769ca73771b0353a20a38e2e2872b1d034caf1b4d392e49d11730939b544208663c99b4eca4e6684bc0b52ad8da7d5b19399e4badd867fe0fd70a234731285c6804c2a4f56711ddb8c82c99740f207854891028af34e27e5e00000000000000006d58a19c0870ceeaa225e8edf1fcbca5ee993c26c3d8a81adc2d2903c857128d54616b6f79616b69000dfc3e010008440100150408ec04000504f079ffffffff8d0941132d8b818e3688d3fedaf35f06607329a11609c8988bc9beaba7620280eb1b638500f1b0be7851eeb718bd8c150fb7a0d93de240ec9633ef61ab0cb299fc2d7c9ebf7b257b708b2f09c880ceff9f56f57ba61b31b25aa7d837b57571ddec04000010000000f6000000dc010000c2020000e400199e00020d0b7eac010d27a201007e5000f0658c61ab8ddf9bca9e5f197b4c109ba711c47bd3ce1ee1817c34a62b0878c3d3d0743605ad2c921c1e374f17fc3e056c300e90274033765e377682d42ddf3803bd245566f7fcb05ee5152b318c7132df9e2f1909162f52d6ce5c3258ed1a4fd731560fe400000011af00030d097e96000d27a201007e5000f061823821d2cfa82d8d780ed0876c4d0c318f1b3fd4f1fd737786d0a21bdcef7c606bbad4131d8506ac553bf1abd1a187f911310bbb4a4ce878cf60b75540d5187b0a1ef67624cb1d3e39c66dbf932f1b0916d97d7f1b20ee2bf0cbc963f5e2efab9e062ee60011017e960011289e01007e5000f06192395c50082d15bc3c4e7fe88b105805813975c19a458fca626098365c255f377d9e245037c1b621750d263c565166510a43fa90e4478fb0cce3dde050db2b0f81ce2b20e8615defccab99423795f2d5edc31b2844602f913e416bd764f81abaf0062ee60091667e96009ede0011017e5000f065878aa8c6899a9ed25b66fcaee48033b6c1f16d23e1565327fcea6f9cc42b5731a46d9402a64d50314fca6e08b72d62a718c8324d00a2365190093221e51a109dadaed0f38b7e8ee402b557c7d4ba5b5775858a2190ac997dbc8bd9c336371f58520c6d58a19c6e6404f055ab4de8ffccf7b19aa6d7d4ccc4c82f091ebd5715b978681cf010a0b67a38242ae933d7238681678b4e14f1598387b70d3beaf7c956e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b42100003100fe0100fe0100fe0100d601007e7401516e0880c3c9110b05010c7b66986701091cfc010000c0702734010c5e01008c114188951f5052ea700e7c99d32aabb6c5ba121b04d8d1ec722bb7d9d58306ccfc010000fe"
	block, err := hex.DecodeString(blockHex)
	require.NoError(t, err)
	chunkSize := ((uint(len(block))+numChunks-1)/numChunks + 30) / 31 * 31
	committer := newCommitter(chunkSize)
	node, err := NewSource(committer, numChunks, block)
	require.NoError(t, err)
	emptyNode := NewNode(committer, numChunks)

	for i := 0; i < int(numChunks); i++ {
		_, err = emptyNode.decode()
		require.ErrorIs(t, ErrNoData, err)
		message, err := node.PrepareMessage()
		require.NoError(t, err)
		require.NoError(t, emptyNode.receive(message))
	}
	decoded, err := emptyNode.decode()
	require.NoError(t, err)
	// Remove termination byte
	require.DeepEqual(t, block[:len(block)-1], decoded)
}

func TestOneScalar(t *testing.T) {
	scalarHex := "1f5052ea700e7c99d32aabb6c5ba121b04d8d1ec722bb7d9d58306ccfc010000"
	stringBytes := []byte{0x1f, 0x50, 0x52, 0xea, 0x70, 0xe, 0x7c, 0x99, 0xd3, 0x2a, 0xab, 0xb6, 0xc5, 0xba, 0x12, 0x1b, 0x4, 0xd8, 0xd1, 0xec, 0x72, 0x2b, 0xb7, 0xd9, 0xd5, 0x83, 0x6, 0xcc, 0xfc, 0x1, 0x0, 0x0}
	scalarBytes, err := hex.DecodeString(scalarHex)
	require.DeepEqual(t, stringBytes, scalarBytes)
	require.NoError(t, err)
	scalar := &ristretto.Scalar{}
	require.NoError(t, scalar.Decode(scalarBytes))
	obtained := scalar.Encode(nil)
	require.DeepEqual(t, scalarBytes, obtained)
}
