package primitives

import (
	"math"

	mathprysm "github.com/OffchainLabs/prysm/v7/math"
)

// add increases a by x.
// Panics on overflow.
func add(a, x uint64) uint64 {
	res, err := safeAdd(a, x)
	if err != nil {
		panic(err) // lint:nopanic -- Panic is communicated in the godoc.
	}
	return res
}

// sub subtracts x from a.
// Panics on underflow.
func sub(a, x uint64) uint64 {
	res, err := safeSub(a, x)
	if err != nil {
		panic(err) // lint:nopanic -- Panic is communicated in the godoc.
	}
	return res
}

// mul multiplies a by x.
// Panics on overflow.
func mul(a, x uint64) uint64 {
	res, err := safeMul(a, x)
	if err != nil {
		panic(err) // lint:nopanic -- Panic is communicated in the godoc.
	}
	return res
}

// div divides a by x.
// Panics on division by zero.
func div(a, x uint64) uint64 {
	res, err := safeDiv(a, x)
	if err != nil {
		panic(err) // lint:nopanic -- Panic is communicated in the godoc.
	}
	return res
}

// mod returns a % x.
// Panics on division by zero.
func mod(a, x uint64) uint64 {
	res, err := safeMod(a, x)
	if err != nil {
		panic(err) // lint:nopanic -- Panic is communicated in the godoc.
	}
	return res
}

// safeAdd increases a by x.
// Returns error on overflow.
func safeAdd(a, x uint64) (uint64, error) {
	return mathprysm.Add64(a, x)
}

// safeSub subtracts x from a.
// Returns error on underflow.
func safeSub(a, x uint64) (uint64, error) {
	return mathprysm.Sub64(a, x)
}

// safeMul multiplies a by x.
// Returns error on overflow.
func safeMul(a, x uint64) (uint64, error) {
	return mathprysm.Mul64(a, x)
}

// safeDiv divides a by x.
// Returns error on division by zero.
func safeDiv(a, x uint64) (uint64, error) {
	return mathprysm.Div64(a, x)
}

// safeMod returns a % x.
// Returns error on division by zero.
func safeMod(a, x uint64) (uint64, error) {
	return mathprysm.Mod64(a, x)
}

// cappedAdd increases a by x.
// On overflow, returns MaxUint64.
func cappedAdd(a, x uint64) uint64 {
	res, err := safeAdd(a, x)
	if err != nil {
		return math.MaxUint64
	}
	return res
}

// flooredSub subtracts x from a.
// On underflow, returns 0.
func flooredSub(a, x uint64) uint64 {
	if a < x {
		return 0
	}
	return a - x
}

// cappedMul multiplies a by x.
// On overflow, returns MaxUint64.
func cappedMul(a, x uint64) uint64 {
	res, err := safeMul(a, x)
	if err != nil {
		return math.MaxUint64
	}
	return res
}

// diff returns the absolute difference |a - x|.
func diff(a, x uint64) uint64 {
	if a > x {
		return a - x
	}
	return x - a
}

// isZero returns true if a is zero.
func isZero(a uint64) bool {
	return a == 0
}

// clamp restricts val to the [a, b] range.
func clamp(val, a, b uint64) uint64 {
	if val < a {
		return a
	}
	if val > b {
		return b
	}
	return val
}
