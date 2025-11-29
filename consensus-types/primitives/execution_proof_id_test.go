package primitives_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

func TestExecutionProofId_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		id    primitives.ExecutionProofId
		valid bool
	}{
		{
			name:  "valid proof id 0",
			id:    0,
			valid: true,
		},
		{
			name:  "valid proof id 1",
			id:    1,
			valid: true,
		},
		{
			name:  "valid proof id 7 (max valid)",
			id:    7,
			valid: true,
		},
		{
			name:  "invalid proof id 8 (at limit)",
			id:    8,
			valid: false,
		},
		{
			name:  "invalid proof id 255",
			id:    255,
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.IsValid(); got != tt.valid {
				t.Errorf("ExecutionProofId.IsValid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestExecutionProofId_Casting(t *testing.T) {
	id := primitives.ExecutionProofId(5)

	t.Run("uint8", func(t *testing.T) {
		if uint8(id) != 5 {
			t.Errorf("Casting to uint8 failed: got %v, want 5", uint8(id))
		}
	})

	t.Run("from uint8", func(t *testing.T) {
		var x uint8 = 7
		if primitives.ExecutionProofId(x) != 7 {
			t.Errorf("Casting from uint8 failed: got %v, want 7", primitives.ExecutionProofId(x))
		}
	})

	t.Run("int", func(t *testing.T) {
		var x = 3
		if primitives.ExecutionProofId(x) != 3 {
			t.Errorf("Casting from int failed: got %v, want 3", primitives.ExecutionProofId(x))
		}
	})
}
