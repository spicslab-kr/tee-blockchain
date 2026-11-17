//go:build linux

package aws

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	nsmIoctlMagic  = 0x0A
	nsmMaxResponse = 0x3000
)

type iovec struct {
	base *byte
	len  uint64
}

type nsmMessage struct {
	request  iovec
	response iovec
}

// Attest requests a document from the NSM device (device; empty means /dev/nsm) with an ioctl, from inside the
// enclave, and returns the raw COSE_Sign1 document. The caller checks the nonce first.
func Attest(ctx context.Context, device string, nonce, userData []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dev := device
	if dev == "" {
		dev = "/dev/nsm"
	}
	req, err := encodeAttestationRequest(nonce, userData)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(dev, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dev, err)
	}
	defer f.Close()
	resp := make([]byte, nsmMaxResponse)
	msg := nsmMessage{
		request:  iovec{base: &req[0], len: uint64(len(req))},
		response: iovec{base: &resp[0], len: uint64(len(resp))},
	}
	// _IOWR(0x0A, 0, struct nsm_message)
	code := uintptr(3<<30 | uintptr(unsafe.Sizeof(msg))<<16 | nsmIoctlMagic<<8 | 0)
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), code, uintptr(unsafe.Pointer(&msg))); errno != 0 {
		return nil, fmt.Errorf("nsm ioctl: %w", errno)
	}
	return decodeAttestationResponse(resp[:msg.response.len])
}
