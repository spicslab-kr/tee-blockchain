package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	p "tb/internal/tee/parse/user"
	ra "tb/internal/tee/request/user"
)

// raParse — 문서를 읽어 서명 · 체인 · 난수를 확인하고 측정값을 낸다. 결과 파일은 nitro-cli 의 measurements.json 과 같은 모양이라
// 그대로 register --measurements 에 넣을 수 있다.
func raParse(args []string) error {
	fs := flag.NewFlagSet("ra-parse", flag.ContinueOnError)
	docPath := fs.String("doc", "", "RA document file (as saved by ra-request)")
	noncePath := fs.String("nonce", "", "the nonce that was sent, as hex or a file. empty uses <doc>.nonce; if that is missing too, the nonce check is skipped")
	format := fs.String("format", p.AWSFormat, "document format")
	rootPath := fs.String("root", "", "platform verification key (root certificate PEM). empty uses the built-in one")
	out := fs.String("out", "", "file to save the measurements to (default: <doc>.measurements.json)")
	at := fs.String("at", "", "time at which to judge the certificate chain (RFC3339). empty uses the document's time")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *docPath == "" {
		return errors.New("--doc is required")
	}
	raw, err := os.ReadFile(*docPath)
	if err != nil {
		return err
	}
	doc := ra.RADoc{Format: *format, Raw: raw}

	var parser interface {
		p.Parser
		Info(ra.RADoc) (p.Info, error)
	}
	switch *format {
	case p.AWSFormat:
		a := p.AWSParser{}
		if *at != "" {
			if a.Now, err = time.Parse(time.RFC3339, *at); err != nil {
				return fmt.Errorf("--at: %w", err)
			}
		}
		parser = a
	default:
		return fmt.Errorf("%w: no parser for %q", p.ErrWrongFormat, *format)
	}

	var root []byte
	if *rootPath != "" {
		if root, err = os.ReadFile(*rootPath); err != nil {
			return err
		}
	}
	if err := parser.VerifySig(context.Background(), doc, root); err != nil {
		return err
	}
	fmt.Println("signature  ok (chain to platform root)")

	nonce, err := loadNonce(*noncePath, *docPath)
	if err != nil {
		return err
	}
	if nonce != nil {
		if err := parser.CheckNonce(doc, nonce); err != nil {
			return err
		}
		fmt.Println("nonce      ok")
	} else {
		fmt.Println("nonce      not checked (none given)")
	}

	info, err := parser.Info(doc)
	if err != nil {
		return err
	}
	fmt.Printf("module     %s\ntimestamp  %s\ndebug      %v\n", info.Module, info.Timestamp.Format(time.RFC3339), info.Debug)
	ms, err := parser.Parse(doc)
	if err != nil {
		return err
	}
	values := map[string]string{}
	for _, m := range ms {
		values[m.Name] = hex.EncodeToString(m.Value)
		fmt.Printf("%-10s %s\n", m.Name, values[m.Name])
	}
	if *out == "" {
		*out = *docPath + ".measurements.json"
	}
	b, _ := json.MarshalIndent(map[string]any{"Format": *format, "Measurements": values}, "", "  ")
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("→ %s\n", *out)
	return nil
}

// loadNonce — hex 문자열, hex 가 든 파일, 또는 <doc>.nonce. 셋 다 없으면 nil.
func loadNonce(src, docPath string) (ra.Nonce, error) {
	if src == "" {
		b, err := os.ReadFile(docPath + ".nonce")
		if err != nil {
			return nil, nil
		}
		src = strings.TrimSpace(string(b))
	} else if b, err := os.ReadFile(src); err == nil {
		src = strings.TrimSpace(string(b))
	}
	n, err := hex.DecodeString(strings.TrimPrefix(src, "0x"))
	if err != nil {
		return nil, fmt.Errorf("--nonce: %w", err)
	}
	return n, nil
}
