package gcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"tb/internal/tee/request/auditor"
)

const Format = "gcp-cs-jwt"

// Extractor 는 Confidential Space 런처 소켓에 토큰을 요청한다. 돌아오는 JWT 가 RA 문서다.
type Extractor struct {
	Socket   string // 기본 /run/container_launcher/teeserver.sock
	Audience string // 기본 tb
	// Client 가 있으면 Socket 대신 쓴다 (테스트용)
	Client *http.Client
}

var _ auditor.Extractor = (*Extractor)(nil)

const DefaultSocket = "/run/container_launcher/teeserver.sock"

type tokenRequest struct {
	Audience  string   `json:"audience"`
	Nonces    []string `json:"nonces"`
	TokenType string   `json:"token_type"`
}

func (e Extractor) Extract(ctx context.Context, nonce auditor.Nonce) (auditor.RADoc, error) {
	if err := nonce.Validate(); err != nil {
		return auditor.RADoc{}, err
	}
	aud := e.Audience
	if aud == "" {
		aud = "tb"
	}
	body, _ := json.Marshal(tokenRequest{Audience: aud, Nonces: []string{base64.StdEncoding.EncodeToString(nonce)}, TokenType: "OIDC"})
	client := e.Client
	if client == nil {
		sock := e.Socket
		if sock == "" {
			sock = DefaultSocket
		}
		client = &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		}}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://launcher/v1/token", bytes.NewReader(body))
	if err != nil {
		return auditor.RADoc{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return auditor.RADoc{}, fmt.Errorf("launcher: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, auditor.MaxDocSize))
	if err != nil {
		return auditor.RADoc{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return auditor.RADoc{}, fmt.Errorf("launcher: %s: %s", resp.Status, bytes.TrimSpace(raw))
	}
	if len(raw) == 0 {
		return auditor.RADoc{}, fmt.Errorf("launcher: empty token")
	}
	return auditor.RADoc{Format: Format, Raw: raw}, nil
}
