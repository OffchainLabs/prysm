package kzg

const (
	bytesPerFieldElement = 32
	fieldElementsPerBlob = 4096
	fieldElementsPerCell = 64

	// BytesPerBlob is the number of bytes in a single blob.
	BytesPerBlob = fieldElementsPerBlob * bytesPerFieldElement

	// BytesPerCell is the number of bytes in a single cell.
	BytesPerCell = fieldElementsPerCell * bytesPerFieldElement

	// BytesPerCommitment is the number of bytes in a KZG commitment.
	BytesPerCommitment = 48

	// BytesPerProof is the number of bytes in a KZG proof.
	BytesPerProof = 48
)

// Blob represents a serialized chunk of data.
type Blob [BytesPerBlob]byte

// Cell represents a chunk of an encoded Blob.
type Cell [BytesPerCell]byte

// Commitment represent a KZG commitment to a Blob.
type Commitment [BytesPerCommitment]byte

// Proof represents a KZG proof that attests to the validity of a Blob or parts of it.
type Proof [BytesPerProof]byte

// Bytes48 is a 48-byte array, used by the batch verification APIs for values
// that may be either a commitment or a proof.
type Bytes48 [48]byte
