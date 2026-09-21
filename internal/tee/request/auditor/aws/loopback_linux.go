//go:build linux

package aws

import (
	"encoding/binary"
	"syscall"
	"unsafe"
)

// LoopbackUp 은 lo 인터페이스를 올린다. 엔클레이브는 부팅 직후 lo 가 내려가 있어 127.0.0.1 에 붙는 소프트웨어가 실패한다.
func LoopbackUp() error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	var ifr [40]byte // struct ifreq: 이름 16 바이트 + 값
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
