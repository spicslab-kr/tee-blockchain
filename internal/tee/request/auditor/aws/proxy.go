package aws

import (
	"context"
	"errors"
	"io"
	"net"
	"time"
)

// Proxy runs on the parent instance. It forwards each connection accepted on ln to the enclave at cid:port.
// The enclave has no network and is reachable from outside only over vsock, hence this bridge. It stops when ctx ends.
func Proxy(ctx context.Context, ln net.Listener, cid, port uint32) error {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go bridge(conn, cid, port)
	}
}

func bridge(client net.Conn, cid, port uint32) {
	defer client.Close()
	enclave, err := DialVsock(cid, port)
	if err != nil {
		return
	}
	defer enclave.Close()
	deadline := time.Now().Add(60 * time.Second)
	_ = client.SetDeadline(deadline)
	_ = enclave.SetDeadline(deadline)
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(enclave, client); closeWrite(enclave); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, enclave); closeWrite(client); done <- struct{}{} }()
	<-done
	<-done
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}
