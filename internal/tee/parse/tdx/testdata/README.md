# Recorded TDX quote

One quote from a real Intel TDX guest, used by every test in `internal/tee/parse/tdx`. It lets the parser, the
role adapters and the CLI be tested with no TDX instance running.

| File | Content |
|---|---|
| `gcp-c3-quote.bin` | The raw quote, 8000 bytes, as read from ConfigFS TSM `outblob` |
| `gcp-c3-quote.nonce` | The 64 bytes written to `inblob` (the TD's REPORTDATA), as 128 hex characters and a newline |

sha256 of `gcp-c3-quote.bin`: `228da7b4e87f2fd21c7732d91bfe709f5fa81850f8216768b5b1d48fb46b8a4d`

## Where it came from

| | |
|---|---|
| Cloud | Google Cloud, zone us-central1-a, Confidential VM type TDX |
| Machine type | `c3-standard-4` |
| OS | Ubuntu 24.04.5 LTS |
| Kernel | `7.0.0-1017-gcp` |
| Quote made | 2026-11-10, 13:43 UTC (file time on the instance) |
| Copied here | 2026-11-10, 14:51 UTC |
| Interface | ConfigFS TSM, `/sys/kernel/config/tsm/report`, `provider` = `tdx_guest` |

Header, read little endian: version 4, attestation key type 2 (ECDSA P-256), TEE type `0x81` (TDX).

## The nonce

REPORTDATA is SHA-512 of the ASCII string `attestamp-tdx-fixture-0001`, with no trailing newline:

```
printf %s attestamp-tdx-fixture-0001 | sha512sum
dfa2ff7751a4dde2e7f7cd9d227a9028bc03dfafeec073a7cac5bb72e0b70555d8bd8a96dfa33aece35bbb5723b7bdca7622cd832098ae289c974100bba21892
```

A fixed, documented value rather than a random one, so anyone can check that the file was not swapped.

## Recording another one

On a TDX guest with ConfigFS TSM (Linux 6.7 or later):

```
sudo mkdir /sys/kernel/config/tsm/report/q
cat /sys/kernel/config/tsm/report/q/provider            # must print tdx_guest
printf %s attestamp-tdx-fixture-0001 | sha512sum | cut -c1-128 | xxd -r -p \
  | sudo tee /sys/kernel/config/tsm/report/q/inblob >/dev/null
sudo cat /sys/kernel/config/tsm/report/q/outblob > quote.bin
sudo rmdir /sys/kernel/config/tsm/report/q
```

Or with the recorder in `record/main.go`, which does the same with only the standard library, checks that the quote
carries the REPORTDATA written, and also saves the kernel command line and boot ID. It is skipped by `go build ./...`;
build it by name from the repository root and run it as root on the guest:

```
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o record-tdx ./internal/tee/parse/tdx/testdata/record
sudo ./record-tdx -out fx -nonce-text attestamp-tdx-fixture-0001
```

`record-tdx` has not been run on a guest yet (2026-11-11). To record quotes with an application in the initrd, see
`docs/plan/app-fixture-recording.md`.

The quote carries no secret, but it does carry the PCK certificate of the physical machine it was made on. MROWNER is
set by GCP and differs per instance; the parser ignores it.
