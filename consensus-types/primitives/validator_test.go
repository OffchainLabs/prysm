package primitives

import (
	"testing"
	"time"
)

func TestValidatorIndex_Casting(t *testing.T) {
	valIdx := ValidatorIndex(42)

	t.Run("time.Duration", func(t *testing.T) {
		if uint64(time.Duration(valIdx)) != uint64(valIdx) {
			t.Error("ValidatorIndex should produce the same result with time.Duration")
		}
	})

	t.Run("floats", func(t *testing.T) {
		var x1 float32 = 42.2
		if ValidatorIndex(x1) != valIdx {
			t.Errorf("Unequal: %v = %v", ValidatorIndex(x1), valIdx)
		}

		var x2 = 42.2
		if ValidatorIndex(x2) != valIdx {
			t.Errorf("Unequal: %v = %v", ValidatorIndex(x2), valIdx)
		}
	})

	t.Run("int", func(t *testing.T) {
		var x = 42
		if ValidatorIndex(x) != valIdx {
			t.Errorf("Unequal: %v = %v", ValidatorIndex(x), valIdx)
		}
	})
}

func TestValidatorIndex_BuilderIndexFlagConversions(t *testing.T) {
	base := uint64(42)

	unflagged := ValidatorIndex(base)
	if unflagged.IsBuilderIndex() {
		t.Fatalf("expected unflagged validator index to not be a builder index")
	}
	if got, want := unflagged.ToBuilderIndex(), BuilderIndex(base); got != want {
		t.Fatalf("unexpected builder index: got %d want %d", got, want)
	}

	flagged := ValidatorIndex(base | BuilderIndexFlag)
	if !flagged.IsBuilderIndex() {
		t.Fatalf("expected flagged validator index to be a builder index")
	}
	if got, want := flagged.ToBuilderIndex(), BuilderIndex(base); got != want {
		t.Fatalf("unexpected builder index: got %d want %d", got, want)
	}

	builder := BuilderIndex(base)
	roundTrip := builder.ToValidatorIndex()
	if !roundTrip.IsBuilderIndex() {
		t.Fatalf("expected round-tripped validator index to be a builder index")
	}
	if got, want := roundTrip.ToBuilderIndex(), builder; got != want {
		t.Fatalf("unexpected round-trip builder index: got %d want %d", got, want)
	}
}
