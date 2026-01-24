package primitives_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	mathprysm "github.com/OffchainLabs/prysm/v7/math"
)

func TestOps_Add(t *testing.T) {
	tests := []struct {
		a, b     uint64
		want     uint64
		panicMsg string
	}{
		{a: 0, b: 0, want: 0},
		{a: 0, b: 1, want: 1},
		{a: 1, b: 0, want: 1},
		{a: 100, b: 200, want: 300},
		{a: 1 << 32, b: 1, want: 4294967297},
		{a: 1 << 32, b: 100, want: 4294967396},
		{a: 1 << 31, b: 1 << 31, want: 4294967296},
		{a: 1 << 63, b: 1, want: 9223372036854775809},
		{a: math.MaxUint64, b: 0, want: math.MaxUint64},
		{a: math.MaxUint64 - 1, b: 1, want: math.MaxUint64},
		// Overflow cases
		{a: math.MaxUint64, b: 1, want: 0, panicMsg: mathprysm.ErrAddOverflow.Error()},
		{a: 1 << 63, b: 1 << 63, want: 0, panicMsg: mathprysm.ErrAddOverflow.Error()},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Add(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() { primitives.Add(primitives.Slot(tt.a), tt.b) })
				assertPanic(t, tt.panicMsg, func() { primitives.AddT(primitives.Slot(tt.a), primitives.Slot(tt.b)) })
			} else {
				got := primitives.Add(primitives.Slot(tt.a), tt.b)
				if uint64(got) != tt.want {
					t.Errorf("Add() = %d, want %d", got, tt.want)
				}
				got = primitives.AddT(primitives.Slot(tt.a), primitives.Slot(tt.b))
				if uint64(got) != tt.want {
					t.Errorf("AddT() = %d, want %d", got, tt.want)
				}
			}
		})
		t.Run(fmt.Sprintf("SafeAdd(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got, err := primitives.SafeAdd(primitives.Slot(tt.a), tt.b)
			checkSafeOp(t, "SafeAdd()", uint64(got), err, tt.want, tt.panicMsg)

			got, err = primitives.SafeAddT(primitives.Slot(tt.a), primitives.Slot(tt.b))
			checkSafeOp(t, "SafeAddT()", uint64(got), err, tt.want, tt.panicMsg)
		})
	}
}

func TestOps_Sub(t *testing.T) {
	tests := []struct {
		a, b     uint64
		want     uint64
		panicMsg string
	}{
		{a: 0, b: 0, want: 0},
		{a: 1, b: 0, want: 1},
		{a: 1, b: 1, want: 0},
		{a: 300, b: 200, want: 100},
		{a: 1 << 32, b: 1, want: 4294967295},
		{a: 1 << 32, b: 100, want: 4294967196},
		{a: 1 << 63, b: 1, want: 9223372036854775807},
		{a: math.MaxUint64, b: 0, want: math.MaxUint64},
		{a: math.MaxUint64, b: math.MaxUint64, want: 0},
		{a: math.MaxUint64, b: 1, want: math.MaxUint64 - 1},
		// Underflow cases
		{a: 0, b: 1, want: 0, panicMsg: mathprysm.ErrSubUnderflow.Error()},
		{a: 100, b: 200, want: 0, panicMsg: mathprysm.ErrSubUnderflow.Error()},
		{a: math.MaxUint64 - 1, b: math.MaxUint64, want: 0, panicMsg: mathprysm.ErrSubUnderflow.Error()},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Sub(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() { primitives.Sub(primitives.Slot(tt.a), tt.b) })
				assertPanic(t, tt.panicMsg, func() { primitives.SubT(primitives.Slot(tt.a), primitives.Slot(tt.b)) })
			} else {
				got := primitives.Sub(primitives.Slot(tt.a), tt.b)
				if uint64(got) != tt.want {
					t.Errorf("Sub() = %d, want %d", got, tt.want)
				}
				got = primitives.SubT(primitives.Slot(tt.a), primitives.Slot(tt.b))
				if uint64(got) != tt.want {
					t.Errorf("SubT() = %d, want %d", got, tt.want)
				}
			}
		})
		t.Run(fmt.Sprintf("SafeSub(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got, err := primitives.SafeSub(primitives.Slot(tt.a), tt.b)
			checkSafeOp(t, "SafeSub()", uint64(got), err, tt.want, tt.panicMsg)

			got, err = primitives.SafeSubT(primitives.Slot(tt.a), primitives.Slot(tt.b))
			checkSafeOp(t, "SafeSubT()", uint64(got), err, tt.want, tt.panicMsg)
		})
	}
}

func TestOps_Mul(t *testing.T) {
	tests := []struct {
		a, b     uint64
		want     uint64
		panicMsg string
	}{
		{a: 0, b: 0, want: 0},
		{a: 0, b: 1, want: 0},
		{a: 1, b: 0, want: 0},
		{a: 1, b: 1, want: 1},
		{a: 100, b: 200, want: 20000},
		{a: 1 << 32, b: 1, want: 1 << 32},
		{a: 1 << 32, b: 100, want: 429496729600},
		{a: 1 << 31, b: 1 << 31, want: 1 << 62},
		{a: 1 << 62, b: 2, want: 1 << 63},
		// Overflow cases
		{a: 1 << 32, b: 1 << 32, want: 0, panicMsg: mathprysm.ErrMulOverflow.Error()},
		{a: 1 << 62, b: 4, want: 0, panicMsg: mathprysm.ErrMulOverflow.Error()},
		{a: 1 << 63, b: 2, want: 0, panicMsg: mathprysm.ErrMulOverflow.Error()},
		{a: math.MaxUint64, b: 2, want: 0, panicMsg: mathprysm.ErrMulOverflow.Error()},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Mul(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() { primitives.Mul(primitives.Slot(tt.a), tt.b) })
				assertPanic(t, tt.panicMsg, func() { primitives.MulT(primitives.Slot(tt.a), primitives.Slot(tt.b)) })
			} else {
				got := primitives.Mul(primitives.Slot(tt.a), tt.b)
				if uint64(got) != tt.want {
					t.Errorf("Mul() = %d, want %d", got, tt.want)
				}
				got = primitives.MulT(primitives.Slot(tt.a), primitives.Slot(tt.b))
				if uint64(got) != tt.want {
					t.Errorf("MulT() = %d, want %d", got, tt.want)
				}
			}
		})
		t.Run(fmt.Sprintf("SafeMul(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got, err := primitives.SafeMul(primitives.Slot(tt.a), tt.b)
			checkSafeOp(t, "SafeMul()", uint64(got), err, tt.want, tt.panicMsg)

			got, err = primitives.SafeMulT(primitives.Slot(tt.a), primitives.Slot(tt.b))
			checkSafeOp(t, "SafeMulT()", uint64(got), err, tt.want, tt.panicMsg)
		})
	}
}

func TestOps_Div(t *testing.T) {
	tests := []struct {
		a, b     uint64
		want     uint64
		panicMsg string
	}{
		{a: 0, b: 1, want: 0},
		{a: 1, b: 1, want: 1},
		{a: 100, b: 10, want: 10},
		{a: 100, b: 3, want: 33}, // Integer division truncates
		{a: 1 << 32, b: 1 << 32, want: 1},
		{a: 429496729600, b: 1 << 32, want: 100},
		{a: 1 << 63, b: 1 << 62, want: 2},
		{a: math.MaxUint64, b: 1, want: math.MaxUint64},
		{a: math.MaxUint64, b: math.MaxUint64, want: 1},
		// Division by zero
		{a: 0, b: 0, want: 0, panicMsg: mathprysm.ErrDivByZero.Error()},
		{a: 1, b: 0, want: 0, panicMsg: mathprysm.ErrDivByZero.Error()},
		{a: math.MaxUint64, b: 0, want: 0, panicMsg: mathprysm.ErrDivByZero.Error()},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Div(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() { primitives.Div(primitives.Slot(tt.a), tt.b) })
				assertPanic(t, tt.panicMsg, func() { primitives.DivT(primitives.Slot(tt.a), primitives.Slot(tt.b)) })
			} else {
				got := primitives.Div(primitives.Slot(tt.a), tt.b)
				if uint64(got) != tt.want {
					t.Errorf("Div() = %d, want %d", got, tt.want)
				}
				got = primitives.DivT(primitives.Slot(tt.a), primitives.Slot(tt.b))
				if uint64(got) != tt.want {
					t.Errorf("DivT() = %d, want %d", got, tt.want)
				}
			}
		})
		t.Run(fmt.Sprintf("SafeDiv(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got, err := primitives.SafeDiv(primitives.Slot(tt.a), tt.b)
			checkSafeOp(t, "SafeDiv()", uint64(got), err, tt.want, tt.panicMsg)

			got, err = primitives.SafeDivT(primitives.Slot(tt.a), primitives.Slot(tt.b))
			checkSafeOp(t, "SafeDivT()", uint64(got), err, tt.want, tt.panicMsg)
		})
	}
}

func TestOps_Mod(t *testing.T) {
	tests := []struct {
		a, b     uint64
		want     uint64
		panicMsg string
	}{
		{a: 0, b: 1, want: 0},
		{a: 1, b: 1, want: 0},
		{a: 5, b: 3, want: 2},
		{a: 100, b: 7, want: 2},
		{a: 1 << 32, b: 17, want: 1},
		{a: 1 << 32, b: 19, want: (1 << 32) % 19},
		{a: 1<<63 + 1, b: 2, want: 1}, // Odd number mod 2
		{a: 1 << 63, b: 2, want: 0},   // Even number mod 2
		{a: math.MaxUint64, b: math.MaxUint64, want: 0},
		{a: math.MaxUint64, b: 2, want: 1}, // MaxUint64 is odd
		// Division by zero
		{a: 0, b: 0, want: 0, panicMsg: mathprysm.ErrDivByZero.Error()},
		{a: 1, b: 0, want: 0, panicMsg: mathprysm.ErrDivByZero.Error()},
		{a: math.MaxUint64, b: 0, want: 0, panicMsg: mathprysm.ErrDivByZero.Error()},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Mod(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() { primitives.Mod(primitives.Slot(tt.a), tt.b) })
				assertPanic(t, tt.panicMsg, func() { primitives.ModT(primitives.Slot(tt.a), primitives.Slot(tt.b)) })
			} else {
				got := primitives.Mod(primitives.Slot(tt.a), tt.b)
				if uint64(got) != tt.want {
					t.Errorf("Mod() = %d, want %d", got, tt.want)
				}
				got = primitives.ModT(primitives.Slot(tt.a), primitives.Slot(tt.b))
				if uint64(got) != tt.want {
					t.Errorf("ModT() = %d, want %d", got, tt.want)
				}
			}
		})
		t.Run(fmt.Sprintf("SafeMod(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got, err := primitives.SafeMod(primitives.Slot(tt.a), tt.b)
			checkSafeOp(t, "SafeMod()", uint64(got), err, tt.want, tt.panicMsg)

			got, err = primitives.SafeModT(primitives.Slot(tt.a), primitives.Slot(tt.b))
			checkSafeOp(t, "SafeModT()", uint64(got), err, tt.want, tt.panicMsg)
		})
	}
}

func TestOps_CappedAdd(t *testing.T) {
	tests := []struct {
		a, b uint64
		want uint64
	}{
		{a: 0, b: 0, want: 0},
		{a: 0, b: 1, want: 1},
		{a: 100, b: 50, want: 150},
		{a: math.MaxUint64 - 1, b: 1, want: math.MaxUint64},
		{a: math.MaxUint64, b: 0, want: math.MaxUint64},
		// Overflow cases - should cap at MaxUint64 instead of panicking
		{a: math.MaxUint64, b: 1, want: math.MaxUint64},
		{a: math.MaxUint64, b: math.MaxUint64, want: math.MaxUint64},
		{a: 1 << 63, b: 1 << 63, want: math.MaxUint64},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("CappedAdd(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := primitives.CappedAdd(primitives.Slot(tt.a), tt.b)
			if uint64(got) != tt.want {
				t.Errorf("CappedAdd() = %d, want %d", got, tt.want)
			}
		})
		t.Run(fmt.Sprintf("CappedAddT(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := primitives.CappedAddT(primitives.Slot(tt.a), primitives.Slot(tt.b))
			if uint64(got) != tt.want {
				t.Errorf("CappedAddT() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOps_FlooredSub(t *testing.T) {
	tests := []struct {
		a, b uint64
		want uint64
	}{
		{a: 0, b: 0, want: 0},
		{a: 1, b: 0, want: 1},
		{a: 100, b: 50, want: 50},
		{a: math.MaxUint64, b: 1, want: math.MaxUint64 - 1},
		// Underflow cases - should floor to 0 instead of panicking
		{a: 0, b: 1, want: 0},
		{a: 50, b: 100, want: 0},
		{a: 0, b: math.MaxUint64, want: 0},
		{a: 1, b: math.MaxUint64, want: 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("FlooredSub(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := primitives.FlooredSub(primitives.Slot(tt.a), tt.b)
			if uint64(got) != tt.want {
				t.Errorf("FlooredSub() = %d, want %d", got, tt.want)
			}
		})
		t.Run(fmt.Sprintf("FlooredSubT(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := primitives.FlooredSubT(primitives.Slot(tt.a), primitives.Slot(tt.b))
			if uint64(got) != tt.want {
				t.Errorf("FlooredSubT() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOps_CappedMul(t *testing.T) {
	tests := []struct {
		a, b uint64
		want uint64
	}{
		{a: 0, b: 0, want: 0},
		{a: 0, b: 1, want: 0},
		{a: 1, b: 1, want: 1},
		{a: 100, b: 50, want: 5000},
		// Large values without overflow
		{a: 1 << 32, b: 1, want: 1 << 32},
		{a: 1 << 31, b: 1 << 31, want: 1 << 62},
		// Overflow cases - should cap at MaxUint64 instead of panicking
		{a: math.MaxUint64, b: 2, want: math.MaxUint64},
		{a: 1 << 32, b: 1 << 32, want: math.MaxUint64},
		{a: 1 << 63, b: 2, want: math.MaxUint64},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("CappedMul(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := primitives.CappedMul(primitives.Slot(tt.a), tt.b)
			if uint64(got) != tt.want {
				t.Errorf("CappedMul() = %d, want %d", got, tt.want)
			}
		})
		t.Run(fmt.Sprintf("CappedMulT(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := primitives.CappedMulT(primitives.Slot(tt.a), primitives.Slot(tt.b))
			if uint64(got) != tt.want {
				t.Errorf("CappedMulT() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOps_Diff(t *testing.T) {
	tests := []struct {
		a, b uint64
		want uint64
	}{
		{a: 0, b: 0, want: 0},
		{a: 100, b: 50, want: 50},
		{a: 50, b: 100, want: 50}, // Absolute difference: |50-100| = 50
		{a: 100, b: 100, want: 0},
		{a: 0, b: math.MaxUint64, want: math.MaxUint64},
		{a: math.MaxUint64, b: 0, want: math.MaxUint64},
		{a: math.MaxUint64, b: math.MaxUint64, want: 0},
		{a: math.MaxUint64, b: math.MaxUint64 - 1, want: 1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Diff(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := primitives.Diff(primitives.Slot(tt.a), tt.b)
			if uint64(got) != tt.want {
				t.Errorf("Diff() = %d, want %d", got, tt.want)
			}
			got = primitives.DiffT(primitives.Slot(tt.a), primitives.Slot(tt.b))
			if uint64(got) != tt.want {
				t.Errorf("DiffT() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOps_Max(t *testing.T) {
	tests := []struct {
		a, b uint64
		want uint64
	}{
		{a: 0, b: 0, want: 0},
		{a: 0, b: 1, want: 1},
		{a: 1, b: 0, want: 1},
		{a: 10, b: 20, want: 20},
		{a: 20, b: 10, want: 20},
		{a: 100, b: 100, want: 100},
		{a: 0, b: math.MaxUint64, want: math.MaxUint64},
		{a: math.MaxUint64, b: 0, want: math.MaxUint64},
		{a: math.MaxUint64, b: math.MaxUint64, want: math.MaxUint64},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Max(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := primitives.Max(primitives.Slot(tt.a), primitives.Slot(tt.b))
			if uint64(got) != tt.want {
				t.Errorf("Max() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOps_Min(t *testing.T) {
	tests := []struct {
		a, b uint64
		want uint64
	}{
		{a: 0, b: 0, want: 0},
		{a: 0, b: 1, want: 0},
		{a: 1, b: 0, want: 0},
		{a: 10, b: 20, want: 10},
		{a: 20, b: 10, want: 10},
		{a: 100, b: 100, want: 100},
		{a: 0, b: math.MaxUint64, want: 0},
		{a: math.MaxUint64, b: 0, want: 0},
		{a: math.MaxUint64, b: math.MaxUint64, want: math.MaxUint64},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Min(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := primitives.Min(primitives.Slot(tt.a), primitives.Slot(tt.b))
			if uint64(got) != tt.want {
				t.Errorf("Min() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOps_Clamp(t *testing.T) {
	tests := []struct {
		val, min, max uint64
		want          uint64
	}{
		// Within range
		{val: 50, min: 0, max: 100, want: 50},
		{val: 0, min: 0, max: 100, want: 0},
		{val: 100, min: 0, max: 100, want: 100},
		// Below min
		{val: 0, min: 10, max: 100, want: 10},
		{val: 5, min: 10, max: 100, want: 10},
		// Above max
		{val: 200, min: 10, max: 100, want: 100},
		{val: math.MaxUint64, min: 0, max: 100, want: 100},
		// Edge case (min == max)
		{val: 0, min: 50, max: 50, want: 50},
		{val: 100, min: 50, max: 50, want: 50},
		{val: 50, min: 50, max: 50, want: 50},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Clamp(%d,%d,%d)", tt.val, tt.min, tt.max), func(t *testing.T) {
			got := primitives.Clamp(primitives.Slot(tt.val), primitives.Slot(tt.min), primitives.Slot(tt.max))
			if uint64(got) != tt.want {
				t.Errorf("Clamp() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOps_IsZero(t *testing.T) {
	tests := []struct {
		val  uint64
		want bool
	}{
		{val: 0, want: true},
		{val: 1, want: false},
		{val: 100, want: false},
		{val: math.MaxUint64, want: false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("IsZero(%d)", tt.val), func(t *testing.T) {
			got := primitives.IsZero(primitives.Slot(tt.val))
			if got != tt.want {
				t.Errorf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func assertPanic(t *testing.T, panicMessage string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic not thrown")
			return
		}
		err, ok := r.(error)
		if !ok {
			t.Errorf("Expected panic with error, got: %T", r)
			return
		}
		if err.Error() != panicMessage {
			t.Errorf("Unexpected panic message, want: %q, got: %q", panicMessage, err.Error())
		}
	}()
	f()
}

// checkSafeOp verifies a safe operation result (value + error).
func checkSafeOp(t *testing.T, name string, got uint64, err error, want uint64, wantErr string) {
	t.Helper()
	if wantErr != "" {
		if err == nil || err.Error() != wantErr {
			t.Errorf("%s error = %v, want %v", name, err, wantErr)
		}
	} else {
		if err != nil {
			t.Errorf("%s unexpected error: %v", name, err)
		}
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
}
