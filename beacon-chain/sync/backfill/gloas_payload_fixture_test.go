package backfill

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
)

func testGloasPayload(slot primitives.Slot, blockHash, parentHash []byte) *enginev1.ExecutionPayloadGloas {
	return &enginev1.ExecutionPayloadGloas{
		ParentHash:    parentHash,
		FeeRecipient:  make([]byte, 20),
		StateRoot:     make([]byte, 32),
		ReceiptsRoot:  make([]byte, 32),
		LogsBloom:     make([]byte, 256),
		PrevRandao:    make([]byte, 32),
		BlockNumber:   uint64(slot),
		BaseFeePerGas: make([]byte, 32),
		BlockHash:     blockHash,
		SlotNumber:    slot,
	}
}
