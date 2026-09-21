package vendor

import (
	"context"
	"errors"
	"io"
	"net"
	"time"
)

// TCPRequester 는 teeAddr(host:port)로 접속해 난수를 보내고 RA 문서를 받는다.
// Nitro 는 부모 인스턴스가 TCP↔vsock 을 이어 주므로 어느 플랫폼이든 TCP 로 부른다.
type TCPRequester struct {
	Timeout time.Duration // 0 이면 30s
	Nonces  NonceSource   // nil 이면 DefaultNonceSource
}

// RequestFresh 는 난수를 새로 만들어 보내고, 문서와 함께 그 난수를 돌려준다.
func (q TCPRequester) RequestFresh(ctx context.Context, teeAddr string) (Nonce, RADoc, error) {
	src := q.Nonces
	if src == nil {
		src = DefaultNonceSource
	}
	n, err := src.Generate(ctx)
	if err != nil {
		return nil, RADoc{}, err
	}
	doc, err := q.Request(ctx, n, teeAddr)
	return n, doc, err
}

var _ Requester = (*TCPRequester)(nil)

func (q TCPRequester) Request(ctx context.Context, nonce Nonce, teeAddr string) (RADoc, error) {
	if err := nonce.Validate(); err != nil {
		return RADoc{}, err
	}
	to := q.Timeout
	if to == 0 {
		to = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", teeAddr)
	if err != nil {
		return RADoc{}, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	if err := writeRequest(conn, nonce); err != nil {
		return RADoc{}, err
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
	return readResponse(conn)
}

// Serve 는 TEE 안에서 돈다. 연결마다 난수를 읽어 ex 로 문서를 뽑고 돌려준다. ctx 가 끝나면 멈춘다.
func Serve(ctx context.Context, ln net.Listener, ex Extractor) error {
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
		go handle(ctx, conn, ex)
	}
}

func handle(ctx context.Context, conn net.Conn, ex Extractor) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	nonce, err := readRequest(conn)
	if err != nil {
		_ = writeResponse(conn, RADoc{}, err)
		drain(conn)
		return
	}
	doc, err := ex.Extract(ctx, nonce)
	_ = writeResponse(conn, doc, err)
	drain(conn)
}

// drain — 남은 입력을 비운다. 읽지 않은 바이트가 있는 채로 닫으면 응답이 RST 로 버려질 수 있다.
func drain(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = io.Copy(io.Discard, conn)
}
