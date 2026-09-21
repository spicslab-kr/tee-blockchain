package user

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestNonceGen(t *testing.T) {
	a, err := NonceGen()
	if err != nil || len(a) != NonceSize {
		t.Fatalf("nonce: %v %d", err, len(a))
	}
	b, _ := NonceGen()
	if bytes.Equal(a, b) {
		t.Fatal("two nonces must differ")
	}
	if err := Nonce(make([]byte, 16)).Validate(); err == nil {
		t.Fatal("short nonce must be rejected")
	}
}

type fixedSource struct{ n Nonce }

func (f fixedSource) Generate(context.Context) (Nonce, error) { return f.n, nil }

func TestNonceSource_Injected(t *testing.T) {
	want := Nonce(bytes.Repeat([]byte{0xAB}, NonceSize))
	ex := &fakeExtractor{platform: "aws-nitro"}
	addr := serve(t, ex)
	n, doc, err := TCPRequester{Nonces: fixedSource{want}}.RequestFresh(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(n, want) || !bytes.Equal(ex.seen, want) {
		t.Fatal("requester must use the injected nonce source")
	}
	if !bytes.HasSuffix(doc.Raw, want) {
		t.Fatal("document must be bound to that nonce")
	}
	// 기본은 CryptoRand
	if _, ok := DefaultNonceSource.(CryptoRand); !ok {
		t.Fatalf("default source = %T", DefaultNonceSource)
	}
}

func TestWire_RoundTrip(t *testing.T) {
	n, _ := NonceGen()
	var buf bytes.Buffer
	if err := writeRequest(&buf, n); err != nil {
		t.Fatal(err)
	}
	got, err := readRequest(&buf)
	if err != nil || !bytes.Equal(got, n) {
		t.Fatalf("request round trip: %v", err)
	}
	doc := RADoc{Format: "aws-nitro", Raw: bytes.Repeat([]byte{0xd2}, 4000)}
	buf.Reset()
	if err := writeResponse(&buf, doc, nil); err != nil {
		t.Fatal(err)
	}
	back, err := readResponse(&buf)
	if err != nil || back.Format != "aws-nitro" || !bytes.Equal(back.Raw, doc.Raw) {
		t.Fatalf("response round trip: %v", err)
	}
}

func TestWire_ErrorStatus(t *testing.T) {
	var buf bytes.Buffer
	_ = writeResponse(&buf, RADoc{Format: "aws-nitro"}, errors.New("nsm busy"))
	_, err := readResponse(&buf)
	if !errors.Is(err, ErrExtractFailed) || !strings.Contains(err.Error(), "nsm busy") {
		t.Fatalf("want ErrExtractFailed with message, got %v", err)
	}
}

func TestWire_Rejects(t *testing.T) {
	if _, err := readRequest(bytes.NewReader([]byte("XXXX\x01"))); !errors.Is(err, ErrBadRequest) {
		t.Errorf("bad magic: %v", err)
	}
	if _, err := readRequest(bytes.NewReader([]byte("TBRA\x01short"))); !errors.Is(err, ErrBadRequest) {
		t.Errorf("truncated: %v", err)
	}
	var buf bytes.Buffer
	if err := writeResponse(&buf, RADoc{Format: "aws-nitro", Raw: make([]byte, MaxDocSize+1)}, nil); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversize write: %v", err)
	}
	// 길이 필드만 크게 조작
	hdr := append([]byte("TBRA\x01\x00\x03aws"), 0xff, 0xff, 0xff, 0xff)
	if _, err := readResponse(bytes.NewReader(hdr)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversize read: %v", err)
	}
	if err := writeRequest(&buf, Nonce("short")); err == nil {
		t.Error("short nonce must not be written")
	}
}

type fakeExtractor struct {
	platform string
	err      error
	seen     Nonce
}

func (f *fakeExtractor) Extract(_ context.Context, n Nonce) (RADoc, error) {
	f.seen = n
	if f.err != nil {
		return RADoc{}, f.err
	}
	return RADoc{Format: f.platform, Raw: append([]byte("doc:"), n...)}, nil
}

func serve(t *testing.T, ex Extractor) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = Serve(ctx, ln, ex) }()
	return ln.Addr().String()
}

func TestTCP_RequestAndServe(t *testing.T) {
	ex := &fakeExtractor{platform: "aws-nitro"}
	addr := serve(t, ex)
	n, _ := NonceGen()
	doc, err := TCPRequester{}.Request(context.Background(), n, addr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ex.seen, n) {
		t.Error("extractor must receive the exact nonce")
	}
	if doc.Format != "aws-nitro" || !bytes.HasPrefix(doc.Raw, []byte("doc:")) {
		t.Errorf("doc: %+v", doc)
	}
}

func TestTCP_ExtractorErrorReachesRequester(t *testing.T) {
	addr := serve(t, &fakeExtractor{err: errors.New("device missing")})
	n, _ := NonceGen()
	_, err := TCPRequester{}.Request(context.Background(), n, addr)
	if !errors.Is(err, ErrExtractFailed) || !strings.Contains(err.Error(), "device missing") {
		t.Fatalf("got %v", err)
	}
}

func TestTCP_BadRequestAnswered(t *testing.T) {
	addr := serve(t, &fakeExtractor{platform: "aws-nitro"})
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("GARBAGE"))
	_ = conn.(*net.TCPConn).CloseWrite()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = readResponse(conn)
	if !errors.Is(err, ErrExtractFailed) || !strings.Contains(err.Error(), "malformed request") {
		t.Fatalf("server must answer a malformed request with an error frame, got %v", err)
	}
}

func TestTCP_ShortNonceRefusedLocally(t *testing.T) {
	if _, err := (TCPRequester{}).Request(context.Background(), Nonce("x"), "127.0.0.1:1"); err == nil {
		t.Fatal("must not dial with a bad nonce")
	}
}

func TestTCP_Timeout(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			time.Sleep(2 * time.Second)
			c.Close()
		}
	}()
	n, _ := NonceGen()
	_, err := TCPRequester{Timeout: 200 * time.Millisecond}.Request(context.Background(), n, ln.Addr().String())
	if err == nil {
		t.Fatal("must time out")
	}
}

func TestRADoc_ExpectFormat(t *testing.T) {
	doc := RADoc{Format: "aws-nitro"}
	if err := doc.ExpectFormat("aws-nitro"); err != nil {
		t.Fatal(err)
	}
	if err := doc.ExpectFormat(""); err != nil {
		t.Fatal("empty want must accept any format:", err)
	}
	if err := doc.ExpectFormat("gcp-cs-jwt"); !errors.Is(err, ErrWrongFormat) {
		t.Fatalf("want ErrWrongFormat, got %v", err)
	}
}
