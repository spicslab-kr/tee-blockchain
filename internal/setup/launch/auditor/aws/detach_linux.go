//go:build linux

package aws

import "syscall"

// detached starts the process in a new session so it stays alive after the parent exits.
func detached() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
