package primitives

import (
	"fmt"
	"math"
	"testing"

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
		t.Run(fmt.Sprintf("add(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() { add(tt.a, tt.b) })
			} else {
				got := add(tt.a, tt.b)
				if got != tt.want {
					t.Errorf("add() = %d, want %d", got, tt.want)
				}
			}
		})
		t.Run(fmt.Sprintf("safeAdd(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got, err := safeAdd(tt.a, tt.b)
			checkSafeOp(t, "safeAdd()", got, err, tt.want, tt.panicMsg)
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
		t.Run(fmt.Sprintf("sub(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() { sub(tt.a, tt.b) })
			} else {
				got := sub(tt.a, tt.b)
				if got != tt.want {
					t.Errorf("sub() = %d, want %d", got, tt.want)
				}
			}
		})
		t.Run(fmt.Sprintf("safeSub(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got, err := safeSub(tt.a, tt.b)
			checkSafeOp(t, "safeSub()", got, err, tt.want, tt.panicMsg)
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
		t.Run(fmt.Sprintf("mul(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() { mul(tt.a, tt.b) })
			} else {
				got := mul(tt.a, tt.b)
				if got != tt.want {
					t.Errorf("mul() = %d, want %d", got, tt.want)
				}
			}
		})
		t.Run(fmt.Sprintf("safeMul(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got, err := safeMul(tt.a, tt.b)
			checkSafeOp(t, "safeMul()", got, err, tt.want, tt.panicMsg)
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
		t.Run(fmt.Sprintf("div(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() { div(tt.a, tt.b) })
			} else {
				got := div(tt.a, tt.b)
				if got != tt.want {
					t.Errorf("div() = %d, want %d", got, tt.want)
				}
			}
		})
		t.Run(fmt.Sprintf("safeDiv(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got, err := safeDiv(tt.a, tt.b)
			checkSafeOp(t, "safeDiv()", got, err, tt.want, tt.panicMsg)
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
		t.Run(fmt.Sprintf("mod(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() { mod(tt.a, tt.b) })
			} else {
				got := mod(tt.a, tt.b)
				if got != tt.want {
					t.Errorf("mod() = %d, want %d", got, tt.want)
				}
			}
		})
		t.Run(fmt.Sprintf("safeMod(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got, err := safeMod(tt.a, tt.b)
			checkSafeOp(t, "safeMod()", got, err, tt.want, tt.panicMsg)
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
		t.Run(fmt.Sprintf("cappedAdd(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := cappedAdd(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("cappedAdd() = %d, want %d", got, tt.want)
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
		t.Run(fmt.Sprintf("flooredSub(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := flooredSub(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("flooredSub() = %d, want %d", got, tt.want)
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
		t.Run(fmt.Sprintf("cappedMul(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := cappedMul(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("cappedMul() = %d, want %d", got, tt.want)
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
		t.Run(fmt.Sprintf("diff(%d,%d)", tt.a, tt.b), func(t *testing.T) {
			got := diff(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("diff() = %d, want %d", got, tt.want)
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
		t.Run(fmt.Sprintf("clamp(%d,%d,%d)", tt.val, tt.min, tt.max), func(t *testing.T) {
			got := clamp(tt.val, tt.min, tt.max)
			if got != tt.want {
				t.Errorf("clamp() = %d, want %d", got, tt.want)
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
		t.Run(fmt.Sprintf("isZero(%d)", tt.val), func(t *testing.T) {
			got := isZero(tt.val)
			if got != tt.want {
				t.Errorf("isZero() = %v, want %v", got, tt.want)
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
