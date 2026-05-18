package primitives

import "testing"

func BenchmarkSlot_Add(b *testing.B) {
	s := Slot(1000)
	for b.Loop() {
		_ = s.Add(100)
	}
}

func BenchmarkSlot_SafeAdd(b *testing.B) {
	s := Slot(1000)
	for b.Loop() {
		_, _ = s.SafeAdd(100)
	}
}

func BenchmarkSlot_AddSlot(b *testing.B) {
	s := Slot(1000)
	x := Slot(100)
	for b.Loop() {
		_ = s.AddSlot(x)
	}
}

func BenchmarkSlot_SafeAddSlot(b *testing.B) {
	s := Slot(1000)
	x := Slot(100)
	for b.Loop() {
		_, _ = s.SafeAddSlot(x)
	}
}
