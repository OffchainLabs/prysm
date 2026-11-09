package backfill

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/ethereum/go-ethereum/core/types/bal"
)

// balRLPBytes encodes a block access list the way a builder commits it in a payload.
func balRLPBytes(l bal.BlockAccessList) []byte {
	var buf bytes.Buffer
	if err := l.EncodeRLP(&buf); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func testGloasPayload(slot primitives.Slot, blockHash, parentHash []byte) *enginev1.ExecutionPayloadGloas {
	return &enginev1.ExecutionPayloadGloas{
		ParentHash:      parentHash,
		FeeRecipient:    make([]byte, 20),
		StateRoot:       make([]byte, 32),
		ReceiptsRoot:    make([]byte, 32),
		LogsBloom:       make([]byte, 256),
		PrevRandao:      make([]byte, 32),
		BlockNumber:     uint64(slot),
		BaseFeePerGas:   make([]byte, 32),
		BlockHash:       blockHash,
		BlockAccessList: balRLPBytes(bal.BlockAccessList{}),
		SlotNumber:      slot,
	}
}
