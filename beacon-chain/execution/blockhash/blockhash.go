package blockhash

import (
	"bytes"
	"fmt"
	"math/big"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/pkg/errors"
)

var (
	ErrMismatch          = errors.New("execution payload block hash mismatch")
	errUnsupportedType   = errors.New("unsupported execution payload type")
	errEmptyTransaction  = errors.New("zero length transaction")
	errMissingBeaconRoot = errors.New("missing parent beacon block root")
	errMissingRequests   = errors.New("missing execution requests")
)

type txList interface {
	Len() int
	Get(i int) []byte
}

type rawTxs [][]byte

func (t rawTxs) Len() int         { return len(t) }
func (t rawTxs) Get(i int) []byte { return t[i] }

type derivableTxs struct{ txList }

func (t derivableTxs) EncodeIndex(i int, w *bytes.Buffer) { w.Write(t.Get(i)) }

// Compute returns the execution block hash computed from the payload contents.
func Compute(payload interfaces.ExecutionData, parentBeaconRoot *[32]byte, requests enginev1.ExecutionRequester) (common.Hash, error) {
	start := time.Now()
	defer func() { blockHashComputationTime.Observe(float64(time.Since(start).Microseconds())) }()

	header := &types.Header{
		ParentHash:  common.BytesToHash(payload.ParentHash()),
		UncleHash:   types.EmptyUncleHash,
		Coinbase:    common.BytesToAddress(payload.FeeRecipient()),
		Root:        common.BytesToHash(payload.StateRoot()),
		ReceiptHash: common.BytesToHash(payload.ReceiptsRoot()),
		Bloom:       types.BytesToBloom(payload.LogsBloom()),
		Difficulty:  common.Big0,
		Number:      new(big.Int).SetUint64(payload.BlockNumber()),
		GasLimit:    payload.GasLimit(),
		GasUsed:     payload.GasUsed(),
		Time:        payload.Timestamp(),
		Extra:       payload.ExtraData(),
		MixDigest:   common.BytesToHash(payload.PrevRandao()),
		BaseFee:     bytesutil.LittleEndianBytesToBigInt(payload.BaseFeePerGas()),
	}

	var txs txList
	switch p := payload.Proto().(type) {
	case *enginev1.ExecutionPayload:
		txs = rawTxs(p.Transactions)
	case *enginev1.ExecutionPayloadCapella:
		txs = rawTxs(p.Transactions)
		header.WithdrawalsHash = withdrawalsRoot(p.Withdrawals)
	case *enginev1.ExecutionPayloadDeneb:
		txs = rawTxs(p.Transactions)
		header.WithdrawalsHash = withdrawalsRoot(p.Withdrawals)
		if err := setDenebFields(header, p.BlobGasUsed, p.ExcessBlobGas, parentBeaconRoot); err != nil {
			return common.Hash{}, err
		}
		if requests != nil {
			if err := setRequestsHash(header, requests); err != nil {
				return common.Hash{}, err
			}
		}
	case *enginev1.ExecutionPayloadGloas:
		if p.Transactions == nil {
			return common.Hash{}, errors.New("nil transaction list")
		}
		txs = p.Transactions
		header.WithdrawalsHash = withdrawalsRoot(p.Withdrawals)
		if err := setDenebFields(header, p.BlobGasUsed, p.ExcessBlobGas, parentBeaconRoot); err != nil {
			return common.Hash{}, err
		}
		if requests == nil {
			return common.Hash{}, errMissingRequests
		}
		if err := setRequestsHash(header, requests); err != nil {
			return common.Hash{}, err
		}
		slot := uint64(p.SlotNumber)
		header.SlotNumber = &slot
		balHash := crypto.Keccak256Hash(p.BlockAccessList)
		header.BlockAccessListHash = &balHash
	default:
		return common.Hash{}, errors.Wrapf(errUnsupportedType, "%T", p)
	}

	for i := range txs.Len() {
		if len(txs.Get(i)) == 0 {
			return common.Hash{}, errors.Wrapf(errEmptyTransaction, "index %d", i)
		}
	}
	header.TxHash = types.DeriveSha(derivableTxs{txs}, trie.NewStackTrie(nil))
	return header.Hash(), nil
}

// Verify checks that the payload's block hash matches the one computed from its contents.
func Verify(payload interfaces.ExecutionData, parentBeaconRoot *[32]byte, requests enginev1.ExecutionRequester) error {
	computed, err := Compute(payload, parentBeaconRoot, requests)
	if err != nil {
		return err
	}
	claimed := common.BytesToHash(payload.BlockHash())
	if computed != claimed {
		blockHashMismatchCount.Inc()
		return fmt.Errorf("%w: computed=%#x payload=%#x", ErrMismatch, computed, claimed)
	}
	return nil
}

func setDenebFields(header *types.Header, blobGasUsed, excessBlobGas uint64, parentBeaconRoot *[32]byte) error {
	if parentBeaconRoot == nil {
		return errMissingBeaconRoot
	}
	root := common.Hash(*parentBeaconRoot)
	header.BlobGasUsed = &blobGasUsed
	header.ExcessBlobGas = &excessBlobGas
	header.ParentBeaconRoot = &root
	return nil
}

func setRequestsHash(header *types.Header, requests enginev1.ExecutionRequester) error {
	flat, err := requests.FlattenRequests()
	if err != nil {
		return errors.Wrap(err, "could not flatten execution requests")
	}
	raw := make([][]byte, len(flat))
	for i, r := range flat {
		raw[i] = r
	}
	h := types.CalcRequestsHash(raw)
	header.RequestsHash = &h
	return nil
}

func withdrawalsRoot(withdrawals []*enginev1.Withdrawal) *common.Hash {
	ws := make(types.Withdrawals, len(withdrawals))
	for i, w := range withdrawals {
		ws[i] = &types.Withdrawal{
			Index:     w.Index,
			Validator: uint64(w.ValidatorIndex),
			Address:   common.BytesToAddress(w.Address),
			Amount:    w.Amount,
		}
	}
	h := types.DeriveSha(ws, trie.NewStackTrie(nil))
	return &h
}
