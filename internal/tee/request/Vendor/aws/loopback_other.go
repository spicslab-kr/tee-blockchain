//go:build !linux

package aws

import "errors"

func LoopbackUp() error { return errors.New("loopback setup is only available on linux") }
