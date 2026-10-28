### Ignored

- Remove the `github.com/prysmaticlabs/fastssz` dependency: SSZ-QL proof verification uses `trie.VerifyMerkleProof`, and the remaining offset/marshal helpers come from `methodical-ssz` or the standard library.
