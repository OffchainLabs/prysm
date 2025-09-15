package options

// BlobsOption is a functional option for configuring blob retrieval
type BlobsOption func(*BlobsConfig)

// BlobsConfig holds configuration for blob retrieval
type BlobsConfig struct {
	Indices         []int
	VersionedHashes [][]byte
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
