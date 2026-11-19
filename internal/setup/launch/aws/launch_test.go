package aws

import "testing"

func TestMemoryForSize(t *testing.T) {
	for size, want := range map[int64]int{0: 512, 100 << 20: 512, 200 << 20: 1024, 700 << 20: 3072} {
		if got := memoryForSize(size); got != want {
			t.Errorf("memoryForSize(%d MiB) = %d, want %d", size>>20, got, want)
		}
	}
}

func TestUsableMiB(t *testing.T) {
	for _, c := range []struct{ n1G, n2M, want int }{
		{0, 4352, 8192},        // 2 MiB pages alone stop at 4096 regions (measured: 8704 MiB failed with "Reached max memory regions 4096")
		{1, 4096, 1024 + 8190}, // one 1 GiB page leaves room for 4095 small pages (measured: 8704 MiB started)
		{9, 0, 9216},
		{0, 100, 200},
	} {
		if got := usableMiB(c.n1G, c.n2M); got != c.want {
			t.Errorf("usableMiB(%d, %d) = %d, want %d", c.n1G, c.n2M, got, c.want)
		}
	}
}
