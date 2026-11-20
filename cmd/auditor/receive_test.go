package main

import (
	"slices"
	"testing"
)

// receive printed tee_info by ranging over the map, in another order on every run
// (docs/results/git-transport-2026-11-20.md, 9). Go randomises map order, so many runs catch an unsorted printout.
func TestTEEInfoLines_Sorted(t *testing.T) {
	info := map[string]string{"zone": "z", "image": "i", "project_id": "p", "instance_name": "n"}
	want := []string{"  tee_info.image = i", "  tee_info.instance_name = n", "  tee_info.project_id = p", "  tee_info.zone = z"}
	for range 200 {
		if got := teeInfoLines(info); !slices.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}
