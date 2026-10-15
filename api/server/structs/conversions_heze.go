package structs

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// ToConsensus converts the JSON inclusion list to the consensus type.
func (il *InclusionList) ToConsensus() (*eth.InclusionList, error) {
	if il == nil {
		return nil, server.NewDecodeError(errNilValue, "InclusionList")
	}
	slot, err := strconv.ParseUint(il.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	validatorIndex, err := strconv.ParseUint(il.ValidatorIndex, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "ValidatorIndex")
	}
	dependentRoot, err := bytesutil.DecodeHexWithLength(il.DependentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "DependentRoot")
	}
	txs := make([][]byte, len(il.Transactions))
	for i, tx := range il.Transactions {
		txs[i], err = bytesutil.DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, server.NewDecodeError(err, fmt.Sprintf("Transactions[%d]", i))
		}
	}
	txList, err := enginev1.NewProgressiveTransactionList(txs)
	if err != nil {
		return nil, server.NewDecodeError(err, "Transactions")
	}
	return &eth.InclusionList{
		Slot:           primitives.Slot(slot),
		ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
		DependentRoot:  dependentRoot,
		Transactions:   txList,
	}, nil
}

// ToConsensus converts the JSON signed inclusion list to the consensus type.
func (s *SignedInclusionList) ToConsensus() (*eth.SignedInclusionList, error) {
	if s == nil {
		return nil, server.NewDecodeError(errNilValue, "SignedInclusionList")
	}
	msg, err := s.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	sig, err := bytesutil.DecodeHexWithLength(s.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	return &eth.SignedInclusionList{Message: msg, Signature: sig}, nil
}

// InclusionListFromConsensus converts the consensus inclusion list to the JSON type.
func InclusionListFromConsensus(il *eth.InclusionList) *InclusionList {
	if il == nil {
		return nil
	}
	return &InclusionList{
		Slot:           fmt.Sprintf("%d", il.Slot),
		ValidatorIndex: fmt.Sprintf("%d", il.ValidatorIndex),
		DependentRoot:  hexutil.Encode(il.DependentRoot),
		Transactions:   TransactionsFromConsensus(il.Transactions.Slice()),
	}
}

// SignedInclusionListFromConsensus converts the consensus signed inclusion list to the JSON type.
func SignedInclusionListFromConsensus(s *eth.SignedInclusionList) *SignedInclusionList {
	if s == nil {
		return nil
	}
	return &SignedInclusionList{
		Message:   InclusionListFromConsensus(s.Message),
		Signature: hexutil.Encode(s.Signature),
	}
}

// TransactionsFromConsensus hex encodes the transactions. It returns an empty list, not nil, for no transactions.
func TransactionsFromConsensus(txs [][]byte) []string {
	out := make([]string, len(txs))
	for i, tx := range txs {
		out[i] = hexutil.Encode(tx)
	}
	return out
}
