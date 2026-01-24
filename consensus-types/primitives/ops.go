package primitives

import (
	"math"

	mathprysm "github.com/OffchainLabs/prysm/v7/math"
)

// Uint64Primitive constrains types with underlying uint64.
type Uint64Primitive interface {
	~uint64
}

// Add increases a by x.
// Panics on overflow.
func Add[T Uint64Primitive](a T, x uint64) T {
	res, err := SafeAdd(a, x)
	if err != nil {
		panic(err) // lint:nopanic -- Panic is communicated in the godoc.
	}
	return res
}

// AddT increases a by b (same type).
// Panics on overflow.
func AddT[T Uint64Primitive](a, b T) T {
	return Add(a, uint64(b))
}

// Sub subtracts x from a.
// Panics on underflow.
func Sub[T Uint64Primitive](a T, x uint64) T {
	res, err := SafeSub(a, x)
	if err != nil {
		panic(err) // lint:nopanic -- Panic is communicated in the godoc.
	}
	return res
}

// SubT subtracts b from a (same type).
// Panics on underflow.
func SubT[T Uint64Primitive](a, b T) T {
	return Sub(a, uint64(b))
}

// Mul multiplies a by x.
// Panics on overflow.
func Mul[T Uint64Primitive](a T, x uint64) T {
	res, err := SafeMul(a, x)
	if err != nil {
		panic(err) // lint:nopanic -- Panic is communicated in the godoc.
	}
	return res
}

// MulT multiplies a by b (same type).
// Panics on overflow.
func MulT[T Uint64Primitive](a, b T) T {
	return Mul(a, uint64(b))
}

// Div divides a by x.
// Panics on division by zero.
func Div[T Uint64Primitive](a T, x uint64) T {
	res, err := SafeDiv(a, x)
	if err != nil {
		panic(err) // lint:nopanic -- Panic is communicated in the godoc.
	}
	return res
}

// DivT divides a by b (same type).
// Panics on division by zero.
func DivT[T Uint64Primitive](a, b T) T {
	return Div(a, uint64(b))
}

// Mod returns a % x.
// Panics on division by zero.
func Mod[T Uint64Primitive](a T, x uint64) T {
	res, err := SafeMod(a, x)
	if err != nil {
		panic(err) // lint:nopanic -- Panic is communicated in the godoc.
	}
	return res
}

// ModT returns a % b (same type).
// Panics on division by zero.
func ModT[T Uint64Primitive](a, b T) T {
	return Mod(a, uint64(b))
}

// SafeAdd increases a by x.
// Returns error on overflow.
func SafeAdd[T Uint64Primitive](a T, x uint64) (T, error) {
	res, err := mathprysm.Add64(uint64(a), x)
	return T(res), err
}

// SafeAddT increases a by b (same type).
// Returns error on overflow.
func SafeAddT[T Uint64Primitive](a, b T) (T, error) {
	return SafeAdd(a, uint64(b))
}

// SafeSub subtracts x from a.
// Returns error on underflow.
func SafeSub[T Uint64Primitive](a T, x uint64) (T, error) {
	res, err := mathprysm.Sub64(uint64(a), x)
	return T(res), err
}

// SafeSubT subtracts b from a (same type).
// Returns error on underflow.
func SafeSubT[T Uint64Primitive](a, b T) (T, error) {
	return SafeSub(a, uint64(b))
}

// SafeMul multiplies a by x.
// Returns error on overflow.
func SafeMul[T Uint64Primitive](a T, x uint64) (T, error) {
	res, err := mathprysm.Mul64(uint64(a), x)
	return T(res), err
}

// SafeMulT multiplies a by b (same type).
// Returns error on overflow.
func SafeMulT[T Uint64Primitive](a, b T) (T, error) {
	return SafeMul(a, uint64(b))
}

// SafeDiv divides a by x.
// Returns error on division by zero.
func SafeDiv[T Uint64Primitive](a T, x uint64) (T, error) {
	res, err := mathprysm.Div64(uint64(a), x)
	return T(res), err
}

// SafeDivT divides a by b (same type).
// Returns error on division by zero.
func SafeDivT[T Uint64Primitive](a, b T) (T, error) {
	return SafeDiv(a, uint64(b))
}

// SafeMod returns a % x.
// Returns error on division by zero.
func SafeMod[T Uint64Primitive](a T, x uint64) (T, error) {
	res, err := mathprysm.Mod64(uint64(a), x)
	return T(res), err
}

// SafeModT returns a % b (same type).
// Returns error on division by zero.
func SafeModT[T Uint64Primitive](a, b T) (T, error) {
	return SafeMod(a, uint64(b))
}

// CappedAdd increases a by x.
// On overflow, returns MaxUint64.
func CappedAdd[T Uint64Primitive](a T, x uint64) T {
	res, err := SafeAdd(a, x)
	if err != nil {
		return T(uint64(math.MaxUint64))
	}
	return res
}

// CappedAddT increases a by b (same type).
// On overflow, returns MaxUint64.
func CappedAddT[T Uint64Primitive](a, b T) T {
	return CappedAdd(a, uint64(b))
}

// FlooredSub subtracts x from a.
// On underflow, returns 0.
func FlooredSub[T Uint64Primitive](a T, x uint64) T {
	if uint64(a) < x {
		return 0
	}
	return a - T(x)
}

// FlooredSubT subtracts b from a (same type).
// On underflow, returns 0.
func FlooredSubT[T Uint64Primitive](a, b T) T {
	return FlooredSub(a, uint64(b))
}

// CappedMul multiplies a by x.
// On overflow, returns MaxUint64.
func CappedMul[T Uint64Primitive](a T, x uint64) T {
	res, err := SafeMul(a, x)
	if err != nil {
		return T(uint64(math.MaxUint64))
	}
	return res
}

// CappedMulT multiplies a by b (same type).
// On overflow, returns MaxUint64.
func CappedMulT[T Uint64Primitive](a, b T) T {
	return CappedMul(a, uint64(b))
}

// Diff returns the absolute difference |a - x|.
func Diff[T Uint64Primitive](a T, x uint64) T {
	if uint64(a) > x {
		return a - T(x)
	}
	return T(x) - a
}

// DiffT returns the absolute difference |a - b| (same type).
func DiffT[T Uint64Primitive](a, b T) T {
	return Diff(a, uint64(b))
}

// Max returns the larger of a and b.
func Max[T Uint64Primitive](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// Min returns the smaller of a and b.
func Min[T Uint64Primitive](a, b T) T {
	if a < b {
		return a
	}
	return b
}

// IsZero returns true if a is zero.
func IsZero[T Uint64Primitive](a T) bool {
	return a == 0
}

// Clamp restricts val to the [a, b] range.
func Clamp[T Uint64Primitive](val, a, b T) T {
	if val < a {
		return a
	}
	if val > b {
		return b
	}
	return val
}
