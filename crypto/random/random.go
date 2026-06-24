package random

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
)

// DeterministicRandomness creates a deterministic 32 byte array from a seed
func DeterministicRandomness(seed int64) [32]byte {
	var seedBytes [8]byte
	binary.BigEndian.PutUint64(seedBytes[:], uint64(seed))
	return sha256.Sum256(seedBytes[:])
}

// GetRandFieldElement returns a serialized random field element in big-endian
func GetRandFieldElement(seed int64) [32]byte {
	bytes := DeterministicRandomness(seed)
	var r fr.Element
	r.SetBytes(bytes[:])

	return GoKZG.SerializeScalar(r)
}

// GetRandBlob returns a random blob using the passed seed as entropy
func GetRandBlob(seed int64) GoKZG.Blob {
	var blob GoKZG.Blob
	bytesPerBlob := GoKZG.ScalarsPerBlob * GoKZG.SerializedScalarSize
	for i := 0; i < bytesPerBlob; i += GoKZG.SerializedScalarSize {
		fieldElementBytes := GetRandFieldElement(seed + int64(i))
		copy(blob[i:i+GoKZG.SerializedScalarSize], fieldElementBytes[:])
	}
	return blob
}
