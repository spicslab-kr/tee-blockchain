// att-ra-responder answers attestation requests from inside the TEE. On Nitro it is the one binary every role's build
// layers into the enclave image, so it is measured; on TDX and SEV-SNP it runs from the guest's disk and is not.
//
// att-vendor and att-auditor are compiled from different source (cmd/vendor and cmd/auditor),
// so a shared responder keeps the image PCR0/PCR2 identical between their independent
// builds of the same Dockerfile.
//
// Inside a Nitro enclave only two commands run (see the entrypoint.sh that internal/setup/build/aws/build.go embeds):
// "lo-up" and "ra-serve --extractor nsm". Inside an Intel TDX or AMD SEV-SNP guest it is
// "ra-serve --extractor tdx | snp-initrd | snp-verity --listen tcp:0.0.0.0:<port>". None depends
// on the role that built the image, so this binary carries just these, built once from one source.
//
// It imports attestamp/internal/tee/request/Vendor and its aws package because each role has
// its own copy of Nonce/RADoc/Extractor and no shared one exists. The Vendor copy was picked
// arbitrarily; the Auditor or User copy would behave the same.
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"attestamp/internal/cli"
	ra "attestamp/internal/tee/request/Vendor"
	awsra "attestamp/internal/tee/request/Vendor/aws"
	snpra "attestamp/internal/tee/request/Vendor/snp"
	tdxra "attestamp/internal/tee/request/Vendor/tdx"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "ra-serve":
		err = raServe(os.Args[2:])
	case "lo-up":
		err = awsra.LoopbackUp()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		cli.Errorln("att-ra-responder: ", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `att-ra-responder runs inside the TEE. in a Nitro enclave every role's build layers this one binary as is;
on a TDX or SEV-SNP guest it runs from the guest's disk

  ra-serve  --listen tcp:<host:port> | vsock:<port>
            --extractor nsm | tdx | snp-initrd | snp-verity | fixture=<doc>  [--user-data <hex>]
            takes a nonce per request and returns an RA document
              nsm         AWS Nitro Enclaves (/dev/nsm), inside the enclave, usually with --listen vsock:<port>
              tdx         Intel TDX guest: a quote from ConfigFS TSM                          (intel-tdx)
              snp-initrd  AMD SEV-SNP guest on GCP: vTPM attestation + SNP report             (amd-sev-snp-initrd)
              snp-verity  the same evidence, labelled for the dm-verity option                (amd-sev-snp-verity)
            --listen defaults to tcp:127.0.0.1:5000. A confidential VM is reached over its own network, so give
            --listen tcp:0.0.0.0:5000 explicitly there, and allow port 5000 in the firewall only from the
            verifier's address
  lo-up     brings up loopback in a Nitro enclave. once, before starting the software`)
}

func raServe(args []string) error {
	fs := flag.NewFlagSet("ra-serve", flag.ContinueOnError)
	listen := fs.String("listen", "tcp:127.0.0.1:5000", "tcp:<host:port> or vsock:<port>; listening on every interface (tcp:0.0.0.0:<port>) must be asked for explicitly")
	extractor := fs.String("extractor", "nsm", "nsm, tdx, snp-initrd, snp-verity or fixture=<doc file>")
	userData := fs.String("user-data", "", "value to bind into the document (hex)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var ex ra.Extractor
	switch {
	case *extractor == "nsm":
		ud, err := hex.DecodeString(*userData)
		if err != nil {
			return fmt.Errorf("--user-data: %w", err)
		}
		ex = awsra.Extractor{UserData: ud}
	case *extractor == "tdx":
		ex = tdxra.Extractor{}
	case *extractor == "snp-initrd":
		ex = snpra.Extractor{Format: snpra.FormatInitrd}
	case *extractor == "snp-verity":
		ex = snpra.Extractor{Format: snpra.FormatVerity}
	case strings.HasPrefix(*extractor, "fixture="):
		fx, err := ra.LoadFixture(awsra.Format, strings.TrimPrefix(*extractor, "fixture="))
		if err != nil {
			return err
		}
		ex = fx
		fmt.Fprintln(os.Stderr, "warning: fixture extractor: the document is not bound to the nonce; test use only")
	default:
		return errors.New("--extractor: nsm, tdx, snp-initrd, snp-verity or fixture=<doc>")
	}
	var ln net.Listener
	var err error
	switch {
	case strings.HasPrefix(*listen, "tcp:"):
		ln, err = net.Listen("tcp", strings.TrimPrefix(*listen, "tcp:"))
	case strings.HasPrefix(*listen, "vsock:"):
		var port uint64
		port, err = strconv.ParseUint(strings.TrimPrefix(*listen, "vsock:"), 10, 32)
		if err == nil {
			ln, err = awsra.ListenVsock(uint32(port))
		}
	default:
		return errors.New("--listen: tcp:<host:port> or vsock:<port>")
	}
	if err != nil {
		return err
	}
	fmt.Printf("ra-serve listening on %s\n", ln.Addr())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return ra.Serve(ctx, ln, ex)
}
