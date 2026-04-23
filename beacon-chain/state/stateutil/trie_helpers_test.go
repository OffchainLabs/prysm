package stateutil_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

func TestMerkleizeTrieLeaves_BadHashLayer(t *testing.T) {
	hashLayer := make([][32]byte, 12)
	layers := make([][][32]byte, 20)
	_, _, err := stateutil.MerkleizeTrieLeaves(layers, hashLayer)
	assert.ErrorContains(t, "hash layer is a non power of 2", err)
}

