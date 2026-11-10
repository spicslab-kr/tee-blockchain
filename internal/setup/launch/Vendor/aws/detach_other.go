//go:build !linux

package aws

import "syscall"

func detached() *syscall.SysProcAttr { return nil }
