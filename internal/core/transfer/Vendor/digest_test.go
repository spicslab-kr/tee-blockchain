package vendor

import (
	"testing"
)

func TestDigest(t *testing.T) {
	if got := Digest([]byte("abc")); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("sha256(abc) = %s", got)
	}
}
