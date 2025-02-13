package execution

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	pb "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

func (s *Service) ReconstructPayloadEnvelope(ctx context.Context, e *pb.SignedBlindPayloadEnvelope) (*pb.SignedExecutionPayloadEnvelope, error) {
	b, err := s.ExecutionBlockByHash(ctx, common.Hash(e.Message.BlockHash), true)
	if err != nil {
		return nil, err
	}
	txs := make([][]byte, len(b.Transactions))
	for i, t := range b.Transactions {
		txs[i], err = t.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}
	return &pb.SignedExecutionPayloadEnvelope{
		Message: &pb.ExecutionPayloadEnvelope{
			Payload: &pb.ExecutionPayloadDeneb{
				ParentHash:    b.ParentHash.Bytes(),
				FeeRecipient:  b.Coinbase.Bytes(),
				StateRoot:     b.Root.Bytes(),
				ReceiptsRoot:  b.ReceiptHash.Bytes(),
				LogsBloom:     b.Bloom.Bytes(),
				PrevRandao:    b.MixDigest.Bytes(),
				BlockNumber:   b.Number.Uint64(),
				GasLimit:      b.GasLimit,
				GasUsed:       b.GasUsed,
				Timestamp:     b.Time,
				ExtraData:     b.Extra,
				BaseFeePerGas: bytesutil.PadTo(bytesutil.ReverseByteOrder(b.BaseFee.Bytes()), fieldparams.RootLength),
				BlockHash:     b.Hash.Bytes(),
				Transactions:  txs,
				Withdrawals:   b.Withdrawals,
				BlobGasUsed:   *b.BlobGasUsed,
				ExcessBlobGas: *b.ExcessBlobGas,
			},
			ExecutionRequests:  e.Message.ExecutionRequests,
			BuilderIndex:       e.Message.BuilderIndex,
			BeaconBlockRoot:    e.Message.BeaconBlockRoot,
			Slot:               e.Message.Slot,
			BlobKzgCommitments: e.Message.BlobKzgCommitments,
			StateRoot:          e.Message.StateRoot,
		},
		Signature: e.Signature,
	}, nil
}
