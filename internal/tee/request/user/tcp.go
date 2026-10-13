package user

import (
	"context"
	"errors"
	"io"
	"net"
	"time"
)

// TCPRequester connects to teeAddr (host:port), sends a nonce and receives the RA document.
// On Nitro the parent instance bridges TCP and vsock, so every platform is reached over TCP.
type TCPRequester struct {
	Timeout time.Duration // 30s if 0
	Nonces  NonceSource   // DefaultNonceSource if nil
}

// RequestFresh generates and sends a new nonce, and returns that nonce together with the document.
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

// Serve runs inside the TEE. For each connection it reads a nonce, extracts a document with ex and sends it back. It stops when ctx ends.
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

// drain discards the remaining input. Closing with unread bytes can make the response be dropped by an RST.
func drain(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = io.Copy(io.Discard, conn)
}
