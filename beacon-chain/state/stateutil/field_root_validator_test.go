package stateutil

import (
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	mathutil "github.com/OffchainLabs/prysm/v7/math"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestValidatorConstants(t *testing.T) {
	v := &ethpb.Validator{}
	refV := reflect.ValueOf(v).Elem()
	numFields := refV.NumField()
	numOfValFields := 0

	for i := range numFields {
		if strings.Contains(refV.Type().Field(i).Name, "state") ||
			strings.Contains(refV.Type().Field(i).Name, "sizeCache") ||
			strings.Contains(refV.Type().Field(i).Name, "unknownFields") {
			continue
		}
		numOfValFields++
	}
	assert.Equal(t, validatorFieldRoots, numOfValFields)
	assert.Equal(t, uint64(validatorLeaves), mathutil.PowerOf2(validatorTreeDepth))
	assert.Equal(t, true, validatorFieldRoots <= validatorLeaves && validatorFieldRoots > validatorLeaves/2,
		"The depth must be the smallest that fits all fields")

	cv := CompactValidatorFromProto(v)
	_, err := ValidatorRegistryRoot(version.Phase0, []CompactValidator{cv})
	assert.NoError(t, err)
}

func TestOptimizedValidatorRoots_MatchesPerValidatorRoot(t *testing.T) {
	r := rand.New(rand.NewSource(3))

	// GOMAXPROCS sets the group count: one group, uneven last group, more procs than trees.
	for _, procs := range []int{1, 7, 64} {
		for _, n := range []int{1, 5, 1003, 4096} {
			t.Run(fmt.Sprintf("procs=%d/n=%d", procs, n), func(t *testing.T) {
				defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(procs))
				vals := randomCompactValidators(r, n)
				roots := OptimizedValidatorRoots(vals)
				require.Equal(t, n, len(roots))
				for i := range vals {
					want, err := ValidatorRootWithHasher(vals[i].ToProto())
					require.NoError(t, err)
					require.Equal(t, want, roots[i], "validator %d", i)
				}
			})
		}
	}
}

func TestMerkleizeInPlace_NonPowerOfTwoFields(t *testing.T) {
	// A field count that is not a power of two (9 fields, 16 leaves) must give
	// the same roots as SSZ merkleization with zero padding.
	const fields, depth = 9, 4
	r := rand.New(rand.NewSource(4))

	// GOMAXPROCS sets the group count: one group, uneven last group, more procs than trees.
	for _, procs := range []int{1, 7, 64} {
		for _, n := range []int{1, 5, 1003} {
			t.Run(fmt.Sprintf("procs=%d/n=%d", procs, n), func(t *testing.T) {
				defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(procs))
				fieldRoots := make([][][32]byte, n)
				for i := range fieldRoots {
					fieldRoots[i] = make([][32]byte, fields)
					for k := range fieldRoots[i] {
						r.Read(fieldRoots[i][k][:])
					}
				}
				roots := merkleizeInPlace(n, depth, func(i int, leaves [][32]byte) {
					copy(leaves, fieldRoots[i])
				})
				require.Equal(t, n, len(roots))
				for i := range fieldRoots {
					want, err := ssz.BitwiseMerkleize(fieldRoots[i], fields, fields)
					require.NoError(t, err)
					require.Equal(t, want, roots[i], "tree %d", i)
				}
			})
		}
	}
}

func BenchmarkOptimizedValidatorRoots(b *testing.B) {
	vals := randomCompactValidators(rand.New(rand.NewSource(1)), 1_000_000)
	b.ReportAllocs()
	for b.Loop() {
		OptimizedValidatorRoots(vals)
	}
}
