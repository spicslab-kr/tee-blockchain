package aws

import (
	"context"
	"errors"
	"io"
	"net"
	"time"
)

// Proxy 는 부모 인스턴스에서 돈다. ln 으로 들어온 연결을 하나씩 엔클레이브 cid:port 로 이어 준다.
// 엔클레이브는 네트워크가 없어 밖에서 vsock 으로만 닿기 때문에 이 다리가 필요하다. ctx 가 끝나면 멈춘다.
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
