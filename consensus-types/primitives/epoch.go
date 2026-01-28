package primitives

import (
	"fmt"

	fssz "github.com/prysmaticlabs/fastssz"
)

var (
	_ fssz.HashRoot    = (Epoch)(0)
	_ fssz.Marshaler   = (*Epoch)(nil)
	_ fssz.Unmarshaler = (*Epoch)(nil)
)

// Epoch represents a single epoch.
type Epoch uint64

// Uint64 returns the epoch as a uint64.
func (e Epoch) Uint64() uint64 {
	return uint64(e)
}

// MaxEpoch compares two epochs and returns the greater one.
func MaxEpoch(a, b Epoch) Epoch {
	return max(a, b)
}

// Mul multiplies epoch by x.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (e Epoch) Mul(x uint64) Epoch {
	return Epoch(mul(uint64(e), x))
}

// SafeMul multiplies epoch by x.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (e Epoch) SafeMul(x uint64) (Epoch, error) {
	res, err := safeMul(uint64(e), x)
	return Epoch(res), err
}

// CappedMul safely multiplies the epoch by x, returning MaxUint64 if the result would overflow.
func (e Epoch) CappedMul(x uint64) Epoch {
	return Epoch(cappedMul(uint64(e), x))
}

// MulEpoch multiplies epoch by another epoch.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (e Epoch) MulEpoch(x Epoch) Epoch {
	return Epoch(mul(uint64(e), uint64(x)))
}

// SafeMulEpoch multiplies epoch by another epoch.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (e Epoch) SafeMulEpoch(x Epoch) (Epoch, error) {
	res, err := safeMul(uint64(e), uint64(x))
	return Epoch(res), err
}

// CappedMulEpoch safely multiplies the epoch by x, returning MaxUint64 if the result would overflow.
func (e Epoch) CappedMulEpoch(x Epoch) Epoch {
	return Epoch(cappedMul(uint64(e), uint64(x)))
}

// Div divides epoch by x.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (e Epoch) Div(x uint64) Epoch {
	return Epoch(div(uint64(e), x))
}

// SafeDiv divides epoch by x.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (e Epoch) SafeDiv(x uint64) (Epoch, error) {
	res, err := safeDiv(uint64(e), x)
	return Epoch(res), err
}

// DivEpoch divides epoch by another epoch.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (e Epoch) DivEpoch(x Epoch) Epoch {
	return Epoch(div(uint64(e), uint64(x)))
}

// SafeDivEpoch divides epoch by another epoch.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (e Epoch) SafeDivEpoch(x Epoch) (Epoch, error) {
	res, err := safeDiv(uint64(e), uint64(x))
	return Epoch(res), err
}

// Add increases epoch by x.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (e Epoch) Add(x uint64) Epoch {
	return Epoch(add(uint64(e), x))
}

// SafeAdd increases epoch by x.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (e Epoch) SafeAdd(x uint64) (Epoch, error) {
	res, err := safeAdd(uint64(e), x)
	return Epoch(res), err
}

// CappedAdd safely adds x to the epoch, returning MaxUint64 if the result would overflow.
func (e Epoch) CappedAdd(x uint64) Epoch {
	return Epoch(cappedAdd(uint64(e), x))
}

// AddEpoch increases epoch using another epoch value.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (e Epoch) AddEpoch(x Epoch) Epoch {
	return Epoch(add(uint64(e), uint64(x)))
}

// SafeAddEpoch increases epoch using another epoch value.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (e Epoch) SafeAddEpoch(x Epoch) (Epoch, error) {
	res, err := safeAdd(uint64(e), uint64(x))
	return Epoch(res), err
}

// CappedAddEpoch safely adds x to the epoch, returning MaxUint64 if the result would overflow.
func (e Epoch) CappedAddEpoch(x Epoch) Epoch {
	return Epoch(cappedAdd(uint64(e), uint64(x)))
}

// Sub subtracts x from the epoch.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (e Epoch) Sub(x uint64) Epoch {
	return Epoch(sub(uint64(e), x))
}

// SafeSub subtracts x from the epoch.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (e Epoch) SafeSub(x uint64) (Epoch, error) {
	res, err := safeSub(uint64(e), x)
	return Epoch(res), err
}

// FlooredSub safely subtracts x from the epoch, returning 0 if the result would underflow.
func (e Epoch) FlooredSub(x uint64) Epoch {
	return Epoch(flooredSub(uint64(e), x))
}

// SubEpoch finds difference between two epoch values.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (e Epoch) SubEpoch(x Epoch) Epoch {
	return Epoch(sub(uint64(e), uint64(x)))
}

// SafeSubEpoch finds difference between two epoch values.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (e Epoch) SafeSubEpoch(x Epoch) (Epoch, error) {
	res, err := safeSub(uint64(e), uint64(x))
	return Epoch(res), err
}

// FlooredSubEpoch safely subtracts x from the epoch, returning 0 if the result would underflow.
func (e Epoch) FlooredSubEpoch(x Epoch) Epoch {
	return Epoch(flooredSub(uint64(e), uint64(x)))
}

// Diff returns the absolute difference between epoch and x.
func (e Epoch) Diff(x uint64) Epoch {
	return Epoch(diff(uint64(e), x))
}

// DiffEpoch returns the absolute difference between two epochs.
func (e Epoch) DiffEpoch(x Epoch) Epoch {
	return Epoch(diff(uint64(e), uint64(x)))
}

// Mod returns result of `epoch % x`.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (e Epoch) Mod(x uint64) Epoch {
	return Epoch(mod(uint64(e), x))
}

// SafeMod returns result of `epoch % x`.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (e Epoch) SafeMod(x uint64) (Epoch, error) {
	res, err := safeMod(uint64(e), x)
	return Epoch(res), err
}

// ModEpoch returns result of `epoch % epoch`.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (e Epoch) ModEpoch(x Epoch) Epoch {
	return Epoch(mod(uint64(e), uint64(x)))
}

// SafeModEpoch returns result of `epoch % epoch`.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (e Epoch) SafeModEpoch(x Epoch) (Epoch, error) {
	res, err := safeMod(uint64(e), uint64(x))
	return Epoch(res), err
}

// HashTreeRoot --
func (e Epoch) HashTreeRoot() ([32]byte, error) {
	return fssz.HashWithDefaultHasher(e)
}

// HashTreeRootWith --
func (e Epoch) HashTreeRootWith(hh *fssz.Hasher) error {
	hh.PutUint64(uint64(e))
	return nil
}

// UnmarshalSSZ --
func (e *Epoch) UnmarshalSSZ(buf []byte) error {
	if len(buf) != e.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", e.SizeSSZ(), len(buf))
	}
	*e = Epoch(fssz.UnmarshallUint64(buf))
	return nil
}

// MarshalSSZTo --
func (e *Epoch) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := e.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ --
func (e *Epoch) MarshalSSZ() ([]byte, error) {
	marshalled := fssz.MarshalUint64([]byte{}, uint64(*e))
	return marshalled, nil
}

// SizeSSZ --
func (e *Epoch) SizeSSZ() int {
	return 8
}
