# Recorded SEV-SNP evidence

Two evidence pairs from one real AMD SEV-SNP guest on GCP, used by the tests in `internal/tee/parse/snp` and
`cmd/snp_e2e_test.go`. They let the parser be written and tested with no instance running.

| Recording | Boot | Binding | Used for |
|---|---|---|---|
| `gcp-n2d-*` | with an initrd | REPORT_DATA = **SHA3-512** of the vTPM attestation | the evidence the parser accepts: every value of `amd-sev-snp-initrd` is asserted |
| `gcp-n2d-initrdless-*` | without an initrd (stock GCP Ubuntu) | REPORT_DATA = **SHA-512** of the vTPM attestation | evidence the parser must refuse, twice over: the binding (`VerifySig`) and the missing initrd (`Parse`) |

Each recording has four files:

| File | Content |
|---|---|
| `<prefix>-att.pb` | vTPM attestation: go-tpm-tools `attest.Attestation`, binary protobuf. Quotes over every PCR bank signed by the GCE attestation key (RSA), with the AK certificate, its intermediates and the TCG event log |
| `<prefix>-report.bin` | SNP attestation report, 1,184 bytes, raw from ConfigFS TSM `outblob` |
| `<prefix>-auxblob.bin` | The certificate table the host returned with the report (ConfigFS TSM `auxblob`), 4,763 bytes |
| `<prefix>-record.txt` | What `record` printed: nonce, REPORT_DATA, sizes |

`record/main.go` is the program that recorded them. It now writes the SHA3-512 binding; the initrdless recording was
made by its earlier version, which used `crypto/sha512` in the same place.

## Where it came from

| | |
|---|---|
| Cloud | Google Cloud, zone us-central1-a, Confidential VM type SEV_SNP |
| Instance | `attestamp-snp-vendor`, machine type `n2d-standard-2` (AMD Milan) |
| OS | Ubuntu 24.04.5 LTS |
| Kernel | `7.0.0-1017-gcp` |
| Interfaces | `/dev/tpmrm0` for the vTPM; ConfigFS TSM, `/sys/kernel/config/tsm/report`, `provider` = `sev_guest`, for the report |

| | `gcp-n2d-initrdless-*` | `gcp-n2d-*` |
|---|---|---|
| Recorded | 2026-11-10, 17:23 UTC | 2026-11-11, between 04:30 and 04:32 UTC |
| Boot change | none (stock image) | `GRUB_FORCE_PARTUUID` commented out in `/etc/default/grub.d/40-force-partuuid.cfg`, `update-grub`, reboot (see `docs/build/sev-snp.md`) |
| Kernel command line (from the event log) | `/vmlinuz-7.0.0-1017-gcp root=PARTUUID=27c09ec4-… ro console=ttyS0,115200 panic=-1` | `/vmlinuz-7.0.0-1017-gcp root=UUID=9d4d258d-… ro console=ttyS0,115200` |
| `att.pb` | 63,957 bytes | 61,835 bytes |
| sha256 of `att.pb` | | `7def9a1bdab73bb29a8cd5cc9accb5489735b63e256a8c6e4077b2326ec81527` |
| sha256 of `report.bin` | | `ee0152d64b9677369518329b73c163c26d6e571b15d96597ae40f254c87e4d0d` |

Read from both reports: VERSION 5, POLICY `0x30000` (DEBUG, bit 19, is clear), SIGNING_KEY 0 (signed by the VCEK).
Both certificate tables are byte-identical (sha256 `45fe2ec1…c993`): same host chip, same TCB.

Checked outside the parser for `gcp-n2d-*`, on the instance or with tools other than Go:

| Value | Check |
|---|---|
| INITRD `4703ec07…b0a2` | `sha256sum /boot/initrd.img-7.0.0-1017-gcp` on the instance |
| KERNEL `3cf8c2c3…9adb` | `sha256sum /boot/vmlinuz-7.0.0-1017-gcp` on the instance |
| CMDLINE `80dfd7d3…3de0` | `printf '%s' '<command line above>' \| sha256sum` |
| REPORT_DATA `82ded258…142e` | `openssl dgst -sha3-512 gcp-n2d-att.pb`, on Windows and on the instance (OpenSSL 3.0.13) |

## The nonce and the binding

- vTPM nonce, both recordings = SHA-256 of the ASCII string `attestamp-snp-fixture-0001` =
  `d529879465187801d4b9ffc1c7d0848a2f028e5df9ab46d3eb7a9d5cdb7f925f` (32 bytes, the `tee/request` nonce size).
- `gcp-n2d-*`: REPORT_DATA = SHA3-512 of the whole `gcp-n2d-att.pb` file = `82ded258…142e` (full value in
  `gcp-n2d-record.txt`).
- `gcp-n2d-initrdless-*`: REPORT_DATA = SHA-512 of the whole `gcp-n2d-initrdless-att.pb` file = `29377722…4fb7da`.

The binding changed from SHA-512 to SHA3-512 on 2026-11-11. SHA-512 was what the 2026-11 SEV-SNP test scripts used
(`vendor/vendor_evidence.sh`); those scripts were changed to SHA3-512 the same day.

## Identifying information in these files

These files identify the GCP project and the instance they came from. They are committed anyway, for the same
reason `internal/tee/parse/aws/testdata/fixture-my-web.cbor` carries an EC2 instance ID: without a committed fixture
the parser tests cannot run in CI or for anyone else, and testing without hardware is what this project relies on.
The values cannot be removed without breaking the signatures over them. Both recordings come from the same instance,
so the second one discloses nothing the first did not, except the root file system's UUID.

| File | Contains |
|---|---|
| `*-att.pb` | In the Google-signed AK certificate: the GCP **project ID**, the **zone**, the **instance name** (`attestamp-snp-vendor`) and the numeric **instance ID**, found as text in the subject. The certificate's GCE instance-information extension (OID 1.3.6.1.4.1.11129.2.1.21) also carries the **project number** and the **instance ID** as integers; go-tpm-tools reads them as `gceInstanceInfo` (`server/verify.go`). The event log describes the boot of that instance, including the boot disk's file-system UUID and, on the kernel command line, the root partition's PARTUUID (initrdless) or the root file system's UUID (`gcp-n2d-*`). |
| `*-report.bin` | `CHIP_ID`, which identifies the physical AMD processor of the Google host, and that host's TCB versions. No project or instance identifiers (searched for in the initrdless recording). |
| `*-auxblob.bin` | The VCEK certificate, which carries the same chip ID and TCB. No project or instance identifiers (searched for). |

None of them contains a private key, a credential or an IP address.

## Recording another one

`record/main.go` is outside the module build (Go ignores `testdata`). To build it:

```
mkdir /tmp/record && cp record/main.go /tmp/record && cd /tmp/record
go mod init record
go get github.com/google/go-tpm-tools@v0.4.11 github.com/google/go-tpm@v0.9.8
go mod tidy
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o record .
```

`crypto/sha3` needs Go 1.24 or later; the pinned go1.26.1 has it.

Then on an SEV-SNP guest booted with an initrd: `sudo ./record -out fx -nonce-text attestamp-snp-fixture-0001`.
