package peerdas

import (
	"math/big"

	"github.com/prysmaticlabs/prysm/v5/config/params"
)

// ExtendedSampleCount computes, for a given number of samples per slot and allowed failures the
// number of samples we should actually query from peers.
// https://github.com/ethereum/consensus-specs/blob/dev/specs/fulu/peer-sampling.md#get_extended_sample_count
func ExtendedSampleCount(samplesPerSlot, allowedFailures uint64) uint64 {
	// Retrieve the columns count
	columnsCount := params.BeaconConfig().NumberOfColumns

	// If half of the columns are missing, we are able to reconstruct the data.
	// If half of the columns + 1 are missing, we are not able to reconstruct the data.
	// This is the smallest worst case.
	worstCaseMissing := columnsCount/2 + 1

	// Compute the false positive threshold.
	falsePositiveThreshold := HypergeomCDF(0, columnsCount, worstCaseMissing, samplesPerSlot)

	var sampleCount uint64

	// Finally, compute the extended sample count.
	for sampleCount = samplesPerSlot; sampleCount < columnsCount+1; sampleCount++ {
		if HypergeomCDF(allowedFailures, columnsCount, worstCaseMissing, sampleCount) <= falsePositiveThreshold {
			break
		}
	}

	return sampleCount
}

// HypergeomCDF computes the hypergeometric cumulative distribution function.
// https://en.wikipedia.org/wiki/Hypergeometric_distribution
func HypergeomCDF(k, M, n, N uint64) float64 {
	denominatorInt := new(big.Int).Binomial(int64(M), int64(N)) // lint:ignore uintcast
	denominator := new(big.Float).SetInt(denominatorInt)

	rBig := big.NewFloat(0)

	for i := uint64(0); i < k+1; i++ {
		a := new(big.Int).Binomial(int64(n), int64(i)) // lint:ignore uintcast
		b := new(big.Int).Binomial(int64(M-n), int64(N-i))
		numeratorInt := new(big.Int).Mul(a, b)
		numerator := new(big.Float).SetInt(numeratorInt)
		item := new(big.Float).Quo(numerator, denominator)
		rBig.Add(rBig, item)
	}

	r, _ := rBig.Float64()

	return r
}
