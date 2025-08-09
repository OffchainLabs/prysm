package ssz

import (
	"bytes"
	"encoding/binary"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

type Transaction []byte

func (t Transaction) HashTreeRoot() ([32]byte, error) {
	return ByteSliceRoot(t, fieldparams.MaxBytesPerTxLength)
}

// Uint64Root computes the HashTreeRoot Merkleization of
// a simple uint64 value according to the Ethereum
// Simple Serialize specification.
func Uint64Root(val uint64) [32]byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, val)
	root := bytesutil.ToBytes32(buf)
	return root
}

// ForkRoot computes the HashTreeRoot Merkleization of Fork
func ForkRoot(fork *ethpb.Fork) ([32]byte, error) {
	if fork == nil {
		fieldRoots := make([][32]byte, 3)
		return BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
	}
	return fork.HashTreeRoot()
}

// CheckpointRoot computes the HashTreeRoot Merkleization of Checkpoint
func CheckpointRoot(checkpoint *ethpb.Checkpoint) ([32]byte, error) {
	if checkpoint == nil {
		fieldRoots := make([][32]byte, 2)
		return BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
	}
	return checkpoint.HashTreeRoot()
}

// ByteArrayRootWithLimit computes the HashTreeRoot Merkleization of
// a list of [32]byte roots according to the Ethereum Simple Serialize
// specification.
func ByteArrayRootWithLimit(roots [][]byte, limit uint64) ([32]byte, error) {
	newRoots := make([][32]byte, len(roots))
	for i, r := range roots {
		copy(newRoots[i][:], r)
	}
	result, err := BitwiseMerkleize(newRoots, uint64(len(newRoots)), limit)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute byte array merkleization")
	}
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, uint64(len(newRoots))); err != nil {
		return [32]byte{}, errors.Wrap(err, "could not marshal byte array length")
	}
	// We need to mix in the length of the slice.
	output := make([]byte, 32)
	copy(output, buf.Bytes())
	mixedLen := MixInLength(result, output)
	return mixedLen, nil
}

// SlashingsRoot computes the HashTreeRoot Merkleization of
// a list of uint64 slashing values according to the Ethereum
// Simple Serialize specification.
func SlashingsRoot(slashings []uint64) ([32]byte, error) {
	slashingMarshaling := make([][]byte, fieldparams.SlashingsLength)
	for i := 0; i < len(slashings) && i < len(slashingMarshaling); i++ {
		slashBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(slashBuf, slashings[i])
		slashingMarshaling[i] = slashBuf
	}
	slashingChunks, err := PackByChunk(slashingMarshaling)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not pack slashings into chunks")
	}
	return BitwiseMerkleize(slashingChunks, uint64(len(slashingChunks)), uint64(len(slashingChunks)))
}

// TransactionsRoot computes the HTR for the Transactions' property of the ExecutionPayload
func TransactionsRoot(txs [][]byte) ([32]byte, error) {
	transactions := make([]Transaction, len(txs))
	for i, tx := range txs {
		transactions[i] = Transaction(tx)
	}
	return SliceRoot(transactions, fieldparams.MaxTxsPerPayloadLength)
}

// WithdrawalSliceRoot computes the HTR of a slice of withdrawals.
// The limit parameter is used as input to the bitwise merkleization algorithm.
func WithdrawalSliceRoot(withdrawals []*enginev1.Withdrawal, limit uint64) ([32]byte, error) {
	return SliceRoot(withdrawals, limit)
}

// DepositRequestsSliceRoot computes the HTR of a slice of deposit requests.
// The limit parameter is used as input to the bitwise merkleization algorithm.
func DepositRequestsSliceRoot(depositRequests []*enginev1.DepositRequest, limit uint64) ([32]byte, error) {
	return SliceRoot(depositRequests, limit)
}

// WithdrawalRequestsSliceRoot computes the HTR of a slice of withdrawal requests from the EL.
// The limit parameter is used as input to the bitwise merkleization algorithm.
func WithdrawalRequestsSliceRoot(withdrawalRequests []*enginev1.WithdrawalRequest, limit uint64) ([32]byte, error) {
	return SliceRoot(withdrawalRequests, limit)
}

// ByteSliceRoot is a helper func to merkleize an arbitrary List[Byte, N]
// this func runs Chunkify + MerkleizeVector
// max length is dividable by 32 ( root length )
func ByteSliceRoot(slice []byte, maxLength uint64) ([32]byte, error) {
	chunkedRoots, err := PackByChunk([][]byte{slice})
	if err != nil {
		return [32]byte{}, err
	}
	maxRootLength := (maxLength + 31) / 32 // nearest number divisible by root length (32)
	bytesRoot, err := BitwiseMerkleize(chunkedRoots, uint64(len(chunkedRoots)), maxRootLength)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute merkleization")
	}
	bytesRootBuf := new(bytes.Buffer)
	if err := binary.Write(bytesRootBuf, binary.LittleEndian, uint64(len(slice))); err != nil {
		return [32]byte{}, errors.Wrap(err, "could not marshal length")
	}
	bytesRootBufRoot := make([]byte, 32)
	copy(bytesRootBufRoot, bytesRootBuf.Bytes())
	return MixInLength(bytesRoot, bytesRootBufRoot), nil
}

func withdrawalRoot(w *enginev1.Withdrawal) ([32]byte, error) {
	if w == nil {
		fieldRoots := make([][32]byte, 4)
		return BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
	}
	return w.HashTreeRoot()
}
