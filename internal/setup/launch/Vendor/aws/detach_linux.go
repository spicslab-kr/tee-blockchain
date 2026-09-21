//go:build linux

package aws

import "syscall"

// detached — 부모가 끝나도 살아 있게 새 세션으로 띄운다.
func detached() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
