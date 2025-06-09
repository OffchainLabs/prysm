package eth

import (
	fuzz "github.com/google/gofuzz"
)

// Fuzz makes sure that a fuzzed beacon state follows the following rules:
//   - Items of block roots, state roots and randao mixes have correct lengths. This is to satisfy multi-value slice constructors.
func (b *BeaconState) Fuzz(c fuzz.Continue) {
	c.FuzzNoCustom(b)
	fuzzFields(b.BlockRoots, b.StateRoots, b.RandaoMixes, c)
}

// Fuzz makes sure that a fuzzed beacon state follows the following rules:
//   - Items of block roots, state roots and randao mixes have correct lengths. This is to satisfy multi-value slice constructors.
func (b *BeaconStateAltair) Fuzz(c fuzz.Continue) {
	c.FuzzNoCustom(b)
	fuzzFields(b.BlockRoots, b.StateRoots, b.RandaoMixes, c)
}

// Fuzz makes sure that a fuzzed beacon state follows the following rules:
//   - Items of block roots, state roots and randao mixes have correct lengths. This is to satisfy multi-value slice constructors.
func (b *BeaconStateBellatrix) Fuzz(c fuzz.Continue) {
	c.FuzzNoCustom(b)
	fuzzFields(b.BlockRoots, b.StateRoots, b.RandaoMixes, c)
}

// Fuzz makes sure that a fuzzed beacon state follows the following rules:
//   - Items of block roots, state roots and randao mixes have correct lengths. This is to satisfy multi-value slice constructors.
func (b *BeaconStateCapella) Fuzz(c fuzz.Continue) {
	c.FuzzNoCustom(b)
	fuzzFields(b.BlockRoots, b.StateRoots, b.RandaoMixes, c)
}

// Fuzz makes sure that a fuzzed beacon state follows the following rules:
//   - Items of block roots, state roots and randao mixes have correct lengths. This is to satisfy multi-value slice constructors.
func (b *BeaconStateDeneb) Fuzz(c fuzz.Continue) {
	c.FuzzNoCustom(b)
	fuzzFields(b.BlockRoots, b.StateRoots, b.RandaoMixes, c)
}

// Fuzz makes sure that a fuzzed beacon state follows the following rules:
//   - Items of block roots, state roots and randao mixes have correct lengths. This is to satisfy multi-value slice constructors.
func (b *BeaconStateElectra) Fuzz(c fuzz.Continue) {
	c.FuzzNoCustom(b)
	fuzzFields(b.BlockRoots, b.StateRoots, b.RandaoMixes, c)
}

func fuzzFields(blockRoots [][]byte, stateRoots [][]byte, randaoMixes [][]byte, c fuzz.Continue) {
	for i := 0; i < len(blockRoots); i++ {
		// We have to fuzz an array because fuzzing a slice can change its length.
		var arr [32]byte
		c.Fuzz(&arr)
		blockRoots[i] = arr[:]
	}
	for i := 0; i < len(stateRoots); i++ {
		// We have to fuzz an array because fuzzing a slice can change its length.
		var arr [32]byte
		c.Fuzz(&arr)
		stateRoots[i] = arr[:]
	}
	for i := 0; i < len(randaoMixes); i++ {
		// We have to fuzz an array because fuzzing a slice can change its length.
		var arr [32]byte
		c.Fuzz(&arr)
		randaoMixes[i] = arr[:]
	}
}
