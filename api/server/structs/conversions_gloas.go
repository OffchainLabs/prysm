package structs

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func ROExecutionPayloadBidFromConsensus(b interfaces.ROExecutionPayloadBid) *ExecutionPayloadBid {
	pbh := b.ParentBlockHash()
	pbr := b.ParentBlockRoot()
	bh := b.BlockHash()
	pr := b.PrevRandao()
	fr := b.FeeRecipient()
	var blobKzgCommitments []string
	for _, commitment := range b.BlobKzgCommitments() {
		blobKzgCommitments = append(blobKzgCommitments, hexutil.Encode(commitment))
	}
	return &ExecutionPayloadBid{
		ParentBlockHash:    hexutil.Encode(pbh[:]),
		ParentBlockRoot:    hexutil.Encode(pbr[:]),
		BlockHash:          hexutil.Encode(bh[:]),
		PrevRandao:         hexutil.Encode(pr[:]),
		FeeRecipient:       hexutil.Encode(fr[:]),
		GasLimit:           fmt.Sprintf("%d", b.GasLimit()),
		BuilderIndex:       fmt.Sprintf("%d", b.BuilderIndex()),
		Slot:               fmt.Sprintf("%d", b.Slot()),
		Value:              fmt.Sprintf("%d", b.Value()),
		ExecutionPayment:   fmt.Sprintf("%d", b.ExecutionPayment()),
		BlobKzgCommitments: blobKzgCommitments,
	}
}

func BuildersFromConsensus(builders []*ethpb.Builder) []*Builder {
	newBuilders := make([]*Builder, len(builders))
	for i, b := range builders {
		newBuilders[i] = BuilderFromConsensus(b)
	}
	return newBuilders
}

func BuilderFromConsensus(b *ethpb.Builder) *Builder {
	return &Builder{
		Pubkey:            hexutil.Encode(b.Pubkey),
		Version:           hexutil.Encode(b.Version),
		ExecutionAddress:  hexutil.Encode(b.ExecutionAddress),
		Balance:           fmt.Sprintf("%d", b.Balance),
		DepositEpoch:      fmt.Sprintf("%d", b.DepositEpoch),
		WithdrawableEpoch: fmt.Sprintf("%d", b.WithdrawableEpoch),
	}
}

func BuilderPendingPaymentsFromConsensus(payments []*ethpb.BuilderPendingPayment) []*BuilderPendingPayment {
	newPayments := make([]*BuilderPendingPayment, len(payments))
	for i, p := range payments {
		newPayments[i] = BuilderPendingPaymentFromConsensus(p)
	}
	return newPayments
}

func BuilderPendingPaymentFromConsensus(p *ethpb.BuilderPendingPayment) *BuilderPendingPayment {
	return &BuilderPendingPayment{
		Weight:     fmt.Sprintf("%d", p.Weight),
		Withdrawal: BuilderPendingWithdrawalFromConsensus(p.Withdrawal),
	}
}

func BuilderPendingWithdrawalsFromConsensus(withdrawals []*ethpb.BuilderPendingWithdrawal) []*BuilderPendingWithdrawal {
	newWithdrawals := make([]*BuilderPendingWithdrawal, len(withdrawals))
	for i, w := range withdrawals {
		newWithdrawals[i] = BuilderPendingWithdrawalFromConsensus(w)
	}
	return newWithdrawals
}

func BuilderPendingWithdrawalFromConsensus(w *ethpb.BuilderPendingWithdrawal) *BuilderPendingWithdrawal {
	return &BuilderPendingWithdrawal{
		FeeRecipient: hexutil.Encode(w.FeeRecipient),
		Amount:       fmt.Sprintf("%d", w.Amount),
		BuilderIndex: fmt.Sprintf("%d", w.BuilderIndex),
	}
}
