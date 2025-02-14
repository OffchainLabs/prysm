package execution

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	pb "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

func (s *Service) ReconstructPayloadEnvelope(ctx context.Context, e *pb.SignedBlindPayloadEnvelope) (*pb.SignedExecutionPayloadEnvelope, error) {
	result := make([]*pb.ExecutionPayloadBody, 0)
	if err := s.rpcClient.CallContext(ctx, &result, GetPayloadBodiesByHashV1, []common.Hash{common.Hash(e.Message.BlockHash)}); err != nil {
		return nil, err
	}
	if result == nil || len(result) == 0 {
		return nil, nil
	}
	b := result[0]

	txs := make([][]byte, len(b.Transactions))
	for i, t := range b.Transactions {
		txs[i] = t
	}
	dr, err := pb.JsonDepositRequestsToProto(b.DepositRequests)
	if err != nil {
		return nil, err
	}
	wr, err := pb.JsonWithdrawalRequestsToProto(b.WithdrawalRequests)
	if err != nil {
		return nil, err
	}
	cr, err := pb.JsonConsolidationRequestsToProto(b.ConsolidationRequests)
	if err != nil {
		return nil, err
	}
	return &pb.SignedExecutionPayloadEnvelope{
		Message: &pb.ExecutionPayloadEnvelope{
			Payload: &pb.ExecutionPayloadDeneb{
				ParentHash:    e.Message.ParentHash,
				FeeRecipient:  e.Message.FeeRecipient,
				StateRoot:     e.Message.StateRoot,
				ReceiptsRoot:  e.Message.ReceiptsRoot,
				LogsBloom:     e.Message.LogsBloom,
				PrevRandao:    e.Message.PrevRandao,
				BlockNumber:   e.Message.BlockNumber,
				GasLimit:      e.Message.GasLimit,
				GasUsed:       e.Message.GasUsed,
				Timestamp:     e.Message.Timestamp,
				ExtraData:     e.Message.ExtraData,
				BaseFeePerGas: e.Message.BaseFeePerGas,
				BlockHash:     e.Message.BlockHash,
				Transactions:  txs,
				Withdrawals:   b.Withdrawals,
				BlobGasUsed:   e.Message.BlobGasUsed,
				ExcessBlobGas: e.Message.ExcessBlobGas,
			},
			ExecutionRequests: &pb.ExecutionRequests{
				Deposits:       dr,
				Withdrawals:    wr,
				Consolidations: cr,
			},
			BuilderIndex:       e.Message.BuilderIndex,
			BeaconBlockRoot:    e.Message.BeaconBlockRoot,
			Slot:               e.Message.Slot,
			BlobKzgCommitments: e.Message.BlobKzgCommitments,
			StateRoot:          e.Message.StateRoot,
		},
		Signature: e.Signature,
	}, nil
}
