package verification

import "testing"

func TestIsGasLimitTargetCompatible(t *testing.T) {
	tests := []struct {
		name           string
		parentGasLimit uint64
		gasLimit       uint64
		targetGasLimit uint64
		want           bool
	}{
		{
			name:           "increase within limit",
			parentGasLimit: 60_000_000,
			gasLimit:       60_000_100,
			targetGasLimit: 60_000_100,
			want:           true,
		},
		{
			name:           "increase exceeding limit clamps to max",
			parentGasLimit: 60_000_000,
			gasLimit:       60_058_592, // 60_000_000 + (60_000_000/1024 - 1)
			targetGasLimit: 100_000_000,
			want:           true,
		},
		{
			name:           "increase exceeding limit off by one fails",
			parentGasLimit: 60_000_000,
			gasLimit:       60_058_593,
			targetGasLimit: 100_000_000,
			want:           false,
		},
		{
			name:           "decrease within limit",
			parentGasLimit: 60_000_000,
			gasLimit:       59_999_990,
			targetGasLimit: 59_999_990,
			want:           true,
		},
		{
			name:           "decrease exceeding limit clamps to min",
			parentGasLimit: 60_000_000,
			gasLimit:       59_941_408, // 60_000_000 - (60_000_000/1024 - 1)
			targetGasLimit: 30_000_000,
			want:           true,
		},
		{
			name:           "target equals parent",
			parentGasLimit: 60_000_000,
			gasLimit:       60_000_000,
			targetGasLimit: 60_000_000,
			want:           true,
		},
		{
			name:           "parent gas limit underflows is guarded",
			parentGasLimit: 1023,
			gasLimit:       1023,
			targetGasLimit: 60_000_000,
			want:           true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGasLimitTargetCompatible(tt.parentGasLimit, tt.gasLimit, tt.targetGasLimit)
			if got != tt.want {
				t.Errorf("isGasLimitTargetCompatible(%d, %d, %d) = %v, want %v",
					tt.parentGasLimit, tt.gasLimit, tt.targetGasLimit, got, tt.want)
			}
		})
	}
}
