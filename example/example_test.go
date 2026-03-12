package example

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Errorf("Add(2, 3) = %d, want 5", got)
	}
}

func BenchmarkAdd(b *testing.B) {
	for b.Loop() {
		Add(2, 3)
	}
}
