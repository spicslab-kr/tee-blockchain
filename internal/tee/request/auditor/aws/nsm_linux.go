//go:build linux

package aws

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"tb/internal/tee/request/auditor"
)

// Extractor 는 엔클레이브 안에서 /dev/nsm 에 ioctl 로 문서를 요청한다.
type Extractor struct {
	Device   string // 기본 /dev/nsm
	UserData []byte // 문서에 묶을 값. 없으면 nil
}

var _ auditor.Extractor = (*Extractor)(nil)

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

func (e Extractor) Extract(ctx context.Context, nonce auditor.Nonce) (auditor.RADoc, error) {
	if err := ctx.Err(); err != nil {
		return auditor.RADoc{}, err
	}
	dev := e.Device
	if dev == "" {
		dev = "/dev/nsm"
	}
	req, err := encodeAttestationRequest(nonce, e.UserData)
	if err != nil {
		return auditor.RADoc{}, err
	}
	f, err := os.OpenFile(dev, os.O_RDWR, 0)
	if err != nil {
		return auditor.RADoc{}, fmt.Errorf("open %s: %w", dev, err)
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
		return auditor.RADoc{}, fmt.Errorf("nsm ioctl: %w", errno)
	}
	doc, err := decodeAttestationResponse(resp[:msg.response.len])
	if err != nil {
		return auditor.RADoc{}, err
	}
	return auditor.RADoc{Format: Format, Raw: doc}, nil
}
