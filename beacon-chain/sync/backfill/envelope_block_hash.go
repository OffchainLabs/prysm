package backfill

import (
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
)

// executionBlockHashGloas derives the execution block hash from the envelope's payload contents
// via geth's own payload-to-block conversion. Equality with bid.block_hash proves the peer served
// the exact payload the bid committed to, without any EL involvement.
func executionBlockHashGloas(p *enginev1.ExecutionPayloadGloas, requests *enginev1.ExecutionRequestsGloas, parentBeaconBlockRoot [32]byte) ([32]byte, error) {
	if p == nil {
		return [32]byte{}, errors.New("nil execution payload")
	}
	// The conversion checks blob versioned hashes against the caller's expectation. This helper
	// only computes the hash, so the expectation is the payload's own blob hashes; the bid's
	// block hash commits to the transactions, making any mismatch fail the final comparison.
	txsBytes := p.Transactions.Slice()
	txs, err := engine.DecodeTransactions(txsBytes)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "decode transactions")
	}
	versionedHashes := make([]common.Hash, 0)
	for _, tx := range txs {
		versionedHashes = append(versionedHashes, tx.BlobHashes()...)
	}
	accessList := &bal.BlockAccessList{}
	if err := rlp.DecodeBytes(p.BlockAccessList, accessList); err != nil {
		return [32]byte{}, errors.Wrap(err, "decode block access list")
	}
	withdrawals := make([]*gethtypes.Withdrawal, 0, len(p.Withdrawals))
	for _, w := range p.Withdrawals {
		withdrawals = append(withdrawals, &gethtypes.Withdrawal{
			Index:     w.Index,
			Validator: uint64(w.ValidatorIndex),
			Address:   common.BytesToAddress(w.Address),
			Amount:    w.Amount,
		})
	}
	flat, err := requests.FlattenRequests()
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "flatten execution requests")
	}
	encodedRequests := make([][]byte, len(flat))
	for i := range flat {
		encodedRequests[i] = flat[i]
	}
	pbr := common.Hash(parentBeaconBlockRoot)
	slotNumber := uint64(p.SlotNumber)
	blobGasUsed := p.BlobGasUsed
	excessBlobGas := p.ExcessBlobGas
	ed := engine.ExecutableData{
		ParentHash:      common.BytesToHash(p.ParentHash),
		FeeRecipient:    common.BytesToAddress(p.FeeRecipient),
		StateRoot:       common.BytesToHash(p.StateRoot),
		ReceiptsRoot:    common.BytesToHash(p.ReceiptsRoot),
		LogsBloom:       p.LogsBloom,
		Random:          common.BytesToHash(p.PrevRandao),
		Number:          p.BlockNumber,
		GasLimit:        p.GasLimit,
		GasUsed:         p.GasUsed,
		Timestamp:       p.Timestamp,
		ExtraData:       p.ExtraData,
		BaseFeePerGas:   bytesutil.LittleEndianBytesToBigInt(p.BaseFeePerGas),
		Transactions:    txsBytes,
		Withdrawals:     withdrawals,
		BlobGasUsed:     &blobGasUsed,
		ExcessBlobGas:   &excessBlobGas,
		SlotNumber:      &slotNumber,
		BlockAccessList: accessList,
	}
	blk, err := engine.ExecutableDataToBlockNoHash(ed, versionedHashes, &pbr, encodedRequests)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "executable data to block")
	}
	return [32]byte(blk.Hash()), nil
}
