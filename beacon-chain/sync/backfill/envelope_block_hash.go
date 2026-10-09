package backfill

import (
	"math/big"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/pkg/errors"
)

// executionBlockHashGloas rebuilds the execution header from the envelope's payload contents
// and returns its keccak hash, mirroring geth's ExecutableDataToBlockNoHash. Equality with
// bid.block_hash proves the peer served the exact payload the bid committed to, without any
// EL involvement.
func executionBlockHashGloas(p *enginev1.ExecutionPayloadGloas, requests *enginev1.ExecutionRequestsGloas, parentBeaconBlockRoot [32]byte) ([32]byte, error) {
	if p == nil {
		return [32]byte{}, errors.New("nil execution payload")
	}
	txs := make(gethtypes.Transactions, 0, len(p.Transactions))
	for i, raw := range p.Transactions {
		tx := &gethtypes.Transaction{}
		if err := tx.UnmarshalBinary(raw); err != nil {
			return [32]byte{}, errors.Wrapf(err, "decode transaction %d", i)
		}
		txs = append(txs, tx)
	}
	withdrawals := make(gethtypes.Withdrawals, 0, len(p.Withdrawals))
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
	requestsHash := gethtypes.CalcRequestsHash(encodedRequests)
	// The EIP-7928 header commitment is keccak over the raw committed access list encoding;
	// decoding is deliberately skipped, differing bytes fail the hash comparison either way.
	balHash := crypto.Keccak256Hash(p.BlockAccessList)
	withdrawalsRoot := gethtypes.DeriveSha(withdrawals, trie.NewStackTrie(nil))
	pbr := common.Hash(parentBeaconBlockRoot)
	slotNumber := uint64(p.SlotNumber)
	blobGasUsed := p.BlobGasUsed
	excessBlobGas := p.ExcessBlobGas
	header := &gethtypes.Header{
		ParentHash:          common.BytesToHash(p.ParentHash),
		UncleHash:           gethtypes.EmptyUncleHash,
		Coinbase:            common.BytesToAddress(p.FeeRecipient),
		Root:                common.BytesToHash(p.StateRoot),
		TxHash:              gethtypes.DeriveSha(txs, trie.NewStackTrie(nil)),
		ReceiptHash:         common.BytesToHash(p.ReceiptsRoot),
		Bloom:               gethtypes.BytesToBloom(p.LogsBloom),
		Difficulty:          common.Big0,
		Number:              new(big.Int).SetUint64(p.BlockNumber),
		GasLimit:            p.GasLimit,
		GasUsed:             p.GasUsed,
		Time:                p.Timestamp,
		Extra:               p.ExtraData,
		MixDigest:           common.BytesToHash(p.PrevRandao),
		BaseFee:             bytesutil.LittleEndianBytesToBigInt(p.BaseFeePerGas),
		WithdrawalsHash:     &withdrawalsRoot,
		BlobGasUsed:         &blobGasUsed,
		ExcessBlobGas:       &excessBlobGas,
		ParentBeaconRoot:    &pbr,
		RequestsHash:        &requestsHash,
		BlockAccessListHash: &balHash,
		SlotNumber:          &slotNumber,
	}
	return [32]byte(header.Hash()), nil
}
