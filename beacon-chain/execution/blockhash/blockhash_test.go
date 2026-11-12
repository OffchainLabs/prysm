package blockhash

import (
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
)

type fixture struct {
	data       engine.ExecutableData
	txs        [][]byte
	blobHashes []common.Hash
	ws         []*enginev1.Withdrawal
	beaconRoot [32]byte
}

func newFixture(t *testing.T) *fixture {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	chainID := big.NewInt(1337)
	signer := types.LatestSignerForChainID(chainID)
	to := common.HexToAddress("0x1234")
	blobHash := kzg4844.CalcBlobHashV1(sha256.New(), &kzg4844.Commitment{1})
	unsigned := []types.TxData{
		&types.LegacyTx{Nonce: 0, GasPrice: big.NewInt(7), Gas: 21000, To: &to, Value: big.NewInt(1)},
		&types.DynamicFeeTx{ChainID: chainID, Nonce: 1, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(9), Gas: 21000, To: &to, Value: big.NewInt(2), Data: []byte{0xde, 0xad}},
		&types.BlobTx{ChainID: uint256.NewInt(1337), Nonce: 2, GasTipCap: uint256.NewInt(1), GasFeeCap: uint256.NewInt(9), Gas: 21000, To: to, BlobFeeCap: uint256.NewInt(3), BlobHashes: []common.Hash{blobHash}},
	}
	f := &fixture{blobHashes: []common.Hash{blobHash}, beaconRoot: [32]byte{0xbe, 0xac}}
	for _, u := range unsigned {
		tx, err := types.SignNewTx(key, signer, u)
		require.NoError(t, err)
		b, err := tx.MarshalBinary()
		require.NoError(t, err)
		f.txs = append(f.txs, b)
	}
	f.ws = []*enginev1.Withdrawal{
		{Index: 5, ValidatorIndex: 10, Address: bytesutil.PadTo([]byte{0xaa}, 20), Amount: 32_000_000_000},
		{Index: 6, ValidatorIndex: 11, Address: bytesutil.PadTo([]byte{0xbb}, 20), Amount: 1},
	}
	f.data = engine.ExecutableData{
		ParentHash:    common.Hash{1},
		FeeRecipient:  common.Address{2},
		StateRoot:     common.Hash{3},
		ReceiptsRoot:  common.Hash{4},
		LogsBloom:     make([]byte, 256),
		Random:        common.Hash{5},
		Number:        100,
		GasLimit:      30_000_000,
		GasUsed:       63_000,
		Timestamp:     1_700_000_000,
		ExtraData:     []byte("prysm"),
		BaseFeePerGas: big.NewInt(1_000_000_007),
		Transactions:  f.txs,
	}
	f.data.LogsBloom[17] = 0xff
	return f
}

func (f *fixture) gethWithdrawals() []*types.Withdrawal {
	ws := make([]*types.Withdrawal, len(f.ws))
	for i, w := range f.ws {
		ws[i] = &types.Withdrawal{Index: w.Index, Validator: uint64(w.ValidatorIndex), Address: common.BytesToAddress(w.Address), Amount: w.Amount}
	}
	return ws
}

func (f *fixture) baseFee() []byte {
	return bytesutil.PadTo(bytesutil.ReverseByteOrder(f.data.BaseFeePerGas.Bytes()), 32)
}

func (f *fixture) gethHash(t *testing.T, versionedHashes []common.Hash, beaconRoot *common.Hash, requests [][]byte) common.Hash {
	block, err := engine.ExecutableDataToBlockNoHash(f.data, versionedHashes, beaconRoot, requests)
	require.NoError(t, err)
	return block.Hash()
}

func (f *fixture) bellatrix() *enginev1.ExecutionPayload {
	d := f.data
	return &enginev1.ExecutionPayload{
		ParentHash: d.ParentHash[:], FeeRecipient: d.FeeRecipient[:], StateRoot: d.StateRoot[:], ReceiptsRoot: d.ReceiptsRoot[:],
		LogsBloom: d.LogsBloom, PrevRandao: d.Random[:], BlockNumber: d.Number, GasLimit: d.GasLimit, GasUsed: d.GasUsed,
		Timestamp: d.Timestamp, ExtraData: d.ExtraData, BaseFeePerGas: f.baseFee(), Transactions: f.txs,
	}
}

func (f *fixture) capella() *enginev1.ExecutionPayloadCapella {
	b := f.bellatrix()
	return &enginev1.ExecutionPayloadCapella{
		ParentHash: b.ParentHash, FeeRecipient: b.FeeRecipient, StateRoot: b.StateRoot, ReceiptsRoot: b.ReceiptsRoot,
		LogsBloom: b.LogsBloom, PrevRandao: b.PrevRandao, BlockNumber: b.BlockNumber, GasLimit: b.GasLimit, GasUsed: b.GasUsed,
		Timestamp: b.Timestamp, ExtraData: b.ExtraData, BaseFeePerGas: b.BaseFeePerGas, Transactions: b.Transactions, Withdrawals: f.ws,
	}
}

func (f *fixture) deneb() *enginev1.ExecutionPayloadDeneb {
	c := f.capella()
	return &enginev1.ExecutionPayloadDeneb{
		ParentHash: c.ParentHash, FeeRecipient: c.FeeRecipient, StateRoot: c.StateRoot, ReceiptsRoot: c.ReceiptsRoot,
		LogsBloom: c.LogsBloom, PrevRandao: c.PrevRandao, BlockNumber: c.BlockNumber, GasLimit: c.GasLimit, GasUsed: c.GasUsed,
		Timestamp: c.Timestamp, ExtraData: c.ExtraData, BaseFeePerGas: c.BaseFeePerGas, Transactions: c.Transactions, Withdrawals: c.Withdrawals,
		BlobGasUsed: 131072, ExcessBlobGas: 262144,
	}
}

func (f *fixture) gloas(t *testing.T, balBytes []byte, slot primitives.Slot) *enginev1.ExecutionPayloadGloas {
	d := f.deneb()
	txs, err := enginev1.NewProgressiveTransactionList(f.txs)
	require.NoError(t, err)
	return &enginev1.ExecutionPayloadGloas{
		ParentHash: d.ParentHash, FeeRecipient: d.FeeRecipient, StateRoot: d.StateRoot, ReceiptsRoot: d.ReceiptsRoot,
		LogsBloom: d.LogsBloom, PrevRandao: d.PrevRandao, BlockNumber: d.BlockNumber, GasLimit: d.GasLimit, GasUsed: d.GasUsed,
		Timestamp: d.Timestamp, ExtraData: d.ExtraData, BaseFeePerGas: d.BaseFeePerGas, Transactions: txs, Withdrawals: d.Withdrawals,
		BlobGasUsed: d.BlobGasUsed, ExcessBlobGas: d.ExcessBlobGas, BlockAccessList: balBytes, SlotNumber: slot,
	}
}

func flatten(t *testing.T, r enginev1.ExecutionRequester) [][]byte {
	flat, err := r.FlattenRequests()
	require.NoError(t, err)
	raw := make([][]byte, len(flat))
	for i, b := range flat {
		raw[i] = b
	}
	return raw
}

func compute(t *testing.T, wrap func() (interfaces.ExecutionData, error), root *[32]byte, requests enginev1.ExecutionRequester) common.Hash {
	payload, err := wrap()
	require.NoError(t, err)
	h, err := Compute(payload, root, requests)
	require.NoError(t, err)
	return h
}

func TestCompute_MatchesGeth(t *testing.T) {
	t.Run("bellatrix", func(t *testing.T) {
		f := newFixture(t)
		want := f.gethHash(t, f.blobHashes, nil, nil)
		got := compute(t, func() (interfaces.ExecutionData, error) { return blocks.WrappedExecutionPayload(f.bellatrix()) }, nil, nil)
		require.Equal(t, want, got)
	})
	t.Run("capella", func(t *testing.T) {
		f := newFixture(t)
		f.data.Withdrawals = f.gethWithdrawals()
		want := f.gethHash(t, f.blobHashes, nil, nil)
		got := compute(t, func() (interfaces.ExecutionData, error) { return blocks.WrappedExecutionPayloadCapella(f.capella()) }, nil, nil)
		require.Equal(t, want, got)
	})
	t.Run("capella no withdrawals", func(t *testing.T) {
		f := newFixture(t)
		f.ws = []*enginev1.Withdrawal{}
		f.data.Withdrawals = []*types.Withdrawal{}
		want := f.gethHash(t, f.blobHashes, nil, nil)
		got := compute(t, func() (interfaces.ExecutionData, error) { return blocks.WrappedExecutionPayloadCapella(f.capella()) }, nil, nil)
		require.Equal(t, want, got)
	})
	t.Run("deneb", func(t *testing.T) {
		f := newFixture(t)
		p := f.deneb()
		f.data.Withdrawals = f.gethWithdrawals()
		f.data.BlobGasUsed, f.data.ExcessBlobGas = &p.BlobGasUsed, &p.ExcessBlobGas
		root := common.Hash(f.beaconRoot)
		want := f.gethHash(t, f.blobHashes, &root, nil)
		got := compute(t, func() (interfaces.ExecutionData, error) { return blocks.WrappedExecutionPayloadDeneb(p) }, &f.beaconRoot, nil)
		require.Equal(t, want, got)
	})
	t.Run("electra", func(t *testing.T) {
		f := newFixture(t)
		p := f.deneb()
		f.data.Withdrawals = f.gethWithdrawals()
		f.data.BlobGasUsed, f.data.ExcessBlobGas = &p.BlobGasUsed, &p.ExcessBlobGas
		requests := &enginev1.ExecutionRequests{
			Deposits:       []*enginev1.DepositRequest{{Pubkey: make([]byte, 48), WithdrawalCredentials: make([]byte, 32), Amount: 32, Signature: make([]byte, 96), Index: 3}},
			Consolidations: []*enginev1.ConsolidationRequest{{SourceAddress: make([]byte, 20), SourcePubkey: make([]byte, 48), TargetPubkey: make([]byte, 48)}},
		}
		root := common.Hash(f.beaconRoot)
		want := f.gethHash(t, f.blobHashes, &root, flatten(t, requests))
		got := compute(t, func() (interfaces.ExecutionData, error) { return blocks.WrappedExecutionPayloadDeneb(p) }, &f.beaconRoot, requests)
		require.Equal(t, want, got)
	})
	t.Run("electra empty requests", func(t *testing.T) {
		f := newFixture(t)
		p := f.deneb()
		f.data.Withdrawals = f.gethWithdrawals()
		f.data.BlobGasUsed, f.data.ExcessBlobGas = &p.BlobGasUsed, &p.ExcessBlobGas
		requests := &enginev1.ExecutionRequests{}
		root := common.Hash(f.beaconRoot)
		want := f.gethHash(t, f.blobHashes, &root, flatten(t, requests))
		got := compute(t, func() (interfaces.ExecutionData, error) { return blocks.WrappedExecutionPayloadDeneb(p) }, &f.beaconRoot, requests)
		require.Equal(t, want, got)
	})
	t.Run("gloas", func(t *testing.T) {
		f := newFixture(t)
		cbal := bal.NewConstructionBlockAccessList()
		cbal.AccountRead(common.Address{9})
		cbal.StorageWrite(1, common.Address{7}, common.Hash{1}, common.Hash{2})
		cbal.BalanceChange(2, common.Address{8}, uint256.NewInt(1000))
		accessList := cbal.ToEncodingObj()
		balBytes, err := rlp.EncodeToBytes(accessList)
		require.NoError(t, err)
		slot := primitives.Slot(4242)
		p := f.gloas(t, balBytes, slot)
		slotNumber := uint64(slot)
		f.data.Withdrawals = f.gethWithdrawals()
		f.data.BlobGasUsed, f.data.ExcessBlobGas = &p.BlobGasUsed, &p.ExcessBlobGas
		f.data.SlotNumber = &slotNumber
		f.data.BlockAccessList = accessList
		requests := &enginev1.ExecutionRequestsGloas{
			Withdrawals:     []*enginev1.WithdrawalRequest{{SourceAddress: make([]byte, 20), ValidatorPubkey: make([]byte, 48), Amount: 5}},
			BuilderDeposits: []*enginev1.BuilderDepositRequest{{Pubkey: make([]byte, 48), WithdrawalCredentials: make([]byte, 32), Amount: 7, Signature: make([]byte, 96)}},
		}
		root := common.Hash(f.beaconRoot)
		want := f.gethHash(t, f.blobHashes, &root, flatten(t, requests))
		got := compute(t, func() (interfaces.ExecutionData, error) { return blocks.WrappedExecutionPayloadGloas(p) }, &f.beaconRoot, requests)
		require.Equal(t, want, got)
	})
}

func TestVerify(t *testing.T) {
	f := newFixture(t)
	p := f.deneb()
	f.data.Withdrawals = f.gethWithdrawals()
	f.data.BlobGasUsed, f.data.ExcessBlobGas = &p.BlobGasUsed, &p.ExcessBlobGas
	root := common.Hash(f.beaconRoot)
	want := f.gethHash(t, f.blobHashes, &root, nil)
	p.BlockHash = want[:]
	payload, err := blocks.WrappedExecutionPayloadDeneb(p)
	require.NoError(t, err)
	require.NoError(t, Verify(payload, &f.beaconRoot, nil))

	p.GasUsed++
	payload, err = blocks.WrappedExecutionPayloadDeneb(p)
	require.NoError(t, err)
	require.ErrorIs(t, Verify(payload, &f.beaconRoot, nil), ErrMismatch)
}

func TestCompute_Errors(t *testing.T) {
	f := newFixture(t)
	payload, err := blocks.WrappedExecutionPayloadDeneb(f.deneb())
	require.NoError(t, err)
	_, err = Compute(payload, nil, nil)
	require.ErrorIs(t, err, errMissingBeaconRoot)

	gp, err := blocks.WrappedExecutionPayloadGloas(f.gloas(t, nil, 1))
	require.NoError(t, err)
	_, err = Compute(gp, &f.beaconRoot, nil)
	require.ErrorIs(t, err, errMissingRequests)

	b := f.bellatrix()
	b.Transactions = append(b.Transactions, []byte{})
	payload, err = blocks.WrappedExecutionPayload(b)
	require.NoError(t, err)
	_, err = Compute(payload, nil, nil)
	require.ErrorIs(t, err, errEmptyTransaction)

	header, err := blocks.WrappedExecutionPayloadHeader(&enginev1.ExecutionPayloadHeader{})
	require.NoError(t, err)
	_, err = Compute(header, nil, nil)
	require.ErrorIs(t, err, errUnsupportedType)
}
