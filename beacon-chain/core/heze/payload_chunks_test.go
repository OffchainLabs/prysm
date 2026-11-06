package heze

import (
	"bytes"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func chunkTestEnvelope(t *testing.T) *ethpb.ExecutionPayloadEnvelopeHeze {
	t.Helper()
	txs, err := enginev1.NewProgressiveTransactionList([][]byte{bytes.Repeat([]byte{0xaa}, 100), bytes.Repeat([]byte{0xbb}, 7000)})
	require.NoError(t, err)
	return &ethpb.ExecutionPayloadEnvelopeHeze{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    bytesutil.PadTo([]byte("parent"), 32),
			FeeRecipient:  bytesutil.PadTo([]byte("fee"), 20),
			StateRoot:     bytesutil.PadTo([]byte("stateroot"), 32),
			ReceiptsRoot:  bytesutil.PadTo([]byte("receipts"), 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    bytesutil.PadTo([]byte("randao"), 32),
			BlockNumber:   100,
			GasLimit:      30000000,
			GasUsed:       21000,
			Timestamp:     1000,
			ExtraData:     []byte("extra"),
			BaseFeePerGas: bytesutil.PadTo([]byte{1}, 32),
			BlockHash:     bytesutil.PadTo([]byte("blockhash"), 32),
			Transactions:  txs,
			Withdrawals:   []*enginev1.Withdrawal{{Index: 1, ValidatorIndex: 2, Address: bytesutil.PadTo([]byte("addr"), 20), Amount: 100}},
			BlobGasUsed:   131072,
			ExcessBlobGas: 0,
			SlotNumber:    99,
		},
		ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
		BuilderIndex:          42,
		ParentBeaconBlockRoot: make([]byte, 32),
	}
}

func TestComputePayloadChunks_Shapes(t *testing.T) {
	n := fieldparams.TotalExecutionPayloadChunks
	cases := []struct {
		name string
		size int
	}{
		{"one byte", 1},
		{"n-1 bytes", n - 1},
		{"n bytes", n},
		{"n+1 bytes", n + 1},
		{"not divisible", 12345},
		{"exactly max", fieldparams.MaxPayloadChunkSize * n},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := bytes.Repeat([]byte{0x7}, tc.size)
			chunks, err := ComputePayloadChunks(data)
			require.NoError(t, err)
			require.Equal(t, n, len(chunks))
			var recomposed []byte
			for _, c := range chunks {
				recomposed = append(recomposed, c...)
			}
			require.DeepEqual(t, data, recomposed)
			// Per spec: chunk size is ceil(len/64), the final non-empty chunk may
			// be shorter and trailing chunks may be empty.
			chunkSize := (tc.size + n - 1) / n
			for i, c := range chunks {
				require.Equal(t, true, len(c) <= chunkSize, "chunk %d larger than chunk size", i)
			}
		})
	}
}

func TestComputePayloadChunks_TooLarge(t *testing.T) {
	data := make([]byte, fieldparams.MaxPayloadChunkSize*fieldparams.TotalExecutionPayloadChunks+1)
	_, err := ComputePayloadChunks(data)
	require.ErrorIs(t, err, errChunkTooLarge)
}

func TestPayloadChunkInclusionProofs(t *testing.T) {
	data := bytes.Repeat([]byte{0x42}, 100_000)
	chunks, err := ComputePayloadChunks(data)
	require.NoError(t, err)
	root, err := ComputePayloadChunksRoot(chunks)
	require.NoError(t, err)

	env := chunkTestEnvelope(t)
	envData, err := env.MarshalSSZ()
	require.NoError(t, err)
	msgs, msgsRoot, err := BuildPayloadChunkMessages(env, 7, [32]byte{1, 2, 3})
	require.NoError(t, err)
	require.Equal(t, fieldparams.TotalExecutionPayloadChunks, len(msgs))

	expectedChunks, err := ComputePayloadChunks(envData)
	require.NoError(t, err)
	expectedRoot, err := ComputePayloadChunksRoot(expectedChunks)
	require.NoError(t, err)
	require.Equal(t, expectedRoot, msgsRoot)
	require.NotEqual(t, root, msgsRoot)

	for _, m := range msgs {
		require.NoError(t, VerifyPayloadChunkInclusionProof(m), "index %d", m.Index)
	}

	t.Run("tampered chunk bytes fail", func(t *testing.T) {
		bad := &ethpb.PayloadChunk{
			PayloadChunksRoot:   msgs[3].PayloadChunksRoot,
			Index:               msgs[3].Index,
			Chunk:               append(bytes.Clone(msgs[3].Chunk), 0xff),
			ChunkInclusionProof: msgs[3].ChunkInclusionProof,
			Slot:                msgs[3].Slot,
			BeaconBlockRoot:     msgs[3].BeaconBlockRoot,
		}
		require.ErrorIs(t, VerifyPayloadChunkInclusionProof(bad), errChunkProofInvalid)
	})

	t.Run("wrong index fails", func(t *testing.T) {
		bad := &ethpb.PayloadChunk{
			PayloadChunksRoot:   msgs[3].PayloadChunksRoot,
			Index:               4,
			Chunk:               msgs[3].Chunk,
			ChunkInclusionProof: msgs[3].ChunkInclusionProof,
		}
		require.ErrorIs(t, VerifyPayloadChunkInclusionProof(bad), errChunkProofInvalid)
	})

	t.Run("index aliasing is rejected by the bound", func(t *testing.T) {
		// index + 64 has identical low CHUNK_INCLUSION_PROOF_DEPTH bits, so the
		// branch walk alone would accept it; the explicit bound must reject it.
		bad := &ethpb.PayloadChunk{
			PayloadChunksRoot:   msgs[3].PayloadChunksRoot,
			Index:               msgs[3].Index + uint64(fieldparams.TotalExecutionPayloadChunks),
			Chunk:               msgs[3].Chunk,
			ChunkInclusionProof: msgs[3].ChunkInclusionProof,
		}
		require.ErrorIs(t, VerifyPayloadChunkInclusionProof(bad), errChunkIndexTooLarge)
	})

	t.Run("truncated proof fails", func(t *testing.T) {
		bad := &ethpb.PayloadChunk{
			PayloadChunksRoot:   msgs[3].PayloadChunksRoot,
			Index:               msgs[3].Index,
			Chunk:               msgs[3].Chunk,
			ChunkInclusionProof: msgs[3].ChunkInclusionProof[:fieldparams.ChunkInclusionProofDepth-1],
		}
		require.ErrorIs(t, VerifyPayloadChunkInclusionProof(bad), errChunkBadProofLen)
	})

	t.Run("oversized chunk is rejected by the size bound", func(t *testing.T) {
		// An oversized chunk decodes: the generated UnmarshalSSZ enforces no
		// upper bound on the byte list. It also proof-verifies, because the leaf
		// is sha256 over whatever bytes arrived. Only the explicit bound stops
		// it, so build a genuinely valid single-chunk commitment over an
		// oversized payload and confirm the bound is what rejects it.
		oversized := make([]byte, fieldparams.MaxPayloadChunkSize+1)
		chunks := make([][]byte, fieldparams.TotalExecutionPayloadChunks)
		chunks[0] = oversized
		for i := 1; i < fieldparams.TotalExecutionPayloadChunks; i++ {
			chunks[i] = []byte{}
		}
		root, err := ComputePayloadChunksRoot(chunks)
		require.NoError(t, err)
		layers := payloadChunkLayers(chunks)
		proof := make([][]byte, fieldparams.ChunkInclusionProofDepth)
		idx := 0
		for d := range fieldparams.ChunkInclusionProofDepth {
			sibling := layers[d][idx^1]
			proof[d] = bytes.Clone(sibling[:])
			idx >>= 1
		}
		bad := &ethpb.PayloadChunk{
			PayloadChunksRoot:   root[:],
			Index:               0,
			Chunk:               oversized,
			ChunkInclusionProof: proof,
		}
		require.ErrorIs(t, VerifyPayloadChunkInclusionProof(bad), errChunkSizeTooLarge)

		// Without the bound this exact chunk would pass, which is what makes the
		// bound load bearing rather than redundant with the proof check.
		withinBound := &ethpb.PayloadChunk{
			PayloadChunksRoot:   bad.PayloadChunksRoot,
			Index:               bad.Index,
			Chunk:               bad.Chunk[:fieldparams.MaxPayloadChunkSize],
			ChunkInclusionProof: bad.ChunkInclusionProof,
		}
		require.ErrorIs(t, VerifyPayloadChunkInclusionProof(withinBound), errChunkProofInvalid)
	})

	t.Run("oversized chunk survives SSZ decode", func(t *testing.T) {
		// Pins the decoder gap the size bound exists to cover: PayloadChunk
		// round-trips a chunk larger than MAX_PAYLOAD_CHUNK_SIZE because only
		// MarshalSSZ checks the list bound.
		oversized := make([]byte, fieldparams.MaxPayloadChunkSize+1)
		wire := &ethpb.PayloadChunk{
			PayloadChunksRoot:   make([]byte, 32),
			Index:               0,
			Chunk:               oversized,
			ChunkInclusionProof: proofOfZeroes(),
			Slot:                7,
			BeaconBlockRoot:     make([]byte, 32),
		}
		// MarshalSSZ refuses it, so hand-assemble the wire bytes the way a
		// hostile peer would.
		wire.Chunk = oversized[:fieldparams.MaxPayloadChunkSize]
		enc, err := wire.MarshalSSZ()
		require.NoError(t, err)
		enc = append(enc, 0x00) // one byte past the bound

		decoded := &ethpb.PayloadChunk{}
		require.NoError(t, decoded.UnmarshalSSZ(enc))
		require.Equal(t, fieldparams.MaxPayloadChunkSize+1, len(decoded.Chunk))
		require.ErrorIs(t, VerifyPayloadChunkInclusionProof(decoded), errChunkSizeTooLarge)
	})
}

func proofOfZeroes() [][]byte {
	proof := make([][]byte, fieldparams.ChunkInclusionProofDepth)
	for i := range proof {
		proof[i] = make([]byte, 32)
	}
	return proof
}

func TestReconstructPayloadEnvelope_RoundTrip(t *testing.T) {
	env := chunkTestEnvelope(t)
	msgs, root, err := BuildPayloadChunkMessages(env, 7, [32]byte{})
	require.NoError(t, err)

	chunks := make([][]byte, len(msgs))
	for i, m := range msgs {
		chunks[i] = m.Chunk
	}
	got, err := ReconstructPayloadEnvelope(chunks)
	require.NoError(t, err)

	wantData, err := env.MarshalSSZ()
	require.NoError(t, err)
	gotData, err := got.MarshalSSZ()
	require.NoError(t, err)
	require.DeepEqual(t, wantData, gotData)

	gotRoot, err := PayloadChunksRootForEnvelope(got)
	require.NoError(t, err)
	require.Equal(t, root, gotRoot)
}

func TestReconstructPayloadEnvelope_Garbage(t *testing.T) {
	chunks := make([][]byte, fieldparams.TotalExecutionPayloadChunks)
	chunks[0] = bytes.Repeat([]byte{0xde}, 33)
	for i := 1; i < len(chunks); i++ {
		chunks[i] = []byte{}
	}
	_, err := ReconstructPayloadEnvelope(chunks)
	require.NotNil(t, err)
}

func TestStampPayloadChunkHints(t *testing.T) {
	env := chunkTestEnvelope(t)
	msgs, _, err := BuildPayloadChunkMessages(env, 0, [32]byte{})
	require.NoError(t, err)
	StampPayloadChunkHints(msgs, 11, [32]byte{9})
	for _, m := range msgs {
		require.Equal(t, uint64(11), uint64(m.Slot))
		require.DeepEqual(t, bytesutil.PadTo([]byte{9}, 32), m.BeaconBlockRoot)
	}
}
