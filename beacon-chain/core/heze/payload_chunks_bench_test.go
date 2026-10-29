package heze

import (
	"bytes"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Benchmarks for the EIP-8411 chunk pipeline, to establish where the
// per-chunk cost actually goes. Run at a given TOTAL_EXECUTION_PAYLOAD_CHUNKS
// and compare across values:
//
//	go test -run XXX -bench BenchmarkChunk -benchmem ./beacon-chain/core/heze/
//
// The point is to separate CPU cost (measured here) from network and queueing
// cost (not measurable here), because the devnet penalty scales with chunk
// count and we need to know whether CPU can account for any of it.

// benchEnvelope builds an envelope whose serialized size is near payloadKB.
func benchEnvelope(b *testing.B, payloadKB int) *ethpb.ExecutionPayloadEnvelopeHeze {
	b.Helper()
	// One large transaction dominates the size; the rest is fixed overhead.
	txs, err := enginev1.NewProgressiveTransactionList([][]byte{
		bytes.Repeat([]byte{0xaa}, 100),
		bytes.Repeat([]byte{0xbb}, payloadKB*1024),
	})
	if err != nil {
		b.Fatal(err)
	}
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

var benchSizes = []struct {
	name string
	kb   int
}{
	{"100KB", 100},
	{"400KB", 400},
}

// BenchmarkChunkBuildAll is the whole publisher-side chunking step: serialize
// the envelope, split it, hash the leaves, build the tree, and generate an
// inclusion proof for every chunk.
func BenchmarkChunkBuildAll(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name, func(b *testing.B) {
			env := benchEnvelope(b, s.kb)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := BuildPayloadChunkMessages(env, 1, [32]byte{}); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(fieldparams.TotalExecutionPayloadChunks), "chunks")
		})
	}
}

// BenchmarkChunkVerifyOne is the receiver-side proof check for a single chunk.
// Multiply by TOTAL_EXECUTION_PAYLOAD_CHUNKS for the per-envelope cost.
func BenchmarkChunkVerifyOne(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name, func(b *testing.B) {
			env := benchEnvelope(b, s.kb)
			msgs, _, err := BuildPayloadChunkMessages(env, 1, [32]byte{})
			if err != nil {
				b.Fatal(err)
			}
			c := msgs[len(msgs)/2]
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := VerifyPayloadChunkInclusionProof(c); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkChunkMarshalOne is the SSZ encode of one chunk message, which the
// publisher pays per chunk and per mesh peer relay.
func BenchmarkChunkMarshalOne(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name, func(b *testing.B) {
			env := benchEnvelope(b, s.kb)
			msgs, _, err := BuildPayloadChunkMessages(env, 1, [32]byte{})
			if err != nil {
				b.Fatal(err)
			}
			c := msgs[len(msgs)/2]
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := c.MarshalSSZ(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkChunkUnmarshalOne is the receiver-side SSZ decode of one chunk.
func BenchmarkChunkUnmarshalOne(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name, func(b *testing.B) {
			env := benchEnvelope(b, s.kb)
			msgs, _, err := BuildPayloadChunkMessages(env, 1, [32]byte{})
			if err != nil {
				b.Fatal(err)
			}
			enc, err := msgs[len(msgs)/2].MarshalSSZ()
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var c ethpb.PayloadChunk
				if err := c.UnmarshalSSZ(enc); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkChunkReconstruct is the receiver-side reassembly: concatenate every
// chunk and SSZ-decode the envelope. Paid once per envelope.
func BenchmarkChunkReconstruct(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name, func(b *testing.B) {
			env := benchEnvelope(b, s.kb)
			msgs, _, err := BuildPayloadChunkMessages(env, 1, [32]byte{})
			if err != nil {
				b.Fatal(err)
			}
			raw := make([][]byte, len(msgs))
			for i, m := range msgs {
				raw[i] = m.Chunk
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := ReconstructPayloadEnvelope(raw); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEnvelopeMarshal is the monolithic comparison point: serializing the
// whole envelope once, which is what the unchunked transport pays.
func BenchmarkEnvelopeMarshal(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name, func(b *testing.B) {
			env := benchEnvelope(b, s.kb)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := env.MarshalSSZ(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
