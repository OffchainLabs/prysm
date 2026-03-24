package structs

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func ROExecutionPayloadBidFromConsensus(b interfaces.ROExecutionPayloadBid) *ExecutionPayloadBid {
	if b == nil {
		return nil
	}

	pbh := b.ParentBlockHash()
	pbr := b.ParentBlockRoot()
	bh := b.BlockHash()
	pr := b.PrevRandao()
	fr := b.FeeRecipient()
	commitments := b.BlobKzgCommitments()
	blobKzgCommitments := make([]string, 0, len(commitments))
	for _, commitment := range commitments {
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

func (s *SignedProposerPreferences) ToConsensus() (*ethpb.SignedProposerPreferences, error) {
	if s.Message == nil {
		return nil, server.NewDecodeError(errNilValue, "Message")
	}
	msg, err := s.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	sig, err := bytesutil.DecodeHexWithLength(s.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	return &ethpb.SignedProposerPreferences{
		Message:   msg,
		Signature: sig,
	}, nil
}

func (p *ProposerPreferences) ToConsensus() (*ethpb.ProposerPreferences, error) {
	slot, err := strconv.ParseUint(p.ProposalSlot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposalSlot")
	}
	valIdx, err := strconv.ParseUint(p.ValidatorIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ValidatorIndex")
	}
	feeRecipient, err := bytesutil.DecodeHexWithLength(p.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "FeeRecipient")
	}
	gasLimit, err := strconv.ParseUint(p.GasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "GasLimit")
	}
	return &ethpb.ProposerPreferences{
		ProposalSlot:   primitives.Slot(slot),
		ValidatorIndex: primitives.ValidatorIndex(valIdx),
		FeeRecipient:   feeRecipient,
		GasLimit:       gasLimit,
	}, nil
}

func SignedProposerPreferencesFromConsensus(sp *ethpb.SignedProposerPreferences) *SignedProposerPreferences {
	return &SignedProposerPreferences{
		Message: &ProposerPreferences{
			ProposalSlot:   fmt.Sprintf("%d", sp.Message.ProposalSlot),
			ValidatorIndex: fmt.Sprintf("%d", sp.Message.ValidatorIndex),
			FeeRecipient:   hexutil.Encode(sp.Message.FeeRecipient),
			GasLimit:       fmt.Sprintf("%d", sp.Message.GasLimit),
		},
		Signature: hexutil.Encode(sp.Signature),
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
