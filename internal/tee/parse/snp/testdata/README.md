# Recorded SEV-SNP evidence

Evidence from one real AMD SEV-SNP guest on GCP (`attestamp-snp-vendor`), used by the tests in
`internal/tee/parse/snp` and `cmd/snp_e2e_test.go`. It lets the parser be written and tested with no instance running.

| Recording | Boot | Binding | Used for |
|---|---|---|---|
| `gcp-n2d-*` | with the distribution initrd | REPORT_DATA = **SHA3-512** of the vTPM attestation | the evidence the parser accepts: every value of `amd-sev-snp-initrd` is asserted |
| `gcp-n2d-initrdless-*` | without an initrd (stock GCP Ubuntu) | REPORT_DATA = **SHA-512** of the vTPM attestation | evidence the parser must refuse, twice over: the binding (`VerifySig`) and the missing initrd (`Parse`) |
| `gcp-n2d-opt1-v1-*`, `gcp-n2d-opt1-v2-*` | Option 1, application archive appended **without padding**: the kernel discarded it | SHA3-512 | T5 pair, and the record that an unpadded append loses the application |
| `gcp-n2d-opt1p-v1-*`, `gcp-n2d-opt1p-v2-*` | Option 1, application archive appended after the base **zero-padded to 4 bytes**: unpacked | SHA3-512 | T5 pair (`amd-sev-snp-initrd`) |
| `gcp-n2d-opt2-v1-*`, `gcp-n2d-opt2-v2-*` | Option 2, squashfs + dm-verity image on disk, verifying initrd, PCR12 extended | SHA3-512 | T5 pair (`amd-sev-snp-verity`); the first real evidence with a non-zero PCR12 |

In each `v1`/`v2` pair the application differs by one byte (`attestamp app v1` → `attestamp app v2`) and nothing
else. The procedure is `docs/plan/app-fixture-recording.md`.

## The two Option 1 pairs, side by side

Both were recorded on 2026-11-11 on the same instance, from the same base initrd (`/boot/initrd.img-7.0.0-1017-gcp`,
50,100,283 bytes, **length mod 4 = 3**), with the same GRUB entry and command line.

| | `gcp-n2d-opt1-*` (unpadded) | `gcp-n2d-opt1p-*` (padded) |
|---|---|---|
| how the initrd was made | base followed directly by the application archive | base zero-padded to 50,100,284 bytes (`truncate -s %4`), then the archive: the bytes `cpio.Join(AppLast)` writes since commit `3c57d4c` |
| kernel log (`dmesg`) | `Initramfs unpacking failed: invalid magic at start of compressed archive` | no such line |
| marker `/dev/attestamp-app-marker` | **absent** | present: `file app 17` / `attestamp app v1` (v2) |
| did the application run | **no** | yes |
| INITRD v1 → v2 | `bc566282…1153` → `1e3d844a…d511` | `a4504052…58bf` → `332163d0…6006` |
| boot log | `gcp-n2d-opt1-v1-boot.txt`, `gcp-n2d-opt1-v2-boot.txt` | `gcp-n2d-opt1p-v1-boot.txt`, `gcp-n2d-opt1p-v2-boot.txt` |

These two pairs cannot be told apart from the evidence alone. Both carry a well-formed quote and an INITRD that
changes with the application. The difference - that in the unpadded pair the kernel discarded the archive and the
application never ran - is visible only in the boot log and in the absence of the marker. That is the point of keeping
the unpadded pair.

Each `*-boot.txt` holds the commands and output from the session, copied verbatim: how the initrd was built, the boot
ID, the command line, the marker and the kernel's initramfs lines (or, for Option 2, the verity status, the mounted
application and PCR12 read with `tpm2_pcrread`).

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

Checked outside the parser for the application recordings (recorded 2026-11-11 between 07:10:59 and 08:03:23 UTC,
the times of the first and last `record.txt`):

| Value | Check |
|---|---|
| INITRD of each Option 1 recording | equals the "combined" sha256 `mkappinitrd` printed on the instance (in each `*-boot.txt`) |
| INITRD of both Option 2 recordings `b0fd172d…f33a` | `sha256sum /boot/initrd-attestamp-verity.img` on the instance |
| CMDLINE of `gcp-n2d-opt2-v1-*` `2c2f6529…8b52` | `printf '%s' '/vmlinuz-7.0.0-1017-gcp root=UUID=9d4d258d-90ac-4d94-b296-007fa0812719 ro console=ttyS0,115200 attestamp.roothash=e29ec085…af0c9' \| sha256sum` (single spaces, as GRUB logs it; the grub.cfg line has two before `console`) |
| PCR12 of both Option 2 recordings | `tpm2_pcrread sha256:12` on the instance, and SHA-256(32 zero bytes ‖ SHA-256("attestamp-verity-ok:" ‖ root hash)) computed with Python |
| the Option 2 image is reproducible | `make-verity-image.sh` run twice for v1 gave the same `app.img`, `app.hash` and root hash |

The nonce of each application recording is SHA-256 of its nonce text, `attestamp-snp-<cell>`, e.g.
`attestamp-snp-opt1p-v1` (in each `*-record.txt`).

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

`record/main.go` is skipped by `go build ./...` (Go ignores `testdata` in patterns) but builds when named, with the
module's own pinned dependencies (go-tpm-tools v0.4.11). From the repository root:

```
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -o record-snp ./internal/tee/parse/snp/testdata/record
```

(The 2026-11-10 recording used a separate scratch module with the same dependency versions, because the module did
not depend on go-tpm-tools yet.) `crypto/sha3` needs Go 1.24 or later; the pinned go1.26.1 has it.

To record evidence with an application in the initrd, see `docs/plan/app-fixture-recording.md`.

Then on an SEV-SNP guest booted with an initrd: `sudo ./record-snp -out fx -nonce-text attestamp-snp-fixture-0001`.
