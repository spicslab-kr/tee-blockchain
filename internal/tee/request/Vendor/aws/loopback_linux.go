//go:build linux

package aws

import (
	"encoding/binary"
	"syscall"
	"unsafe"
)

// LoopbackUp brings up the lo interface. An enclave boots with lo down, so software that binds to 127.0.0.1 would fail.
func LoopbackUp() error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	var ifr [40]byte // struct ifreq: 16-byte name + value
	copy(ifr[:], "lo")
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.SIOCGIFFLAGS, uintptr(unsafe.Pointer(&ifr[0]))); errno != 0 {
		return errno
	}
	flags := binary.NativeEndian.Uint16(ifr[16:18]) | syscall.IFF_UP | syscall.IFF_RUNNING
	binary.NativeEndian.PutUint16(ifr[16:18], flags)
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.SIOCSIFFLAGS, uintptr(unsafe.Pointer(&ifr[0]))); errno != 0 {
		return errno
	}
	return nil
}
