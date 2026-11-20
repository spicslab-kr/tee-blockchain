package main

import (
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A file error must not exit 3 ("network error"): syscall.Errno satisfies net.Error (docs/results/git-transport-2026-11-20.md).
// A real network error still does.
func TestExitCode_FileErrorIsNotNetwork(t *testing.T) {
	_, missing := os.ReadFile(filepath.Join(t.TempDir(), "build-config.json"))
	if missing == nil {
		t.Fatal("reading a missing file must fail")
	}
	for name, c := range map[string]struct {
		err  error
		want int
	}{
		"file not found":          {missing, 1},
		"file not found, wrapped": {fmt.Errorf("https://example.invalid/repo.git: %w", missing), 1},
		"permission denied":       {&fs.PathError{Op: "open", Path: "build-config.json", Err: syscall.EACCES}, 1},
		"connection refused":      {&net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, 3},
		"dns timeout":             {&net.DNSError{Err: "i/o timeout", Name: "example.invalid", IsTimeout: true}, 3},
	} {
		if got := exitCode(c.err); got != c.want {
			t.Errorf("%s (%v): exit %d, want %d", name, c.err, got, c.want)
		}
	}
}
