package aws

import "testing"

func TestMemoryForSize(t *testing.T) {
	for size, want := range map[int64]int{0: 512, 100 << 20: 512, 200 << 20: 1024, 700 << 20: 3072} {
		if got := memoryForSize(size); got != want {
			t.Errorf("memoryForSize(%d MiB) = %d, want %d", size>>20, got, want)
		}
	}
}
