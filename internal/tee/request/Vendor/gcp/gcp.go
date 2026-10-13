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

	"attestamp/internal/tee/request/Vendor"
)

const Format = "gcp-cs-jwt"

// Extractor asks the Confidential Space launcher socket for a token. The JWT that comes back is the RA document.
type Extractor struct {
	Socket   string // default /run/container_launcher/teeserver.sock
	Audience string // default att
	// If Client is set it is used instead of Socket (for tests)
	Client *http.Client
}

var _ vendor.Extractor = (*Extractor)(nil)

const DefaultSocket = "/run/container_launcher/teeserver.sock"

type tokenRequest struct {
	Audience  string   `json:"audience"`
	Nonces    []string `json:"nonces"`
	TokenType string   `json:"token_type"`
}

func (e Extractor) Extract(ctx context.Context, nonce vendor.Nonce) (vendor.RADoc, error) {
	if err := nonce.Validate(); err != nil {
		return vendor.RADoc{}, err
	}
	aud := e.Audience
	if aud == "" {
		aud = "att"
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
		return vendor.RADoc{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return vendor.RADoc{}, fmt.Errorf("launcher: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, vendor.MaxDocSize))
	if err != nil {
		return vendor.RADoc{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return vendor.RADoc{}, fmt.Errorf("launcher: %s: %s", resp.Status, bytes.TrimSpace(raw))
	}
	if len(raw) == 0 {
		return vendor.RADoc{}, fmt.Errorf("launcher: empty token")
	}
	return vendor.RADoc{Format: Format, Raw: raw}, nil
}
