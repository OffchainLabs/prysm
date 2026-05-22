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
	"github.com/ethereum/go-ethereum/common"
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
	erRoot := b.ExecutionRequestsRoot()
	return &ExecutionPayloadBid{
		ParentBlockHash:       hexutil.Encode(pbh[:]),
		ParentBlockRoot:       hexutil.Encode(pbr[:]),
		BlockHash:             hexutil.Encode(bh[:]),
		PrevRandao:            hexutil.Encode(pr[:]),
		FeeRecipient:          hexutil.Encode(fr[:]),
		GasLimit:              fmt.Sprintf("%d", b.GasLimit()),
		BuilderIndex:          fmt.Sprintf("%d", b.BuilderIndex()),
		Slot:                  fmt.Sprintf("%d", b.Slot()),
		Value:                 fmt.Sprintf("%d", b.Value()),
		ExecutionPayment:      fmt.Sprintf("%d", b.ExecutionPayment()),
		BlobKzgCommitments:    blobKzgCommitments,
		ExecutionRequestsRoot: hexutil.Encode(erRoot[:]),
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

func PTCWindowFromConsensus(window []*ethpb.PTCs) []*PTCs {
	out := make([]*PTCs, len(window))
	for i, slot := range window {
		out[i] = PTCsFromConsensus(slot)
	}
	return out
}

func PTCsFromConsensus(p *ethpb.PTCs) *PTCs {
	if p == nil {
		return &PTCs{}
	}
	indices := make([]string, len(p.ValidatorIndices))
	for i, idx := range p.ValidatorIndices {
		indices[i] = fmt.Sprintf("%d", idx)
	}
	return &PTCs{ValidatorIndices: indices}
}

func (s *SignedProposerPreferences) ToConsensus() (*ethpb.SignedProposerPreferences, error) {
	if s == nil {
		return nil, server.NewDecodeError(errNilValue, "SignedProposerPreferences")
	}
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
	dependentRoot, err := bytesutil.DecodeHexWithLength(p.DependentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "DependentRoot")
	}
	slot, err := strconv.ParseUint(p.ProposalSlot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ProposalSlot")
	}
	valIdx, err := strconv.ParseUint(p.ValidatorIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ValidatorIndex")
	}
	feeRecipient, err := bytesutil.DecodeHexWithLength(p.FeeRecipient, common.AddressLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "FeeRecipient")
	}
	gasLimit, err := strconv.ParseUint(p.TargetGasLimit, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "TargetGasLimit")
	}
	return &ethpb.ProposerPreferences{
		DependentRoot:  dependentRoot,
		ProposalSlot:   primitives.Slot(slot),
		ValidatorIndex: primitives.ValidatorIndex(valIdx),
		FeeRecipient:   feeRecipient,
		TargetGasLimit: gasLimit,
	}, nil
}
