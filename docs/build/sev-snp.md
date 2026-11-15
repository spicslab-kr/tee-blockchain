# AMD SEV-SNP: what is measured, what changes it, and what is not done yet

**Status.** The verifying side works on recorded evidence: `ra-parse` checks a GCP SEV-SNP evidence pair and
extracts its values (`internal/tee/parse/snp`, formats `amd-sev-snp-initrd` and `amd-sev-snp-verity`). Requesting
evidence from a running guest works (`att-ra-responder ra-serve --extractor snp-initrd | snp-verity`; the whole chain
passed on a live guest with `snp-initrd` on 2026-11-12). Building the image and launching it are not implemented.
Tools and versions: [toolchain.md](toolchain.md). A known weakness is
written down below under "Same VM": SEV-SNP support is weaker here than Nitro or TDX, and that does not go away
with more code.

Every statement below is marked by where it comes from: **confirmed** (read from our recorded evidence, a spec, a
library's source or an official page, named next to it), **observed** (seen in our own runs, not proved in general)
or **not confirmed**.

## How the evidence is shaped

On GCP the SNP launch measurement covers only the virtual firmware. The boot files that carry the application are
measured by the cloud's vTPM. So one piece of evidence is two documents from two signers:

| part | signed by | carries |
|---|---|---|
| SNP attestation report (1,184 bytes) | the chip's VCEK, chaining to AMD's ARK | the firmware's launch digest, the guest policy, TCB versions, 64 bytes of REPORT_DATA |
| vTPM attestation (go-tpm-tools `attest.Attestation`) | Google's attestation key (AK), chaining to Google's "EK/AK CA Root" | quotes over the PCRs with the verifier's nonce, the TCG event log |

They are bound by **REPORT_DATA = SHA3-512 of the vTPM attestation bytes**: the AMD signature covers REPORT_DATA, so
it covers exactly the vTPM evidence carried next to it. SHA3-512 gives 64 bytes, the size of REPORT_DATA, so the hash
goes in unchanged.

The binding was SHA-512 until **2026-11-11**, when it was changed to SHA3-512 by decision of the project lead.
*Record:* SHA-512 is what the 2026-11 test scripts used; those scripts are no longer available (build-info gaps #5).
Results from 2026-11-10 and earlier were produced with SHA-512. Evidence bound with SHA-512 is now refused
(`TestVerifySig_SHA512BindingIsRefused`, on the one SHA-512 recording this repository keeps).

The SHA3-512 binding applies to the GCP path, the only one we use; if an AWS path is ever added, its binding must be
changed to match.

### Only GCP

SEV-SNP is done on GCP only (decided 2026-11-11); AWS is used for Nitro Enclaves. **The AWS path is not ours:** on
AWS, SEV-SNP is an instance option and the vTPM (NitroTPM) an AMI attribute, so a guest with both needs a snapshot of
a running instance re-registered as an AMI with TPM support (`e2e/cloud/aws-snp.sh`, kept for reference). *Record:*
the 2026-11 AWS runs, made with scripts that are no longer available, reported the same results as GCP, including the
same-VM weakness below; they cannot be re-run.

### Decision: one CBOR container, two format strings

**Container.** `RADoc.Raw` holds one CBOR map with three byte strings:

```
{ "report": <SNP report>, "certs": <the host's certificate table, may be empty>, "vtpm": <vTPM attestation> }
```

Why: the wire protocol carries one format string and one byte string (`tee/request/*/wire.go`), so the two documents
must travel as one. CBOR is what Nitro documents already use and `fxamacker/cbor` is already a dependency; byte
strings need no base64; named keys allow a field to be added later. `snp.Encode` writes CBOR's core deterministic
encoding, so the same evidence always gives the same bytes. The decoder refuses unknown keys and duplicate keys, so
an old verifier cannot silently ignore a field it does not understand. The host's certificate table is carried so
that the AMD chain is checked offline, without asking AMD's key server.

**Format strings.** Two names for the same container:

| format | the application is | values registered |
|---|---|---|
| `amd-sev-snp-initrd` | a cpio archive appended to the base initrd (Option 1) | MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD |
| `amd-sev-snp-verity` | a squashfs image on disk, checked by dm-verity at boot against a root hash on the kernel command line; the result is extended into PCR12 (Option 2) | MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD, PCR12 |

TDX does not need this split: there the Option 2 image goes inside the initrd, which RTMR2 already covers, so
`intel-tdx` carries both options with the same four names (`docs/build/tdx.md`, "Option 1 and Option 2"). On SEV-SNP
the image sits on disk, so a boot-time hook extends PCR12 and a sixth value appears.

Why two: the on-chain `measurementHash` is computed over the sorted list of `name=value` lines. One format with five
values in one case and six in another would give hashes that can never be compared, and a User could not tell
which list a registration used. With two names each list is fixed, and the platform string in the record key keeps
the two apart on chain.

## The values

| name | what it is | where it comes from |
|---|---|---|
| MEASUREMENT | the SNP launch digest (48 bytes): the virtual firmware the guest started in | the AMD-signed report |
| PCR4 | the boot applications the firmware ran (shim and GRUB on GCP's Ubuntu images) | the vTPM quote, SHA-256 bank |
| KERNEL | SHA-256 of the kernel file GRUB loaded | GRUB's measurement into PCR9, replayed from the event log |
| CMDLINE | SHA-256 of the kernel command line GRUB passed, as text | GRUB's `kernel_cmdline` event |
| INITRD | SHA-256 of the initrd file GRUB loaded (base initrd + application archive) | GRUB's measurement into PCR9 |
| PCR12 | the dm-verity result (`verity` only) | the vTPM quote, SHA-256 bank |

Why more than the application's hash: GRUB and the kernel are what measure and load the initrd. Registering INITRD
alone would let a changed boot loader or kernel report the expected initrd while booting another. PCR4, KERNEL and
CMDLINE close that gap. The same reasoning made TDX register MRTD and RTMR0–2, and Nitro PCR0–2.

Why SHA-256: the verity hook (`internal/tee/parse/snp/testdata/record/initramfs-local-bottom.sh`) extends only the
SHA-256 bank, and PCR12's expected value is computed with SHA-256 (`app_fixture_test.go`; it matched the Option 2
recordings of 2026-11-11).

Left out, because they describe the machine or the run rather than the image: `CHIP_ID`, the TCB versions, the
report's `HOST_DATA`, and every other PCR. Nitro leaves out PCR3, PCR4 and PCR8 for the same reason.

The checks `ra-parse` makes, in order (confirmed against the recorded evidence, `internal/tee/parse/snp/*_test.go`):

1. The VCEK from the certificate table chains to the AMD ARK go-sev-guest embeds, and the report's signature
   verifies. go-sev-guest does both in one call, so a certificate problem here is reported as a signature failure.
2. The AK certificate chains to Google's EK/AK CA Root (embedded as `root.pem`).
3. The quotes' signatures, and the event log replayed against the quoted PCRs (go-tpm-tools).
4. REPORT_DATA = SHA3-512(vTPM attestation).
5. The quotes carry the nonce the verifier sent.
6. POLICY.DEBUG (bit 19) is clear. A debug guest is refused.
7. GRUB loaded an initrd. An initrdless boot is refused (see "Variables").
8. For `verity`: the command line has `attestamp.roothash=` and PCR12 is not zero.

## Time and freshness

A Nitro document carries a `timestamp`, and `internal/tee/parse/aws/verify.go` judges the certificate chain as of
that time, because a Nitro leaf certificate lives about three hours. Is there an equivalent here?

**There is no time in either document.**

- The SNP report has no timestamp or wall-clock field. Confirmed: AMD SEV-SNP Firmware ABI Specification (pub. 56860,
  rev. 1.59, Aug 2026), Table 27, offsets 00h–49Fh. The only time-like feature in that spec is Secure TSC (§7.9),
  the guest's own cycle counter, which is not in the report.
- The TPM quote's `TPMS_CLOCK_INFO.clock` is "a time value in milliseconds that advances while the TPM is powered";
  where zero is "out of the scope of this specification". It is reset by `TPM2_Clear` and can be set forward by
  `TPM2_ClockSet`. Confirmed: TCG TPM 2.0 Library Part 2 r1.59, Table 120 (§10.11.1); Part 1 §36.3 calls its
  accuracy approximate. It is a counter, not a trusted wall-clock time.

**The TCB fields are versions, not times.** Confirmed in 56860, Table 27 and §3.3–3.4:

| field | meaning |
|---|---|
| CURRENT_TCB (38h) | the TCB version of the firmware running now |
| REPORTED_TCB (180h) | the TCB version used to derive the VCEK that signed this report; set by the hypervisor, never above the installed TCB |
| COMMITTED_TCB (1E0h) | the TCB the firmware cannot be rolled back below |
| LAUNCH_TCB (1F0h) | the TCB when the guest was launched. Table 27 says "the CommittedTcb at the time the guest was launched", the guest-context table says "the CurrentTcb" — the spec is inconsistent |

In our recorded GCP report all four are `0xe11f000000000004` (confirmed, `gcp-n2d-initrdless-report.bin`).

**The certificates live for years, so judging them "now" is enough.**

| certificate | lifetime | source |
|---|---|---|
| VCEK in our GCP evidence | 2026-11-01 → 2033-11-01 | confirmed: `gcp-n2d-auxblob.bin` |
| VCEK in go-sev-guest's testdata (Milan) | 2022-09-24 → 2029-09-24 | confirmed: `verify/testdata/vcek.testcer` |
| the same chip's VCEK fetched from AMD's KDS on 2026-11-10 | 2026-11-09 → 2033-11-09, same public key | observed: KDS reissues it with the fetch date |
| ARK-Milan, ASK (SEV-Milan) | 2020-10-22 → 2045-10-22 | confirmed: `kdsintf.amd.com/vcek/v1/Milan/cert_chain` |
| Google AK certificate in our evidence | 2026-11-10 → 2056-11-02 | confirmed: `gcp-n2d-initrdless-att.pb` |
| Google EK/AK CA Root | 2022-07-08 → 2122-07-08 | confirmed: `root.pem` |

What AMD's KDS specification (pub. 57230) says about VCEK lifetime is **not confirmed**: the document could not be
retrieved.

**Which key signs: the VCEK.** Google's page says "Version Chip Endorsement Key (VCEK) certificates are cached on
the VM instead of requested directly from AMD's key distribution system"
(docs.cloud.google.com/confidential-computing/confidential-vm/docs/attestation), and both our recorded reports have
SIGNING_KEY = 0 (VCEK) with a VCEK in the host's certificate table (confirmed). Whether GCP ever uses a VLEK is not
confirmed.[^vlek]

[^vlek]: The other AMD key, the VLEK, is not on our path. AWS signs with a VLEK on shared tenancy (and a VCEK on
    Dedicated Hosts), per AWS's EC2 user guide (snp-attestation page). Its chain is
    `kdsintf.amd.com/vlek/v1/Milan/cert_chain`: ASVK (SEV-VLEK-Milan) 2022-11-16 → 2047-11-16 (confirmed from that
    URL). A VLEK lives from one day before issue to seven years after (confirmed: VLEK spec, pub. 58369 rev. 0.10,
    Table 8). The parser has never been run on VLEK-signed evidence.

**Decision.**

1. Both chains are judged at verification time (`Parser.Now`, default the current time). A seven-year VCEK and a
   thirty-year AK certificate make "valid when the report was made" and "valid now" practically the same question,
   and the first cannot be asked anyway: nothing in the evidence says when it was made.
2. Freshness comes only from the nonce. The verifier's nonce is in the quotes' `extraData`; the quotes are inside the
   vTPM attestation; its SHA3-512 is REPORT_DATA. So a report replayed from an earlier request carries an earlier
   nonce and is refused. The verifier's own clock at the time it made the nonce bounds when the evidence was made.
3. Staleness of the platform is a TCB question, not a time question. go-sev-guest's `validate` package can require
   minimum TCB versions (`MinimumTCB`, `MinimumLaunchTCB`) and check that the VCEK's TCB equals REPORTED_TCB.
   Attestamp does not apply any of that yet (see "Not done yet").
4. The TPM `clock` is not used. It could only order quotes from the same TPM within one reset period.

Library behaviour relied on (confirmed in go-sev-guest v0.15.0 source): `verify.Options.Now` is "the time at which
to verify the validity of certificates. If unset, uses time.Now()"; CRL checking (`CheckRevocations`) is off by
default; `DisableCertFetching` keeps it offline. go-sev-guest v0.15.0 accepts report versions 2–5 only. Spec 1.59
defines version 6; our GCP report is version 5 (confirmed). A version 6 report would be refused until the library is
updated.

## Variables

*Record:* each row changed a hash in the 2026-11 tests, with the same source code (observed on GCP and AWS, with
scripts that are no longer available and cannot be re-run). Where this repository has measured the same thing itself,
the row says so.

| variable | what changed | how we kept it fixed |
|---|---|---|
| Go version | Go 1.22.2 and 1.23.4 gave different application binaries (record) | the same Go version on both sides. **Ours:** the responder built with go1.26.1 and go1.26.2 differs (Nitro baseline 2026-11-12, `toolchain-pin-check.sh`), and `build` refuses another version |
| build path | without `-trimpath`, two folders gave two different binaries (record) | `-trimpath`. **Ours:** `TestBuildResponderForArch_ReproducibleAcrossCheckoutPaths`, and two checkout paths gave the same Nitro PCRs (2026-11-12) |
| cpio tool | GNU cpio 2.13 (Amazon Linux 2023) and 2.15 (Ubuntu) packed the same files into different bytes (directory link counts) (record) | our own cpio writer: `internal/setup/build/cpio` (sorted names, time 0, owner 0, fixed inode numbers). It was byte-identical to the 2026-11 scripts' `mkcpio.py`, which is no longer available; the golden files in `internal/setup/build/cpio/testdata` pin that output |
| distribution base initrd | `mkinitramfs` and `dracut` build a different file every time (it contains a random seed) | none possible. The Auditor uses the base initrd the Vendor publishes and rebuilds only the application part |
| folder permissions (umask) | the same files in a squashfs image gave a different image and root hash under umask 022 (folders 755) and umask 002 (folders 775) | **fixed (2026-11-11):** `internal/setup/build/cpio/testdata/make-verity-image.sh` sets **`umask 022`** and sets every folder and file to mode **755** explicitly. With it, the image built on `attestamp-snp-vendor` and on `attestamp-tdx-vendor` was byte-identical (`app.img` sha256 `28bdfcff…ec1c`, root hash `e29ec085…af0c9`). A build that sets neither still depends on the caller's umask |
| squashfs timestamps | `mksquashfs` writes the build time by default | `-mkfs-time 0 -all-time 0`, file times set to 0 |
| dm-verity salt and UUID | `veritysetup format` picks them at random | fixed `--salt` and `--uuid` |

*Record:* with all of the above fixed, the application, its cpio archive and the dm-verity image hashed the same on GCP
(Ubuntu 24.04, 26.04) and AWS (Amazon Linux 2023), across squashfs-tools 4.4, 4.6.1 and 4.7.5, in the 2026-11 runs.
**Ours:** the dm-verity image was byte-identical on the SEV-SNP and the TDX guest (2026-11-11, squashfs-tools 4.6.1 on
both; `docs/plan/test-matrix.md`, T1).

What else changes the registered values (from the evidence's structure; **not measured by changing each one**):

| value | changes with |
|---|---|
| MEASUREMENT | the virtual firmware GCP launches the guest with; GCP may update it |
| PCR4 | shim and GRUB, i.e. the OS image and its updates |
| KERNEL | the kernel package |
| CMDLINE | the boot entry. It names the root by an identifier that differs per disk (the file system's UUID on the stock entry with an initrd, the partition's PARTUUID on the initrdless one), so the Auditor must use the command line the Vendor publishes |
| INITRD | the base initrd and the application archive, **where the archive goes**, and the padding between them. On SEV-SNP the archive is appended after the base initrd (`cpio.AppLast`), which since 2026-11-11 zero-pads the base to a multiple of 4 bytes. Without that padding the kernel discards the archive and the guest still boots, and INITRD still changes with the application (**measured**, see "Correction: SEV-SNP Option 1 as appended in 2026-11" below) |

**Initrdless boot.** GCP's Ubuntu 24.04 image sets `GRUB_FORCE_PARTUUID`, and GRUB then boots the stock entry
**without any initrd** (confirmed: our first recording, `gcp-n2d-initrdless-*`, logs `GRUB_FORCE_PARTUUID set,
attempting initrdless boot.` and measures no initrd). An application in an initrd is then never loaded or measured.
`ra-parse` refuses evidence without an initrd measurement. Turning the setting off, as below, makes the stock entry
load the initrd; `internal/setup/build/cpio/testdata/make-grub-entry.sh` makes a separate entry from it for an
application (`docs/plan/app-fixture-recording.md`). (*Record:* the 2026-11 scripts avoided it by booting their own
GRUB entry with `grub-reboot`.)

### Correction: SEV-SNP Option 1 as appended in 2026-11

The 2026-10-23 slides (slide 7) say both options work on AWS and GCP. **For SEV-SNP Option 1 that is wrong**, as
measured on 2026-11-11 on `attestamp-snp-vendor` with the same unpadded append the 2026-11 scripts used (*record:* they
ran `cat i.bin app.cpio`):

- The base initrd, `/boot/initrd.img-7.0.0-1017-gcp`, is 50,100,283 bytes: **length mod 4 = 3**.
- The application archive appended directly after it was discarded by the kernel:
  `Initramfs unpacking failed: invalid magic at start of compressed archive` (dmesg, both boots).
- The marker the archive carries (`/dev/attestamp-app-marker`, see `docs/plan/app-fixture-recording.md`) was
  **absent**: the application never ran.
- The guest booted normally on the base initramfs, and GRUB measured the whole combined file, so **INITRD changed with
  the application although the application was not there**. Nothing in the evidence shows it.
- The cause is alignment, not the kernel or zstd. The kernel only accepts an uncompressed archive after a compressed
  segment at a 4-byte-aligned offset (`init/initramfs.c`).
- With the base zero-padded to 50,100,284 bytes (`truncate -s %4`), the same append unpacked: no error line, and the
  marker read `attestamp app v1` (and `v2` on the next boot).

Fixed in `cpio.Join(AppLast)`, which now pads the base (commit `3c57d4c`). Both pairs are kept as fixtures with their
boot logs: `internal/tee/parse/snp/testdata/gcp-n2d-opt1-*` (unpadded) and `gcp-n2d-opt1p-*` (padded).

Whether the 2026-11 SEV-SNP Option 1 runs, on GCP or AWS, actually ran the application depends on the length of the
base initrd they used. Those files, their scripts and any boot logs are gone, so it cannot be checked.

Option 2 does not have this problem. The image is on disk, and a failed check shows as `fail` in PCR12 or a zero PCR12
(measured: `status ok` and the expected PCR12 in both Option 2 recordings).

### The boot setting changed on `attestamp-snp-vendor` (2026-11-11)

Done to record `gcp-n2d-*`, the evidence with an initrd. Every step was checked on the instance:

1. The setting is not in `/etc/default/grub` but in `/etc/default/grub.d/40-force-partuuid.cfg`, line 3:
   `GRUB_FORCE_PARTUUID=<the root partition's PARTUUID>`. The file's own comment says "Remove this line to enable boot
   with an initramfs".
2. Original kept: `sudo cp -p /etc/default/grub.d/40-force-partuuid.cfg /root/40-force-partuuid.cfg.orig`.
3. Line commented out: `sudo sed -i 's/^GRUB_FORCE_PARTUUID=/#GRUB_FORCE_PARTUUID=/' /etc/default/grub.d/40-force-partuuid.cfg`.
4. `sudo update-grub`. It printed `Found initrd image: /boot/initrd.img-7.0.0-1017-gcp`, and the default entry in
   `/boot/grub/grub.cfg` became `linux /vmlinuz-7.0.0-1017-gcp root=UUID=<file system UUID> ro console=ttyS0,115200`
   followed by `initrd /initrd.img-7.0.0-1017-gcp`. Before, the command line was `root=PARTUUID=<…> ro
   console=ttyS0,115200 panic=-1` with no initrd.
5. `sudo systemctl reboot`; `/proc/sys/kernel/random/boot_id` changed. The serial console showed `Trying to unpack
   rootfs image as initramfs...` and `Freeing initrd memory: 48928K`, and no longer `attempting initrdless boot`.
6. The TPM event log (`/sys/kernel/security/tpm0/binary_bios_measurements`) holds GRUB's command `initrd
   /initrd.img-7.0.0-1017-gcp` and a measurement of that file. The parser reads that measurement as INITRD, and it equals
   `sha256sum /boot/initrd.img-7.0.0-1017-gcp` on the instance (`4703ec07…b0a2`).

To undo: `sudo cp -p /root/40-force-partuuid.cfg.orig /etc/default/grub.d/40-force-partuuid.cfg`, `sudo update-grub`,
reboot.

The setting stays changed on that instance; it boots with the initrd from now on. One side effect seen: the kernel
command line lost `panic=-1`, which only the initrdless entry adds (observed in the two recordings).

## The instance

`e2e/cloud/gcp-cvm.sh snp create <roles…>` creates it. The settings and why:

| setting | value | why |
|---|---|---|
| machine type | `n2d-standard-2` | SEV-SNP on GCP runs on N2D only (script header, checked against Google's docs 2026-11) |
| CPU platform | `--min-cpu-platform="AMD Milan"` | the script header lists AMD Milan as the CPU platform for SEV-SNP on N2D |
| confidential type | `--confidential-compute-type=SEV_SNP` | |
| maintenance | `--maintenance-policy=TERMINATE` | these VMs cannot live-migrate |
| image | `ubuntu-os-cloud/ubuntu-2404-lts-amd64` | must carry the `SEV_SNP_CAPABLE` guest OS feature; the script checks it |
| zone | `us-central1-a` (default) | one of the zones that offer SEV-SNP |
| vTPM | nothing to set | `/dev/tpm0` and `/dev/tpmrm0` were present without a flag (observed 2026-11-10) |

Our recorded guest, `attestamp-snp-vendor` (read with `gcloud compute instances describe` and `gcloud compute disks
describe` on 2026-11-11):

| | |
|---|---|
| image | `ubuntu-os-cloud/global/images/ubuntu-2404-noble-amd64-v20261105` (the family moves; this is the exact image) |
| boot disk | 64 GB, `pd-standard` |
| CPU platform | AMD Milan (minimum) |
| Shielded VM | Secure Boot **off**, vTPM on, integrity monitoring on |
| maintenance | `TERMINATE` |
| OS, kernel | Ubuntu 24.04.5, `7.0.0-1017-gcp`, with `GRUB_FORCE_PARTUUID` turned off since 2026-11-11 |
| devices | `/dev/sev-guest`, `/dev/tpm0`, `/dev/tpmrm0`; ConfigFS TSM provider `sev_guest`; SEV-SNP active in `dmesg` |
| tools on the guest | OpenSSL 3.0.13 (image default); tpm2-tools 5.6-1build4, installed by hand later the same day for the Option 2 hook. No Go, `snpguest` or `gotpm` |

## From scratch: record and check one evidence pair

1. Create the instance: `bash e2e/cloud/gcp-cvm.sh snp create vendor`. On the instance, set a safety stop:
   `sudo shutdown -h +240`.
2. Build the recorder (`internal/tee/parse/snp/testdata/README.md`, "Recording another one") and copy it over with
   `gcloud compute scp`.
3. On the instance: `sudo ./record -out fx -nonce-text <your text>`. It writes `att.pb`, `report.bin`, `auxblob.bin`
   and `record.txt` (the nonce and REPORT_DATA).
4. Copy the files back and stop the instance: `gcloud compute instances stop <name> --zone <zone>`.
5. Pack the three files into the document `ra-parse` reads, and check it:
   `go run ./internal/tee/parse/snp/testdata/pack -report report.bin -certs auxblob.bin -vtpm att.pb -out doc.cbor`,
   then `att-user ra-parse --doc doc.cbor --format amd-sev-snp-initrd --nonce <hex nonce>`.

On a stock GCP boot step 5 verifies both signatures and the nonce and then refuses, because the boot was initrdless.
Turn `GRUB_FORCE_PARTUUID` off first, as in "The boot setting changed on `attestamp-snp-vendor`" above; the stock
entry then loads the distribution initrd and `amd-sev-snp-initrd` yields five values. To boot an initrd that carries
an application (Option 1), or the dm-verity image with its root hash on the command line (Option 2), follow
`docs/plan/app-fixture-recording.md`: it builds the initrd with `mkappinitrd`, the image with `make-verity-image.sh`,
and the boot entry with `make-grub-entry.sh` from the instance's own stock entry, so nothing is hard-coded.

The two roots of trust in one SEV-SNP measurement set (AMD for MEASUREMENT, Google's vTPM for the rest), and the
check that MEASUREMENT is identical across our initrd and initrdless recordings, are written up in
`docs/build/ra-comparison.md`, "Roots of trust".

## Byte order

Sources: AMD SEV-SNP Firmware ABI, pub. 56860, **rev. 1.58** (May 2025; the revision we could retrieve, cited elsewhere
in this file at 1.59); GHCB, pub. 56421 rev. 2.04; TCG TPM 2.0 Library Part 1 r1.59; TCG PC Client Platform Firmware
Profile v1.05 r23. "Pinned" means a test fails if the order were the other one.

**SNP report** (offsets into the 0x4A0-byte report). Rev. 1.58 states little endian only for the signature and
public-key formats (Tables 141, 142); for the other fields it gives no general convention. Little endian there is
go-sev-guest's reading, backed by the recorded report, where reserved bits make the wrong order visible.

| field | offset, size | order | spec | pinned by |
|---|---|---|---|---|
| VERSION | 0x00, u32 | LE | Table 23 ("Set to 5h"); order not stated | the signature test (read BE the version is outside 2–5 and go-sev-guest refuses it) |
| POLICY | 0x08, u64 | LE | Table 23; Table 9: bit 17 "must be one", bit 19 DEBUG, bits 63:26 MBZ | `TestEndian_PolicyIsLittleEndian` (LE gives 0x30000; BE would clear bit 17), `TestEndian_PolicyHighReservedBitIsRefused`, `TestParse_DebugGuestIsRefused` |
| SIGNER_INFO (SIGNING_KEY bits 4:2) | 0x48, u32 | LE | Table 23: bits 31:5 MBZ | `TestEndian_SignerInfoReservedBitsAreRefused` (the fixture's value is 0, so it alone could not tell) |
| SIGNATURE_ALGO | 0x34, u32 | LE | Table 23; Table 139: 1 = ECDSA P-384 with SHA-384 | signature test |
| signature R, S | 0x2A0, 0x2E8, 72 bytes each | LE, zero-extended | Table 141: "zero-extended little-endian encoded" | `TestEndian_SignatureRIsLittleEndianZeroExtended`; signature test |
| GUEST_SVN, VMPL, CURRENT/REPORTED/COMMITTED/LAUNCH_TCB, PLATFORM_INFO, mitigation vectors | various u32/u64 | LE | Table 23; order not stated | **not pinned**: nothing judges these values today. It will matter once TCB or VMPL checks are added |

**Certificate table** (the host's `auxblob`, GHCB §4.1.8.1). GUIDs are "formatted and encoded as defined in RFC 4122"
(big-endian fields), pinned by `TestEndian_CertTableGUIDIsRFC4122`: rewritten in the EFI mixed-endian order the VCEK is
no longer found. The entry's offset and length are u32 with no stated order; read LE they point at the certificates
(every decode depends on it).

**vTPM attestation.**

| structure | order | spec | pinned by |
|---|---|---|---|
| TPMS_ATTEST in each quote (magic, type, size prefixes, clockInfo, PCR selection) | BE | TPM 2.0 Part 1 §7.1: "The octet at offset zero within the array is the most significant octet" | `TestCheckNonce`, `TestInfo` (the nonce comes out of `extraData`), signature test |
| TPMT_SIGNATURE, TPMT_PUBLIC | BE | same | signature test |
| TCG event log (event headers, Spec ID event, digests' algorithm IDs and sizes) | LE | PC Client PFP §2.7: "All constants and data SHALL be represented as Little Endian format unless otherwise explicitly stated"; §10.1 | the replay against the quoted PCRs, and KERNEL, CMDLINE, INITRD in `TestParse_RealEvidence` |

So one piece of SEV-SNP evidence mixes three conventions: AMD's little-endian report, the TPM's big-endian quotes and
the firmware profile's little-endian event log, plus RFC 4122 GUIDs in the certificate table and EFI-ordered GUIDs in
the event log. Each library handles its own part.

As with TDX, go-sev-guest verifies the report signature over bytes rebuilt from the parsed fields
(`ReportToAbiBytes`), so a passing signature alone does not pin byte order; the reserved-bit checks above do.

Seen while reading, not verified further: in the path we use (no certificate fetching), go-sev-guest does not appear to
compare REPORTED_TCB with the VCEK certificate's TCB extension, so the TCB is not judged (as "Not done yet" says).

## Runtime measurement: what SEV-SNP can and cannot do

Researched 2026-11-11, in answer to the remark that "TDX has runtime checks and SEV-SNP is written as if it can, but in
practice it cannot". The remark is right for SEV-SNP hardware, and for SEV-SNP on GCP today.

**TDX (confirmed).** RTMR0–3 can be extended by the guest at run time. Since Linux 6.16 the kernel exposes them as
`/sys/devices/virtual/misc/tdx_guest/measurements/rtmr[0-3]:sha384`: writing 48 bytes extends the register
(`TDG.MR.RTMR.EXTEND`). The values appear in the next quote, which Intel's Quoting Enclave signs. Attestamp registers
RTMR0–2 and leaves RTMR3, the runtime register, out.

**SEV-SNP (confirmed, ABI 56860 rev. 1.58).**

- The attestation report has **no register the guest can extend**.
- MEASUREMENT ("calculated at launch"), HOST_DATA, ID_KEY_DIGEST, AUTHOR_KEY_DIGEST, FAMILY_ID and IMAGE_ID are all
  fixed at launch.
- At request time the guest supplies only REPORT_DATA (64 bytes, "the firmware does not interpret this data") and a
  VMPL (Table 22, MSG_REPORT_REQ).
- None of the guest message types (CPUID, KEY, REPORT, EXPORT, IMPORT, ABSORB, VMRK, ABSORB_NOMA, TSC_INFO) extends a
  measurement.
- REPORT_DATA does not accumulate. A "runtime" value can only be hashed into it per request, with no
  hardware-kept history.

**Linux (confirmed).** The kernel's measurement-register interface (`drivers/virt/coco/tsm-mr.c`, 6.16) is used by
`tdx-guest` only. `sev-guest` registers report generation and no measurement registers.

**The vTPM on GCP (confirmed, Google's attestation overview, re-read 2026-11-11).** The vTPM of a GCP SEV-SNP VM is
"managed by the host hypervisor" and belongs to a "Software root of trust (hypervisor dependent)"; for SEV-SNP the page
lists "Bootloader, kernel, and userspace measurements: Shielded VM vTPM". (An earlier version of this paragraph quoted
"software-attested instead of hardware" from that page; the phrase is not on it.) Anything extended into PCR10–23 at run
time (IMA, for example) is signed by Google's attestation key, not by AMD, and that vTPM sits outside the SNP trust
boundary. That is also why our PCR4, KERNEL, CMDLINE, INITRD and PCR12 are Google-signed (see "Same VM").

**What would give SEV-SNP a runtime register.**

- A vTPM inside the guest's trust boundary: an SVSM running at VMPL0, as AMD's SVSM specification (pub. 58019)
  describes.
- Linux has had the guest driver since 6.16 (`drivers/char/tpm/tpm_svsm.c`, `CONFIG_TCG_SVSM`); Canonical backported it
  to `linux-gcp` (June 2025, per the research; not checked on our kernel).
- COCONUT-SVSM's vTPM is ephemeral: its EK is regenerated every boot.
- Azure's Confidential VMs run their vTPM "inside the VM in a hardware-based protected memory region… isolated and
  encrypted by AMD SEV-SNP" (Microsoft's docs).
- For GCP, the only mention found is a Google community blog (June 2026) calling SVSM vTPM key isolation "available in
  preview". Google's documentation and release notes have no entry for it (not confirmed).
- How an SVSM vTPM's key is bound into an SNP report could not be confirmed: spec 58019 could not be retrieved.

| capability | TDX | SEV-SNP on GCP |
|---|---|---|
| launch measurement signed by the hardware vendor | MRTD | MEASUREMENT |
| register the guest can extend, signed by the hardware vendor | RTMR0–3 | **none** |
| Linux interface to extend it | sysfs `measurements/rtmr*`, 6.16 | none |
| per-request guest data | REPORTDATA, 64 bytes | REPORT_DATA, 64 bytes |
| runtime PCRs | GCP vTPM (Google-signed) | GCP vTPM (Google-signed, hypervisor-managed) |
| hardware-rooted vTPM | not needed (RTMRs) | SVSM vTPM: kernel driver since 6.16; on GCP only a "preview" claim in a blog |

Attestamp does not use any runtime measurement on either platform today, and this document makes no runtime claim for
SEV-SNP. A future runtime check on SEV-SNP would rest on Google's vTPM, which the "Same VM" weakness below already
limits.

## Same VM

The binding proves that the AMD-signed report covers this vTPM evidence. It does not prove that both come from the
same VM: a vTPM attestation from an approved VM X, hashed into an SNP report requested on another VM Y, verifies.
*Record:* that was seen on GCP and AWS in the 2026-11 runs, with a relay script that is no longer available (test
matrix row T10). This repository has not reproduced it yet: its mixed-recording test combines two recordings of the
**same** VM, which is a different case. `docs/plan/app-fixture-recording.md`, "Reproducing T10", is the procedure to
measure it with two VMs. Nitro and TDX return one document from one TEE and do not have this problem. It goes away only if the boot files are in the SNP
launch measurement itself (direct kernel boot; whether GCP allows it is not confirmed) or if the vTPM runs inside
the SNP guest's own trust boundary.

## Not done yet

| what | why |
|---|---|
| `att-ra-responder ra-serve --extractor snp-initrd` / `snp-verity` answering a verifier on another machine | Phase 2.2. On the guest itself (127.0.0.1) `snp-initrd` passed the whole chain on 2026-11-12 (`e2e/cvm/in-guest-verify.sh`); `snp-verity` was refused on that boot because it had no verity image, and has not run on a verity boot. From outside: start it with `--listen tcp:0.0.0.0:5000` and allow port 5000 only from the verifier's address. The guest also needs outbound access to Google's certificate host while attesting (the AK intermediates are fetched) |
| `setup/build`, `setup/launch` for SEV-SNP | Phase 4b; until then `docs/plan/app-fixture-recording.md` does it by hand |
| a verity boot through the responder | the recordings `gcp-n2d-opt2-*` (2026-11-11) have the verity hook and a PCR12 that matches the formula, but were made with `record-snp`, not with `att-ra-responder`; the tamper case (an image that does not match its root hash) was not recorded |
| TCB minimums, CRLs | not applied; the platform's patch level is not judged |
| AMD chain errors as `ErrChain` | go-sev-guest checks the chain and the signature in one call |
| T10 measured by this repository | the only evidence is the 2026-11 record; the procedure is written (`docs/plan/app-fixture-recording.md`, "Reproducing T10") and not run |

AWS evidence is not on this list: SEV-SNP on AWS was decided against (see "Only GCP").

**The 2026-10/11 shell scripts (removed 2026-11-15).** *Record:* the SEV-SNP experiments of 2026-10 and 2026-11 used
shell scripts outside this repository (`vendor/`, `auditor/`, `user/`, `option1-initrd/`, `option2-dm-verity/`,
`relay-test/`, `mkcpio.py`). Their author has left the lab and they are no longer available anywhere (build-info gaps
#5, #6). This document no longer points to them. Everything they did that this repository relies on was redone here:
the recordings, their measured values and the procedures in `docs/plan/app-fixture-recording.md`. Lost and not
replaced: "the seven 2026-11 tests" as one runnable set, and the relay test (T10, see above).

## Checking

There is no `--verify` for SEV-SNP yet. Until there is, build twice and on two machines, and compare the hashes of the
application, the cpio archive and the disk image before registering anything.
