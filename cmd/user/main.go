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

	"attestamp/internal/cli"
	parse "attestamp/internal/tee/parse/user"
	ra "attestamp/internal/tee/request/user"
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
		cli.Errorln("att-user: ", err)
		os.Exit(exitCode(err))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `att-user — RA request

  ra-parse    --doc <file> [--nonce <hex|file>] [--root <pem>] [--format aws-nitro] [--out-file <file>] [--at <RFC3339>]
              verifies the document's signature, certificate chain and nonce, and extracts the measurements (PCR). the output goes straight into register --measurements
  ra-request  --tee-addr <host:port> [--out-file <file>] [--format <name>] [--timeout 30s]
              generates a nonce, sends it, and saves the RA document. the nonce goes to <file>.nonce.
              with --format, accepts only a document of that format (aws-nitro · gcp-cs-jwt · azure-maa-jwt)
  lookup      --rpc <url> --registry <0x…> (--record-key <0x…> | --measurements <measurements.json | NAME=hex,…>)
              read one record key directly, or find the record key(s) for the measurements first and read them all. a User only looks up, never registers
  compare     --attested <measurements.json | NAME=hex,…> --rpc <url> --registry <0x…> --record-key <0x…> [--trusted-approvers <0x…,0x…>]
              match the value from ra-parse against the chain registration — value, expiry and approver. exit 1 on failure
  compare     --attested <measurements.json | NAME=hex,…> --expected <same shape>
              match two sets of measurements as they are. prints PASS/FAIL with the reasons
  verify      --tee-addr <the Vendor's enclave host:port> --rpc <url> --registry <0x…> [--registry <0x…> …] [--trusted-approvers <0x…,0x…>]
              the trust decision in one go: request and verify the enclave's attestation document, find who registered that measurement, check each
              registration. exit 0 if at least one PASSes, 1 with the reason otherwise. no key, no ETH. --nonce pins the nonce.
              several --registry (e.g. the Vendor's and the Auditor's registry): each is shown side by side and every one must vouch`)
}

func raRequest(args []string) error {
	fs := flag.NewFlagSet("ra-request", flag.ContinueOnError)
	teeAddr := fs.String("tee-addr", "", "TEE address host:port")
	outFile := fs.String("out-file", "ra-doc.cbor", "file to save the document to")
	format := fs.String("format", "", "expected document format. empty accepts any")
	timeout := fs.Duration("timeout", 30*time.Second, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *teeAddr == "" {
		return errors.New("--tee-addr is required")
	}
	nonce, doc, err := ra.TCPRequester{Timeout: *timeout}.RequestFresh(context.Background(), *teeAddr)
	if err != nil {
		return err
	}
	if err := doc.ExpectFormat(*format); err != nil {
		return err
	}
	if err := os.WriteFile(*outFile, doc.Raw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(*outFile+".nonce", []byte(hex.EncodeToString(nonce)+"\n"), 0o644); err != nil {
		return err
	}
	if !quiet {
		fmt.Printf("received %s document, %d bytes → %s (nonce → %s.nonce)\n", doc.Format, len(doc.Raw), *outFile, *outFile)
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
