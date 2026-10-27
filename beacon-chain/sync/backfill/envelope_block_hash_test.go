package backfill

import (
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/holiman/uint256"
)

// handRolledHeaderHash rebuilds the execution header field by field, independently of the
// payload-to-block conversion that production delegates to geth, so a divergence in either
// path fails the comparison. The block access list commitment is keccak over the raw committed
// bytes, deliberately skipping the decode that production performs.
func handRolledHeaderHash(t *testing.T, p *enginev1.ExecutionPayloadGloas, requests *enginev1.ExecutionRequestsGloas, parentBeaconBlockRoot [32]byte) common.Hash {
	t.Helper()
	raw := p.Transactions.Slice()
	txs := make(gethtypes.Transactions, 0, len(raw))
	for _, r := range raw {
		tx := &gethtypes.Transaction{}
		require.NoError(t, tx.UnmarshalBinary(r))
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
	require.NoError(t, err)
	encodedRequests := make([][]byte, len(flat))
	for i := range flat {
		encodedRequests[i] = flat[i]
	}
	requestsHash := gethtypes.CalcRequestsHash(encodedRequests)
	balHash := gethcrypto.Keccak256Hash(p.BlockAccessList)
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
	return header.Hash()
}

func TestExecutionBlockHashGloasMatchesHandRolledHeader(t *testing.T) {
	key, err := gethcrypto.GenerateKey()
	require.NoError(t, err)
	chainID := big.NewInt(11155111)
	signer := gethtypes.LatestSignerForChainID(chainID)
	to := common.Address{0xaa}
	dynTx, err := gethtypes.SignNewTx(key, signer, &gethtypes.DynamicFeeTx{
		ChainID: chainID, Nonce: 7, GasTipCap: big.NewInt(2), GasFeeCap: big.NewInt(1_000_000_000),
		Gas: 21000, To: &to, Value: big.NewInt(1),
	})
	require.NoError(t, err)
	dynTxBytes, err := dynTx.MarshalBinary()
	require.NoError(t, err)

	// A blob transaction exercises the versioned hashes the payload-to-block conversion checks.
	blobTx, err := gethtypes.SignNewTx(key, signer, &gethtypes.BlobTx{
		ChainID: uint256.NewInt(11155111), Nonce: 8, GasTipCap: uint256.NewInt(2), GasFeeCap: uint256.NewInt(1_000_000_000),
		Gas: 21000, To: to, BlobFeeCap: uint256.NewInt(1), BlobHashes: []common.Hash{{0x01, 0xbb}},
	})
	require.NoError(t, err)
	blobTxBytes, err := blobTx.MarshalBinary()
	require.NoError(t, err)

	baseFee := big.NewInt(123_456_789)
	baseFeeLE := bytesutil.PadTo(bytesutil.ReverseByteOrder(baseFee.Bytes()), 32)

	t.Run("content-rich payload with a blob transaction", func(t *testing.T) {
		p := &enginev1.ExecutionPayloadGloas{
			ParentHash:      bytesutil.PadTo([]byte("parent-el-hash"), 32),
			FeeRecipient:    bytesutil.PadTo([]byte("fee-recipient"), 20),
			StateRoot:       bytesutil.PadTo([]byte("state-root"), 32),
			ReceiptsRoot:    bytesutil.PadTo([]byte("receipts-root"), 32),
			LogsBloom:       make([]byte, 256),
			PrevRandao:      bytesutil.PadTo([]byte("randao"), 32),
			BlockNumber:     1234,
			GasLimit:        30_000_000,
			GasUsed:         21_000,
			Timestamp:       1_700_000_000,
			ExtraData:       []byte("prysm-local-hash-test"),
			BaseFeePerGas:   baseFeeLE,
			Transactions:    util.TransactionList(t, dynTxBytes, blobTxBytes),
			Withdrawals:     []*enginev1.Withdrawal{{Index: 5, ValidatorIndex: 9, Address: bytesutil.PadTo([]byte("w-addr"), 20), Amount: 42}},
			BlobGasUsed:     131072,
			ExcessBlobGas:   262144,
			BlockAccessList: balRLPBytes(bal.BlockAccessList{{Address: common.Address{0x01}}}),
			SlotNumber:      primitives.Slot(777),
		}
		requests := &enginev1.ExecutionRequestsGloas{}
		pbr := bytesutil.ToBytes32([]byte("parent-beacon-root"))
		got, err := executionBlockHashGloas(p, requests, pbr)
		require.NoError(t, err)
		require.Equal(t, handRolledHeaderHash(t, p, requests, pbr), common.Hash(got))
	})

	t.Run("fixture-shaped empty payload", func(t *testing.T) {
		p := testGloasPayload(primitives.Slot(10), nil, bytesutil.PadTo([]byte("parent"), 32))
		requests := &enginev1.ExecutionRequestsGloas{}
		pbr := bytesutil.ToBytes32([]byte("pbr"))
		got, err := executionBlockHashGloas(p, requests, pbr)
		require.NoError(t, err)
		require.Equal(t, handRolledHeaderHash(t, p, requests, pbr), common.Hash(got))
	})

	t.Run("undecodable transaction errors", func(t *testing.T) {
		p := testGloasPayload(primitives.Slot(10), nil, bytesutil.PadTo([]byte("parent"), 32))
		p.Transactions = util.TransactionList(t, []byte{0xde, 0xad})
		_, err := executionBlockHashGloas(p, &enginev1.ExecutionRequestsGloas{}, [32]byte{})
		require.NotNil(t, err)
	})

	t.Run("undecodable block access list errors", func(t *testing.T) {
		p := testGloasPayload(primitives.Slot(10), nil, bytesutil.PadTo([]byte("parent"), 32))
		p.BlockAccessList = []byte{0xde, 0xad}
		_, err := executionBlockHashGloas(p, &enginev1.ExecutionRequestsGloas{}, [32]byte{})
		require.NotNil(t, err)
	})

	t.Run("every header input changes the hash", func(t *testing.T) {
		base := testGloasPayload(primitives.Slot(10), nil, bytesutil.PadTo([]byte("parent"), 32))
		pbr := bytesutil.ToBytes32([]byte("pbr"))
		requests := &enginev1.ExecutionRequestsGloas{}
		baseHash, err := executionBlockHashGloas(base, requests, pbr)
		require.NoError(t, err)
		mutations := map[string]func(*enginev1.ExecutionPayloadGloas){
			"gas_used":   func(p *enginev1.ExecutionPayloadGloas) { p.GasUsed++ },
			"timestamp":  func(p *enginev1.ExecutionPayloadGloas) { p.Timestamp++ },
			"extra_data": func(p *enginev1.ExecutionPayloadGloas) { p.ExtraData = []byte("x") },
			"block_access_list": func(p *enginev1.ExecutionPayloadGloas) {
				p.BlockAccessList = balRLPBytes(bal.BlockAccessList{{Address: common.Address{0x01}}})
			},
			"slot_number": func(p *enginev1.ExecutionPayloadGloas) { p.SlotNumber++ },
			"withdrawals": func(p *enginev1.ExecutionPayloadGloas) {
				p.Withdrawals = []*enginev1.Withdrawal{{Index: 1, ValidatorIndex: 1, Address: make([]byte, 20), Amount: 1}}
			},
		}
		for name, mutate := range mutations {
			p := testGloasPayload(primitives.Slot(10), nil, bytesutil.PadTo([]byte("parent"), 32))
			mutate(p)
			h, err := executionBlockHashGloas(p, requests, pbr)
			require.NoError(t, err)
			if h == baseHash {
				t.Fatalf("mutation %q did not change the computed block hash", name)
			}
		}
		h, err := executionBlockHashGloas(base, requests, bytesutil.ToBytes32([]byte("other-pbr")))
		require.NoError(t, err)
		if h == baseHash {
			t.Fatal("parent beacon block root change did not change the computed block hash")
		}
	})
}
