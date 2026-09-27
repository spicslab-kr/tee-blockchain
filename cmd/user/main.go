package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"tb/internal/cli"
	parse "tb/internal/tee/parse/user"
	ra "tb/internal/tee/request/user"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "ra-parse":
		err = raParse(os.Args[2:])
	case "ra-request":
		err = raRequest(os.Args[2:])
	case "lookup":
		_, err = lookup(os.Args[2:])
	case "compare":
		err = compareCmd(os.Args[2:])
	case "verify":
		err = verifyCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		cli.Errorln("tb user: ", err)
		os.Exit(exitCode(err))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `tb user — RA request

  ra-parse    --doc <file> [--nonce <hex|file>] [--root <pem>] [--format aws-nitro] [--out <file>] [--at <RFC3339>]
              verifies the document's signature, certificate chain and nonce, and extracts the measurements (PCR). the output goes straight into register --measurements
  ra-request  --tee <host:port> [--out <file>] [--format <name>] [--timeout 30s]
              generates a nonce, sends it, and saves the RA document. the nonce goes to <file>.nonce.
              with --format, accepts only a document of that format (aws-nitro · gcp-cs-jwt · azure-maa-jwt)
  lookup      --rpc <url> --contract <0x…> (--key <0x…> | --measurements <measurements.json | NAME=hex,…>)
              read one key directly, or find the key(s) for the measurements first and read them all. a User only looks up, never registers
  compare     --a <measurements.json | NAME=hex,…> --rpc <url> --contract <0x…> --key <0x…> [--trusted <0x…,0x…>]
              match the value from ra-parse against the chain registration — value, expiry and trusted signer. exit 1 on failure
  verify      --tee <the Vendor's enclave host:port> --rpc <url> --contract <0x…> [--trusted <0x…,0x…>]
              the trust decision in one go: attest the enclave (T1·T2), find who registered that measurement (C2), check each
              registration (M2). exit 0 if at least one PASSes, 1 with the reason otherwise. no key, no ETH. --nonce pins the nonce`)
}

func raRequest(args []string) error {
	fs := flag.NewFlagSet("ra-request", flag.ContinueOnError)
	tee := fs.String("tee", "", "TEE address host:port")
	out := fs.String("out", "ra-doc.cbor", "file to save the document to")
	format := fs.String("format", "", "expected document format. empty accepts any")
	timeout := fs.Duration("timeout", 30*time.Second, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tee == "" {
		return errors.New("--tee is required")
	}
	nonce, doc, err := ra.TCPRequester{Timeout: *timeout}.RequestFresh(context.Background(), *tee)
	if err != nil {
		return err
	}
	if err := doc.ExpectFormat(*format); err != nil {
		return err
	}
	if err := os.WriteFile(*out, doc.Raw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(*out+".nonce", []byte(hex.EncodeToString(nonce)+"\n"), 0o644); err != nil {
		return err
	}
	if !quiet {
		fmt.Printf("received %s document, %d bytes → %s (nonce → %s.nonce)\n", doc.Format, len(doc.Raw), *out, *out)
	}
	return nil
}

func exitCode(err error) int {
	switch {
	case errors.Is(err, parse.ErrWrongFormat):
		return 4
	case errors.Is(err, parse.ErrSignature), errors.Is(err, parse.ErrChain):
		return 5
	case errors.Is(err, parse.ErrNonce):
		return 6
	case errors.Is(err, parse.ErrBadDocument):
		return 7
	case errors.Is(err, ra.ErrWrongFormat):
		return 4
	case errors.Is(err, ra.ErrExtractFailed):
		return 5
	case errors.Is(err, ra.ErrBadResponse), errors.Is(err, ra.ErrTooLarge):
		return 6
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return 3
	}
	return 1
}
