package ssz

import (
	"bytes"
	"encoding/binary"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/pkg/errors"
)

// Uint64Root computes the HashTreeRoot Merkleization of
// a simple uint64 value according to the Ethereum
// Simple Serialize specification.
func Uint64Root(val uint64) [32]byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, val)
	root := bytesutil.ToBytes32(buf)
	return root
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

// KzgCommitmentsRoot computes the HTR for a list of KZG commitments
func KzgCommitmentsRoot(commitments [][]byte) ([32]byte, error) {
	roots := make([][32]byte, len(commitments))
	for i, commitment := range commitments {
		chunks, err := PackByChunk([][]byte{commitment})
		if err != nil {
			return [32]byte{}, err
		}
		roots[i] = htr.VectorizedSha256(chunks)[0]
	}

	commitmentsRoot, err := BitwiseMerkleize(roots, uint64(len(roots)), fieldparams.MaxBlobCommitmentsPerBlock)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute merkleization")
	}

	length := make([]byte, 32)
	binary.LittleEndian.PutUint64(length[:8], uint64(len(roots)))
	return MixInLength(commitmentsRoot, length), nil
}
