//go:build force_hashtree

package htr

// forceHashtree makes Hash and HashChunks use the hashtree library regardless of the
// EnableHashtree feature flag. `make test hash=hashtree` sets it to test hashtree built from
// source.
const forceHashtree = true
