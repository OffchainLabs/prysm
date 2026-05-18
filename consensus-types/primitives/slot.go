package primitives

import (
	"fmt"

	fssz "github.com/prysmaticlabs/fastssz"
)

var (
	_ fssz.HashRoot    = (Slot)(0)
	_ fssz.Marshaler   = (*Slot)(nil)
	_ fssz.Unmarshaler = (*Slot)(nil)
)

// Slot represents a single slot.
type Slot uint64

// Uint64 returns the slot as a uint64.
func (s Slot) Uint64() uint64 {
	return uint64(s)
}

// Mul multiplies slot by x.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (s Slot) Mul(x uint64) Slot {
	return Slot(mul(uint64(s), x))
}

// SafeMul multiplies slot by x.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (s Slot) SafeMul(x uint64) (Slot, error) {
	res, err := safeMul(uint64(s), x)
	return Slot(res), err
}

// CappedMul safely multiplies the slot by x, returning MaxUint64 if the result would overflow.
func (s Slot) CappedMul(x uint64) Slot {
	return Slot(cappedMul(uint64(s), x))
}

// MulSlot multiplies slot by another slot.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (s Slot) MulSlot(x Slot) Slot {
	return Slot(mul(uint64(s), uint64(x)))
}

// SafeMulSlot multiplies slot by another slot.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (s Slot) SafeMulSlot(x Slot) (Slot, error) {
	res, err := safeMul(uint64(s), uint64(x))
	return Slot(res), err
}

// CappedMulSlot safely multiplies the slot by x, returning MaxUint64 if the result would overflow.
func (s Slot) CappedMulSlot(x Slot) Slot {
	return Slot(cappedMul(uint64(s), uint64(x)))
}

// Div divides slot by x.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (s Slot) Div(x uint64) Slot {
	return Slot(div(uint64(s), x))
}

// SafeDiv divides slot by x.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (s Slot) SafeDiv(x uint64) (Slot, error) {
	res, err := safeDiv(uint64(s), x)
	return Slot(res), err
}

// DivSlot divides slot by another slot.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (s Slot) DivSlot(x Slot) Slot {
	return Slot(div(uint64(s), uint64(x)))
}

// SafeDivSlot divides slot by another slot.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (s Slot) SafeDivSlot(x Slot) (Slot, error) {
	res, err := safeDiv(uint64(s), uint64(x))
	return Slot(res), err
}

// Add increases slot by x.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (s Slot) Add(x uint64) Slot {
	return Slot(add(uint64(s), x))
}

// SafeAdd increases slot by x.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (s Slot) SafeAdd(x uint64) (Slot, error) {
	res, err := safeAdd(uint64(s), x)
	return Slot(res), err
}

// CappedAdd safely adds x to the slot, returning MaxUint64 if the result would overflow.
func (s Slot) CappedAdd(x uint64) Slot {
	return Slot(cappedAdd(uint64(s), x))
}

// AddSlot increases slot by another slot.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (s Slot) AddSlot(x Slot) Slot {
	return Slot(add(uint64(s), uint64(x)))
}

// SafeAddSlot increases slot by another slot.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (s Slot) SafeAddSlot(x Slot) (Slot, error) {
	res, err := safeAdd(uint64(s), uint64(x))
	return Slot(res), err
}

// CappedAddSlot safely adds x to the slot, returning MaxUint64 if the result would overflow.
func (s Slot) CappedAddSlot(x Slot) Slot {
	return Slot(cappedAdd(uint64(s), uint64(x)))
}

// Sub subtracts x from the slot.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (s Slot) Sub(x uint64) Slot {
	return Slot(sub(uint64(s), x))
}

// SafeSub subtracts x from the slot.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (s Slot) SafeSub(x uint64) (Slot, error) {
	res, err := safeSub(uint64(s), x)
	return Slot(res), err
}

// FlooredSub safely subtracts x from the slot, returning 0 if the result would underflow.
func (s Slot) FlooredSub(x uint64) Slot {
	return Slot(flooredSub(uint64(s), x))
}

// SubSlot finds difference between two slot values.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (s Slot) SubSlot(x Slot) Slot {
	return Slot(sub(uint64(s), uint64(x)))
}

// SafeSubSlot finds difference between two slot values.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (s Slot) SafeSubSlot(x Slot) (Slot, error) {
	res, err := safeSub(uint64(s), uint64(x))
	return Slot(res), err
}

// FlooredSubSlot safely subtracts x from the slot, returning 0 if the result would underflow.
func (s Slot) FlooredSubSlot(x Slot) Slot {
	return Slot(flooredSub(uint64(s), uint64(x)))
}

// Diff returns the absolute difference between slot and x.
func (s Slot) Diff(x uint64) Slot {
	return Slot(diff(uint64(s), x))
}

// DiffSlot returns the absolute difference between two slots.
func (s Slot) DiffSlot(x Slot) Slot {
	return Slot(diff(uint64(s), uint64(x)))
}

// Mod returns result of `slot % x`.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (s Slot) Mod(x uint64) Slot {
	return Slot(mod(uint64(s), x))
}

// SafeMod returns result of `slot % x`.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (s Slot) SafeMod(x uint64) (Slot, error) {
	res, err := safeMod(uint64(s), x)
	return Slot(res), err
}

// ModSlot returns result of `slot % slot`.
// In case of arithmetic issues (overflow/underflow/div by zero) panic is thrown.
func (s Slot) ModSlot(x Slot) Slot {
	return Slot(mod(uint64(s), uint64(x)))
}

// SafeModSlot returns result of `slot % slot`.
// In case of arithmetic issues (overflow/underflow/div by zero) error is returned.
func (s Slot) SafeModSlot(x Slot) (Slot, error) {
	res, err := safeMod(uint64(s), uint64(x))
	return Slot(res), err
}

// HashTreeRoot --
func (s Slot) HashTreeRoot() ([32]byte, error) {
	return fssz.HashWithDefaultHasher(s)
}

// HashTreeRootWith --
func (s Slot) HashTreeRootWith(hh *fssz.Hasher) error {
	hh.PutUint64(uint64(s))
	return nil
}

// UnmarshalSSZ --
func (s *Slot) UnmarshalSSZ(buf []byte) error {
	if len(buf) != s.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", s.SizeSSZ(), len(buf))
	}
	*s = Slot(fssz.UnmarshallUint64(buf))
	return nil
}

// MarshalSSZTo --
func (s *Slot) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := s.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ --
func (s *Slot) MarshalSSZ() ([]byte, error) {
	marshalled := fssz.MarshalUint64([]byte{}, uint64(*s))
	return marshalled, nil
}

// SizeSSZ --
func (s *Slot) SizeSSZ() int {
	return 8
}
