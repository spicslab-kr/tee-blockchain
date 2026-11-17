# Recorded TDX quotes

Quotes from a real Intel TDX guest on GCP (`attestamp-tdx-vendor`), used by the tests in `internal/tee/parse/tdx` and
`cmd`. They let the parser, the role adapters and the CLI be tested with no TDX instance running.

| File | Content |
|---|---|
| `gcp-c3-quote.bin` | The first quote, 8000 bytes, as read from ConfigFS TSM `outblob`, from the stock (initrdless) boot |
| `gcp-c3-quote.nonce` | The 64 bytes written to `inblob` (the TD's REPORTDATA), as 128 hex characters and a newline |
| `gcp-c3-opt1-v1-*`, `gcp-c3-opt1-v2-*` | Option 1: a one-file application archive placed before the base initrd (`cpio.AppFirst`), recorded twice with the application changed by one byte |
| `gcp-c3-opt2-v1-*`, `gcp-c3-opt2-v2-*` | Option 2: a squashfs + dm-verity image and its hash tree placed inside the initrd, also `AppFirst`, recorded twice with the image's content changed by one byte |
| `gcp-c3-opt2m-v1-*`, `gcp-c3-opt2m-v2-*` | Option 2 with the image **opened and mounted at boot** (2026-11-12): the same two images, with a base initrd that carries `record/initramfs-hook.sh` and `record/initramfs-local-bottom.sh`. Nonce texts `attestamp-tdx-verity-mounted-v1` and `-v2` |

sha256 of `gcp-c3-quote.bin`: `228da7b4e87f2fd21c7732d91bfe709f5fa81850f8216768b5b1d48fb46b8a4d`

## The application recordings (2026-11-11)

Each has `-quote.bin` (the quote), `-record.txt` (what `record/main.go` printed: nonce text, REPORTDATA, provider,
size, the kernel command line, the boot ID) and `-boot.txt` (the commands and output of the session, copied verbatim:
how the initrd was built, the marker, the kernel's initramfs lines).

- REPORTDATA = SHA-512 of the nonce text `attestamp-tdx-<cell>`, e.g. `attestamp-tdx-opt1-v1`.
- The base initrd was `/boot/initrd.img-7.0.0-1017-gcp`, 50,100,283 bytes (**size mod 4 = 3**), sha256 `4703ec07…b0a2`:
  byte-identical to the one on the SEV-SNP guest.
- On all four boots the kernel logged no unpacking error, and the marker `/dev/attestamp-app-marker` was present,
  naming the application text (Option 1) or `app.img`, `app.hash` and the root hash (Option 2).
- Result, checked by `app_fixture_test.go`: in each pair only RTMR2 differs; MRTD, RTMR0 and RTMR1 are the same in all
  four quotes.

## The mounted recordings (2026-11-12)

The same layout as above. The base initrd was built with `mkinitramfs` while the two scripts in `record/` were
installed (they were removed right after): 50,140,646 bytes, sha256 `66cb9d56…d51e`. On both boots
`/run/attestamp-status` read `ok <root hash>`, `/opt/app-verity` was a read-only squashfs on
`/dev/mapper/attestamp-app` with `dmsetup status` `V` (verified), and the application text was readable from it.
`app_fixture_test.go` checks that MRTD, RTMR0 and RTMR1 are again the same and that RTMR2 differs between v1 and v2 and
from the unmounted Option 2 recordings.

The same identifying information as the first quote: the PCK certificate of the physical machine. `-record.txt` also
holds the root file system's UUID (in the command line) and the boot ID.

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

`record-tdx` made the application recordings of 2026-11-11 and 2026-11-12. To record quotes with an application in the initrd, see
`docs/build/app-fixture-recording.md`.

The quote carries no secret, but it does carry the PCK certificate of the physical machine it was made on. MROWNER is
set by GCP and differs per instance; the parser ignores it.
