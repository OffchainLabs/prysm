package peerdas_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/kzg"
	"github.com/sirupsen/logrus"
)

func generateCommitmentAndProof(blob *kzg.Blob) (*kzg.Commitment, *kzg.Proof, error) {
	commitment, err := kzg.BlobToKZGCommitment(blob)
	if err != nil {
		return nil, nil, err
	}
	proof, err := kzg.ComputeBlobKZGProof(blob, commitment)
	if err != nil {
		return nil, nil, err
	}
	return &commitment, &proof, err
}

// Returns a random blob using the passed seed as entropy
func getRandBlob(seed int64) kzg.Blob {
	var blob kzg.Blob
	bytesPerBlob := GoKZG.ScalarsPerBlob * GoKZG.SerializedScalarSize
	for i := 0; i < bytesPerBlob; i += GoKZG.SerializedScalarSize {
		fieldElementBytes := getRandFieldElement(seed + int64(i))
		copy(blob[i:i+GoKZG.SerializedScalarSize], fieldElementBytes[:])
	}
	return blob
}

// Returns a serialized random field element in big-endian
func getRandFieldElement(seed int64) [32]byte {
	bytes := deterministicRandomness(seed)
	var r fr.Element
	r.SetBytes(bytes[:])

	return GoKZG.SerializeScalar(r)
}

func deterministicRandomness(seed int64) [32]byte {
	// Converts an int64 to a byte slice
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, seed)
	if err != nil {
		logrus.WithError(err).Error("Failed to write int64 to bytes buffer")
		return [32]byte{}
	}
	bytes := buf.Bytes()

	return sha256.Sum256(bytes)
}
