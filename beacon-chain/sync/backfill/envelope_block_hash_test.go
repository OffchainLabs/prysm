package backfill

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
)

// gethGroundTruthHash computes the same hash via geth's own payload-to-block conversion.
func gethGroundTruthHash(t *testing.T, p *enginev1.ExecutionPayloadGloas, requests *enginev1.ExecutionRequestsGloas, typedBal *bal.BlockAccessList, parentBeaconBlockRoot [32]byte) common.Hash {
	t.Helper()
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
	require.NoError(t, err)
	encoded := make([][]byte, len(flat))
	for i := range flat {
		encoded[i] = flat[i]
	}
	bgu := p.BlobGasUsed
	ebg := p.ExcessBlobGas
	sn := uint64(p.SlotNumber)
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
		Transactions:    p.Transactions,
		Withdrawals:     withdrawals,
		BlobGasUsed:     &bgu,
		ExcessBlobGas:   &ebg,
		SlotNumber:      &sn,
		BlockAccessList: typedBal,
	}
	beaconRoot := common.Hash(parentBeaconBlockRoot)
	blk, err := engine.ExecutableDataToBlockNoHash(ed, nil, &beaconRoot, encoded)
	require.NoError(t, err)
	return blk.Hash()
}

func TestExecutionBlockHashGloasMatchesGeth(t *testing.T) {
	key, err := gethcrypto.GenerateKey()
	require.NoError(t, err)
	chainID := big.NewInt(11155111)
	signer := gethtypes.LatestSignerForChainID(chainID)
	to := common.Address{0xaa}
	tx, err := gethtypes.SignNewTx(key, signer, &gethtypes.DynamicFeeTx{
		ChainID: chainID, Nonce: 7, GasTipCap: big.NewInt(2), GasFeeCap: big.NewInt(1_000_000_000),
		Gas: 21000, To: &to, Value: big.NewInt(1),
	})
	require.NoError(t, err)
	txBytes, err := tx.MarshalBinary()
	require.NoError(t, err)

	typedBal := &bal.BlockAccessList{}
	var balBuf bytes.Buffer
	require.NoError(t, typedBal.EncodeRLP(&balBuf))

	baseFee := big.NewInt(123_456_789)
	baseFeeLE := bytesutil.PadTo(bytesutil.ReverseByteOrder(baseFee.Bytes()), 32)

	t.Run("content-rich payload", func(t *testing.T) {
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
			Transactions:    [][]byte{txBytes},
			Withdrawals:     []*enginev1.Withdrawal{{Index: 5, ValidatorIndex: 9, Address: bytesutil.PadTo([]byte("w-addr"), 20), Amount: 42}},
			BlobGasUsed:     131072,
			ExcessBlobGas:   262144,
			BlockAccessList: balBuf.Bytes(),
			SlotNumber:      primitives.Slot(777),
		}
		requests := &enginev1.ExecutionRequestsGloas{}
		pbr := bytesutil.ToBytes32([]byte("parent-beacon-root"))
		got, err := executionBlockHashGloas(p, requests, pbr)
		require.NoError(t, err)
		require.Equal(t, gethGroundTruthHash(t, p, requests, typedBal, pbr), common.Hash(got))
	})

	t.Run("fixture-shaped empty payload", func(t *testing.T) {
		p := testGloasPayload(primitives.Slot(10), nil, bytesutil.PadTo([]byte("parent"), 32))
		p.BlockAccessList = balBuf.Bytes()
		requests := &enginev1.ExecutionRequestsGloas{}
		pbr := bytesutil.ToBytes32([]byte("pbr"))
		got, err := executionBlockHashGloas(p, requests, pbr)
		require.NoError(t, err)
		require.Equal(t, gethGroundTruthHash(t, p, requests, typedBal, pbr), common.Hash(got))
	})

	t.Run("undecodable transaction errors", func(t *testing.T) {
		p := testGloasPayload(primitives.Slot(10), nil, bytesutil.PadTo([]byte("parent"), 32))
		p.Transactions = [][]byte{{0xde, 0xad}}
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
			"gas_used":          func(p *enginev1.ExecutionPayloadGloas) { p.GasUsed++ },
			"timestamp":         func(p *enginev1.ExecutionPayloadGloas) { p.Timestamp++ },
			"extra_data":        func(p *enginev1.ExecutionPayloadGloas) { p.ExtraData = []byte("x") },
			"block_access_list": func(p *enginev1.ExecutionPayloadGloas) { p.BlockAccessList = []byte{0x01} },
			"slot_number":       func(p *enginev1.ExecutionPayloadGloas) { p.SlotNumber++ },
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
