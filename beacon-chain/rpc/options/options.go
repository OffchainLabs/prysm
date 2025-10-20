package options

// BlobsOption is a functional option for configuring blob retrieval
type BlobsOption func(*BlobsConfig)

// BlobsConfig holds configuration for blob retrieval
type BlobsConfig struct {
	Indices         []int
	VersionedHashes [][]byte
	SkipKzgProofs   bool // Post-Fulu optimization: skip expensive KZG proof computations when proofs aren't needed
}

// WithIndices specifies blob indices to retrieve
func WithIndices(indices []int) BlobsOption {
	return func(c *BlobsConfig) {
		c.Indices = indices
	}
}

// WithVersionedHashes specifies versioned hashes to retrieve blobs by
func WithVersionedHashes(hashes [][]byte) BlobsOption {
	return func(c *BlobsConfig) {
		c.VersionedHashes = hashes
	}
}

// WithSkipKzgProofs indicates that KZG proofs should not be computed (post-Fulu optimization)
// Use this when the caller only needs blob data and not the full sidecar with proofs
func WithSkipKzgProofs() BlobsOption {
	return func(c *BlobsConfig) {
		c.SkipKzgProofs = true
	}
}
