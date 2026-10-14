// att-ra-responder is the one binary every role's build layers into the enclave image.
//
// att-vendor and att-auditor are compiled from different source (cmd/vendor and cmd/auditor),
// so a shared responder keeps the image PCR0/PCR2 identical between their independent
// builds of the same Dockerfile.
//
// Inside the enclave only two commands run (see internal/setup/build's entrypoint.sh):
// "lo-up" and "ra-serve --extractor nsm". Neither depends on the role that built the image,
// so this binary carries just those two, built once from one source.
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
	fmt.Fprintln(os.Stderr, `att-ra-responder — runs only inside the enclave. every role's build layers this one binary as is

  ra-serve  --listen tcp:<host:port> | vsock:<port>  --extractor nsm | fixture=<doc>  [--user-data <hex>]
            takes a nonce per request and returns an RA document
  lo-up     brings up loopback. once, before starting the software`)
}

func raServe(args []string) error {
	fs := flag.NewFlagSet("ra-serve", flag.ContinueOnError)
	listen := fs.String("listen", "tcp:127.0.0.1:5000", "tcp:<host:port> or vsock:<port>")
	extractor := fs.String("extractor", "nsm", "nsm or fixture=<doc file>")
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
	case strings.HasPrefix(*extractor, "fixture="):
		fx, err := ra.LoadFixture(awsra.Format, strings.TrimPrefix(*extractor, "fixture="))
		if err != nil {
			return err
		}
		ex = fx
		fmt.Fprintln(os.Stderr, "warning: fixture extractor — the document is not bound to the nonce; test use only")
	default:
		return errors.New("--extractor: nsm or fixture=<doc>")
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
