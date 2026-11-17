//go:build !linux

package aws

import (
	"errors"
	"net"
)

func ListenVsock(port uint32) (net.Listener, error) {
	return nil, errors.New("vsock is only available on linux")
}

func DialVsock(cid, port uint32) (net.Conn, error) {
	return nil, errors.New("vsock is only available on linux")
}
