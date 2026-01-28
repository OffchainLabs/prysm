package primitives_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	mathprysm "github.com/OffchainLabs/prysm/v7/math"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestMaxEpoch(t *testing.T) {
	require.Equal(t, primitives.Epoch(0), primitives.MaxEpoch(0, 0))
	require.Equal(t, primitives.Epoch(1), primitives.MaxEpoch(1, 0))
	require.Equal(t, primitives.Epoch(1), primitives.MaxEpoch(0, 1))
	require.Equal(t, primitives.Epoch(1000), primitives.MaxEpoch(100, 1000))
}

func TestEpoch_Mul(t *testing.T) {
	tests := []struct {
		a, b     uint64
		res      primitives.Epoch
		panicMsg string
	}{
		{a: 0, b: 1, res: 0},
		{a: 1 << 32, b: 1, res: 1 << 32},
		{a: 1 << 32, b: 100, res: 429496729600},
		{a: 1 << 32, b: 1 << 31, res: 9223372036854775808},
		{a: 1 << 32, b: 1 << 32, res: 0, panicMsg: mathprysm.ErrMulOverflow.Error()},
		{a: 1 << 62, b: 2, res: 9223372036854775808},
		{a: 1 << 62, b: 4, res: 0, panicMsg: mathprysm.ErrMulOverflow.Error()},
		{a: 1 << 63, b: 1, res: 9223372036854775808},
		{a: 1 << 63, b: 2, res: 0, panicMsg: mathprysm.ErrMulOverflow.Error()},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Epoch(%v).Mul(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Epoch
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Epoch(tt.a).Mul(tt.b)
				})
			} else {
				res = primitives.Epoch(tt.a).Mul(tt.b)
			}
			if tt.res != res {
				t.Errorf("Epoch.Mul() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SafeMul(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			res, err := primitives.Epoch(tt.a).SafeMul(tt.b)
			if tt.panicMsg != "" && (err == nil || err.Error() != tt.panicMsg) {
				t.Errorf("Expected error not thrown, wanted: %v, got: %v", tt.panicMsg, err)
				return
			}
			if tt.res != res {
				t.Errorf("Epoch.SafeMul() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).MulEpoch(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Epoch
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Epoch(tt.a).MulEpoch(primitives.Epoch(tt.b))
				})
			} else {
				res = primitives.Epoch(tt.a).MulEpoch(primitives.Epoch(tt.b))
			}
			if tt.res != res {
				t.Errorf("Epoch.MulEpoch() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SafeMulEpoch(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			res, err := primitives.Epoch(tt.a).SafeMulEpoch(primitives.Epoch(tt.b))
			if tt.panicMsg != "" && (err == nil || err.Error() != tt.panicMsg) {
				t.Errorf("Expected error not thrown, wanted: %v, got: %v", tt.panicMsg, err)
				return
			}
			if tt.res != res {
				t.Errorf("Epoch.SafeMulEpoch() = %v, want %v", res, tt.res)
			}
		})
		// CappedMul: on overflow, returns MaxUint64 instead of panicking
		t.Run(fmt.Sprintf("Epoch(%v).CappedMul(%v)", tt.a, tt.b), func(t *testing.T) {
			expectedRes := tt.res
			if tt.panicMsg != "" {
				expectedRes = math.MaxUint64 // CappedMul caps at MaxUint64 on overflow
			}
			res := primitives.Epoch(tt.a).CappedMul(tt.b)
			if res != expectedRes {
				t.Errorf("Epoch.CappedMul() = %v, want %v", res, expectedRes)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).CappedMulEpoch(%v)", tt.a, tt.b), func(t *testing.T) {
			expectedRes := tt.res
			if tt.panicMsg != "" {
				expectedRes = math.MaxUint64 // CappedMulEpoch caps at MaxUint64 on overflow
			}
			res := primitives.Epoch(tt.a).CappedMulEpoch(primitives.Epoch(tt.b))
			if res != expectedRes {
				t.Errorf("Epoch.CappedMulEpoch() = %v, want %v", res, expectedRes)
			}
		})
	}
}

func TestEpoch_Div(t *testing.T) {
	tests := []struct {
		a, b     uint64
		res      primitives.Epoch
		panicMsg string
	}{
		{a: 0, b: 1, res: 0},
		{a: 1, b: 0, res: 0, panicMsg: mathprysm.ErrDivByZero.Error()},
		{a: 1 << 32, b: 1 << 32, res: 1},
		{a: 429496729600, b: 1 << 32, res: 100},
		{a: 9223372036854775808, b: 1 << 32, res: 1 << 31},
		{a: 1 << 32, b: 1 << 32, res: 1},
		{a: 9223372036854775808, b: 1 << 62, res: 2},
		{a: 9223372036854775808, b: 1 << 63, res: 1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Epoch(%v).Div(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Epoch
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Epoch(tt.a).Div(tt.b)
				})
			} else {
				res = primitives.Epoch(tt.a).Div(tt.b)
			}
			if tt.res != res {
				t.Errorf("Epoch.Div() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SafeDiv(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			res, err := primitives.Epoch(tt.a).SafeDiv(tt.b)
			if tt.panicMsg != "" && (err == nil || err.Error() != tt.panicMsg) {
				t.Errorf("Expected error not thrown, wanted: %v, got: %v", tt.panicMsg, err)
				return
			}
			if tt.res != res {
				t.Errorf("Epoch.SafeDiv() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).DivEpoch(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Epoch
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Epoch(tt.a).DivEpoch(primitives.Epoch(tt.b))
				})
			} else {
				res = primitives.Epoch(tt.a).DivEpoch(primitives.Epoch(tt.b))
			}
			if tt.res != res {
				t.Errorf("Epoch.DivEpoch() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SafeDivEpoch(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			res, err := primitives.Epoch(tt.a).SafeDivEpoch(primitives.Epoch(tt.b))
			if tt.panicMsg != "" && (err == nil || err.Error() != tt.panicMsg) {
				t.Errorf("Expected error not thrown, wanted: %v, got: %v", tt.panicMsg, err)
				return
			}
			if tt.res != res {
				t.Errorf("Epoch.SafeDivEpoch() = %v, want %v", res, tt.res)
			}
		})
	}
}

func TestEpoch_Add(t *testing.T) {
	tests := []struct {
		a, b     uint64
		res      primitives.Epoch
		panicMsg string
	}{
		{a: 0, b: 1, res: 1},
		{a: 1 << 32, b: 1, res: 4294967297},
		{a: 1 << 32, b: 100, res: 4294967396},
		{a: 1 << 31, b: 1 << 31, res: 4294967296},
		{a: 1 << 63, b: 1 << 63, res: 0, panicMsg: mathprysm.ErrAddOverflow.Error()},
		{a: 1 << 63, b: 1, res: 9223372036854775809},
		{a: math.MaxUint64, b: 1, res: 0, panicMsg: mathprysm.ErrAddOverflow.Error()},
		{a: math.MaxUint64, b: 0, res: math.MaxUint64},
		{a: 1 << 63, b: 2, res: 9223372036854775810},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Epoch(%v).Add(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Epoch
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Epoch(tt.a).Add(tt.b)
				})
			} else {
				res = primitives.Epoch(tt.a).Add(tt.b)
			}
			if tt.res != res {
				t.Errorf("Epoch.Add() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SafeAdd(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			res, err := primitives.Epoch(tt.a).SafeAdd(tt.b)
			if tt.panicMsg != "" && (err == nil || err.Error() != tt.panicMsg) {
				t.Errorf("Expected error not thrown, wanted: %v, got: %v", tt.panicMsg, err)
				return
			}
			if tt.res != res {
				t.Errorf("Epoch.SafeAdd() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).AddEpoch(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Epoch
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Epoch(tt.a).AddEpoch(primitives.Epoch(tt.b))
				})
			} else {
				res = primitives.Epoch(tt.a).AddEpoch(primitives.Epoch(tt.b))
			}
			if tt.res != res {
				t.Errorf("Epoch.AddEpoch() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SafeAddEpoch(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			res, err := primitives.Epoch(tt.a).SafeAddEpoch(primitives.Epoch(tt.b))
			if tt.panicMsg != "" && (err == nil || err.Error() != tt.panicMsg) {
				t.Errorf("Expected error not thrown, wanted: %v, got: %v", tt.panicMsg, err)
				return
			}
			if tt.res != res {
				t.Errorf("Epoch.SafeAddEpoch() = %v, want %v", res, tt.res)
			}
		})
		// CappedAdd: on overflow, returns MaxUint64 instead of panicking
		t.Run(fmt.Sprintf("Epoch(%v).CappedAdd(%v)", tt.a, tt.b), func(t *testing.T) {
			expectedRes := tt.res
			if tt.panicMsg != "" {
				expectedRes = math.MaxUint64 // CappedAdd caps at MaxUint64 on overflow
			}
			res := primitives.Epoch(tt.a).CappedAdd(tt.b)
			if res != expectedRes {
				t.Errorf("Epoch.CappedAdd() = %v, want %v", res, expectedRes)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).CappedAddEpoch(%v)", tt.a, tt.b), func(t *testing.T) {
			expectedRes := tt.res
			if tt.panicMsg != "" {
				expectedRes = math.MaxUint64 // CappedAddEpoch caps at MaxUint64 on overflow
			}
			res := primitives.Epoch(tt.a).CappedAddEpoch(primitives.Epoch(tt.b))
			if res != expectedRes {
				t.Errorf("Epoch.CappedAddEpoch() = %v, want %v", res, expectedRes)
			}
		})
	}
}

func TestEpoch_Sub(t *testing.T) {
	tests := []struct {
		a, b     uint64
		res      primitives.Epoch
		panicMsg string
	}{
		{a: 1, b: 0, res: 1},
		{a: 0, b: 1, res: 0, panicMsg: mathprysm.ErrSubUnderflow.Error()},
		{a: 1 << 32, b: 1, res: 4294967295},
		{a: 1 << 32, b: 100, res: 4294967196},
		{a: 1 << 31, b: 1 << 31, res: 0},
		{a: 1 << 63, b: 1 << 63, res: 0},
		{a: 1 << 63, b: 1, res: 9223372036854775807},
		{a: math.MaxUint64, b: math.MaxUint64, res: 0},
		{a: math.MaxUint64 - 1, b: math.MaxUint64, res: 0, panicMsg: mathprysm.ErrSubUnderflow.Error()},
		{a: math.MaxUint64, b: 0, res: math.MaxUint64},
		{a: 1 << 63, b: 2, res: 9223372036854775806},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Epoch(%v).Sub(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Epoch
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Epoch(tt.a).Sub(tt.b)
				})
			} else {
				res = primitives.Epoch(tt.a).Sub(tt.b)
			}
			if tt.res != res {
				t.Errorf("Epoch.Sub() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SafeSub(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			res, err := primitives.Epoch(tt.a).SafeSub(tt.b)
			if tt.panicMsg != "" && (err == nil || err.Error() != tt.panicMsg) {
				t.Errorf("Expected error not thrown, wanted: %v, got: %v", tt.panicMsg, err)
				return
			}
			if tt.res != res {
				t.Errorf("Epoch.SafeSub() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SubEpoch(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Epoch
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Epoch(tt.a).SubEpoch(primitives.Epoch(tt.b))
				})
			} else {
				res = primitives.Epoch(tt.a).SubEpoch(primitives.Epoch(tt.b))
			}
			if tt.res != res {
				t.Errorf("Epoch.SubEpoch() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SafeSubEpoch(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			res, err := primitives.Epoch(tt.a).SafeSubEpoch(primitives.Epoch(tt.b))
			if tt.panicMsg != "" && (err == nil || err.Error() != tt.panicMsg) {
				t.Errorf("Expected error not thrown, wanted: %v, got: %v", tt.panicMsg, err)
				return
			}
			if tt.res != res {
				t.Errorf("Epoch.SafeSubEpoch() = %v, want %v", res, tt.res)
			}
		})
		// FlooredSub: on underflow, returns 0 instead of panicking
		t.Run(fmt.Sprintf("Epoch(%v).FlooredSub(%v)", tt.a, tt.b), func(t *testing.T) {
			expectedRes := tt.res
			if tt.panicMsg != "" {
				expectedRes = 0 // FlooredSub floors to 0 on underflow
			}
			res := primitives.Epoch(tt.a).FlooredSub(tt.b)
			if res != expectedRes {
				t.Errorf("Epoch.FlooredSub() = %v, want %v", res, expectedRes)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).FlooredSubEpoch(%v)", tt.a, tt.b), func(t *testing.T) {
			expectedRes := tt.res
			if tt.panicMsg != "" {
				expectedRes = 0 // FlooredSubEpoch floors to 0 on underflow
			}
			res := primitives.Epoch(tt.a).FlooredSubEpoch(primitives.Epoch(tt.b))
			if res != expectedRes {
				t.Errorf("Epoch.FlooredSubEpoch() = %v, want %v", res, expectedRes)
			}
		})
	}
}

func TestEpoch_Mod(t *testing.T) {
	tests := []struct {
		a, b     uint64
		res      primitives.Epoch
		panicMsg string
	}{
		{a: 1, b: 0, res: 0, panicMsg: mathprysm.ErrDivByZero.Error()},
		{a: 0, b: 1, res: 0},
		{a: 1 << 32, b: 1 << 32, res: 0},
		{a: 429496729600, b: 1 << 32, res: 0},
		{a: 9223372036854775808, b: 1 << 32, res: 0},
		{a: 1 << 32, b: 1 << 32, res: 0},
		{a: 9223372036854775808, b: 1 << 62, res: 0},
		{a: 9223372036854775808, b: 1 << 63, res: 0},
		{a: 1 << 32, b: 17, res: 1},
		{a: 1 << 32, b: 19, res: (1 << 32) % 19},
		{a: math.MaxUint64, b: math.MaxUint64, res: 0},
		{a: 1 << 63, b: 2, res: 0},
		{a: 1<<63 + 1, b: 2, res: 1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Epoch(%v).Mod(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Epoch
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Epoch(tt.a).Mod(tt.b)
				})
			} else {
				res = primitives.Epoch(tt.a).Mod(tt.b)
			}
			if tt.res != res {
				t.Errorf("Epoch.Mod() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SafeMod(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			res, err := primitives.Epoch(tt.a).SafeMod(tt.b)
			if tt.panicMsg != "" && (err == nil || err.Error() != tt.panicMsg) {
				t.Errorf("Expected error not thrown, wanted: %v, got: %v", tt.panicMsg, err)
				return
			}
			if tt.res != res {
				t.Errorf("Epoch.SafeMod() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).ModEpoch(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			var res primitives.Epoch
			if tt.panicMsg != "" {
				assertPanic(t, tt.panicMsg, func() {
					res = primitives.Epoch(tt.a).ModEpoch(primitives.Epoch(tt.b))
				})
			} else {
				res = primitives.Epoch(tt.a).ModEpoch(primitives.Epoch(tt.b))
			}
			if tt.res != res {
				t.Errorf("Epoch.ModEpoch() = %v, want %v", res, tt.res)
			}
		})
		t.Run(fmt.Sprintf("Epoch(%v).SafeModEpoch(%v) = %v", tt.a, tt.b, tt.res), func(t *testing.T) {
			res, err := primitives.Epoch(tt.a).SafeModEpoch(primitives.Epoch(tt.b))
			if tt.panicMsg != "" && (err == nil || err.Error() != tt.panicMsg) {
				t.Errorf("Expected error not thrown, wanted: %v, got: %v", tt.panicMsg, err)
				return
			}
			if tt.res != res {
				t.Errorf("Epoch.SafeModEpoch() = %v, want %v", res, tt.res)
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
