package gcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"attestamp/internal/tee/request/auditor"
)

func TestExtractor_PostsNonceAndReturnsToken(t *testing.T) {
	n, _ := auditor.NonceGen()
	var got tokenRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/token" {
			http.Error(w, "wrong route", 404)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = w.Write([]byte("eyJ.header.payload.sig"))
	}))
	defer srv.Close()
	client := srv.Client()
	client.Transport = rewrite{base: srv.URL, rt: client.Transport}
	doc, err := Extractor{Client: client, Audience: "svc"}.Extract(context.Background(), n)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Format != "gcp-cs-jwt" || string(doc.Raw) != "eyJ.header.payload.sig" {
		t.Errorf("doc: %+v", doc)
	}
	if got.Audience != "svc" || got.TokenType != "OIDC" || len(got.Nonces) != 1 || got.Nonces[0] != base64.StdEncoding.EncodeToString(n) {
		t.Errorf("request body: %+v", got)
	}
}

func TestExtractor_LauncherErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nonce too short", http.StatusBadRequest)
	}))
	defer srv.Close()
	client := srv.Client()
	client.Transport = rewrite{base: srv.URL, rt: client.Transport}
	n, _ := auditor.NonceGen()
	_, err := Extractor{Client: client}.Extract(context.Background(), n)
	if err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "nonce too short") {
		t.Fatalf("got %v", err)
	}
}

func TestExtractor_ShortNonce(t *testing.T) {
	if _, err := (Extractor{}).Extract(context.Background(), auditor.Nonce("x")); err == nil {
		t.Fatal("short nonce must be rejected before any request")
	}
}

// A RoundTripper that redirects requests to the test server address
type rewrite struct {
	base string
	rt   http.RoundTripper
}

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u := r.base + req.URL.Path
	nr, err := http.NewRequestWithContext(req.Context(), req.Method, u, req.Body)
	if err != nil {
		return nil, err
	}
	nr.Header = req.Header
	return r.rt.RoundTrip(nr)
}
