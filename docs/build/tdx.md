# Intel TDX: what is measured, what changes it, and what is not done yet

**Status.** The verifying side works on a recorded quote: `ra-parse`, `audit` and `verify` accept `intel-tdx`
(`internal/tee/parse/tdx`). Requesting a quote from a running guest, building the image and launching it are not
implemented. The audit-and-verify chain test (`TestRun_E2E_TDX_AuditorUser`) passes against a local anvil chain
(first run 2026-11-11, anvil 1.8.0).

Statements are marked **confirmed** (read from our recorded quote, a spec, a library's source or an official page),
**observed** (seen in our runs) or **not confirmed**.

## The quote

One request returns one signed document, like Nitro: a TDX quote (version 4 on GCP, confirmed). It is the TD report
signed by an attestation key; Intel's Quoting Enclave vouches for that key with a report signed by the platform's PCK
certificate, and the PCK certificate chains to the Intel SGX Root CA. The PCK chain is inside the quote.

`ra-parse` checks, in order:

1. The quote decodes as version 4 with TEE type `0x81` (TDX).
2. The PCK certificate chains, through the intermediate carried in the quote, to the Intel SGX Root CA embedded as
   `internal/tee/parse/tdx/root.pem`. The root at the end of the quote's own chain is never trusted.
3. go-tdx-guest checks the quote signature, the Quoting Enclave report signature and the hash that binds the
   attestation key to that report.
4. REPORTDATA carries the nonce sent.
5. TDATTRIBUTES.DEBUG (bit 0) is clear. A debug TD is refused.

Intel's collateral (TCB info, QE identity) and revocation lists come from Intel's PCS over the network and are **not**
fetched, so the check runs offline like the Nitro one. The cost: the platform's TCB level is not judged, and a quote
from a machine with an out-of-date TDX module still verifies.

The root certificate is the same one go-tdx-guest embeds and the one inside our real quote (sha256 fingerprint
`44:A0:19:6B:…:74:D3`, valid 2018-05-21 → 2049-12-31, confirmed). It has not been compared with a copy downloaded from
Intel.

## The values

| name | what it covers |
|---|---|
| MRTD | the TD's initial contents: the virtual firmware (TDVF) GCP launches |
| RTMR0 | the firmware's configuration |
| RTMR1 | the boot loader and kernel |
| RTMR2 | the kernel command line and the initrd, where the application archive is |

The firmware extends what a TPM would put in PCR1 and 7 into RTMR0, PCR2–6 into RTMR1 and PCR8–15 into RTMR2. That
mapping is the TDVF design; **it has not been confirmed on GCP** by changing each part in turn.

Why all four, not RTMR2 alone: the firmware, boot loader and kernel extend RTMR2 themselves, so a changed kernel could
extend the expected RTMR2 while running something else, and only MRTD and RTMR0–1 would show it. Nitro registers PCR0,
1 and 2 together for the same reason.

Left out, because they describe the machine or whoever launched it, not the image:

| field | why |
|---|---|
| MROWNER, MROWNERCONFIG, MRCONFIGID | set by whoever launches the TD; MROWNER is non-zero in our GCP quote (confirmed). That it differs per instance is the project's working assumption, not measured |
| RTMR3 | extended at run time by the guest; zero in our quote (confirmed) |
| MRSEAM, MRSIGNERSEAM | the TDX module, i.e. the platform |

## Nonce and time

REPORTDATA is 64 bytes and a `tee/request` nonce is 32. `CheckNonce` requires REPORTDATA to be the nonce followed by
zeros; a 64-byte nonce fills all of it. The TDX request side, when written, must put the nonce first and zeros after.

A TDX quote has **no timestamp** (confirmed: the quote format has no time field). The PCK chain is therefore judged at
verification time. That costs little: the PCK leaf in our quote is valid 2026-10-15 → 2033-10-15 (confirmed), not the
three hours of a Nitro leaf. Freshness comes from the nonce.

## Variables

Nothing below has been measured by changing it on TDX; the rows are what the structure above implies, or what the
SEV-SNP runs measured for the same files.

| variable | which value | note |
|---|---|---|
| GCP's TDVF version | MRTD | GCP may update it; every registration would change |
| VM shape (vCPUs, memory) | RTMR0 probably | the firmware's configuration includes the ACPI tables. **Not confirmed** |
| shim, GRUB, kernel | RTMR1 | OS image updates |
| kernel command line | RTMR2 | it carries the root PARTUUID, which differs per disk |
| base initrd | RTMR2 | `mkinitramfs` writes a random seed, so the Vendor's base initrd is used as published (as on SEV-SNP) |
| application archive | RTMR2 | Go version, `-trimpath`, the cpio writer: the same as on SEV-SNP (`docs/build/sev-snp.md`). The archive is written by `internal/setup/build/cpio`, byte-identical to the scripts' `mkcpio.py` |
| archive placement | RTMR2 | the application archive goes **before** the base initrd (`cpio.AppFirst`). Appended after the compressed base initrd, the guest failed to decompress it (docs/plan/cvm-port.md, Phase 3.2). The likely reason is a missing 4-byte alignment, not a kernel limit on appending; see "Before or after the base initrd" below |
| `GRUB_FORCE_PARTUUID` | RTMR2 | GCP's Ubuntu 24.04 image boots its stock entry without an initrd when this is set (confirmed on the SEV-SNP image of the same family). An application in an initrd is then never loaded. See "`GRUB_FORCE_PARTUUID`" below |

## Byte order

Every integer in the quote is read little endian, as Intel's quote specification requires; the signatures and the
attestation key are big endian. Sources: Intel "TDX DCAP Quote Generation Library and Quote Verification Library"
(`Intel_TDX_DCAP_Quoting_Library_API.pdf`, rev. 0.9x), Appendix A.3. Offsets are into the raw quote (header 0x00–0x2F,
TD quote body from 0x30). "Pinned" means a test fails if the order were the other one.

| field | offset, size | order | read by | spec | pinned by |
|---|---|---|---|---|---|
| header version | 0x000, u16 | LE | go-tdx-guest `abi` | A.3 "Endianess: Little Endian (applies to all integer fields)"; A.3.1 "Value: 4" | `TestEndian_HeaderIsLittleEndian`; every fixture test (the library refuses other versions) |
| attestation key type | 0x002, u16 | LE | `abi` | A.3.1: 2 = ECDSA-256-with-P-256 | same |
| TEE type | 0x004, u32 | LE | `abi`, compared in `tdx/doc.go` | A.3.1: `0x00000081` = TDX | same |
| TDATTRIBUTES | 0x0A8, 8 bytes | bit 0 in the first byte (LE numbering) | `tdx/measure.go` (`attr[0]`) | A.3.2 types it a byte array; A.3.4 numbers its bits (0 DEBUG, 28 SEPT_VE_DISABLE, 62:32 reserved) but **does not say which byte holds bit 0** | `TestEndian_TDAttributesBitNumbering` (read LE the fixture sets only bit 28; read BE it would set reserved bit 36), `TestEndian_DebugBitIsNotInLastByte`, `TestParse_DebugTDIsRefused` |
| signed data size | 0x278, u32 | LE | `abi` | A.3 rule | every fixture test |
| quote signature r‖s | 0x27C, 2×32 | BE | `abi` (`SignatureToDER`) | A.3.5 "r component followed by the s component"; byte order inside r and s not stated | `TestVerifySig_RealQuoteWithEmbeddedRoot` (BE is what verifies) |
| attestation key x‖y | 0x2BC, 2×32 | BE | go-tdx-guest `verify` | A.3.6; order inside not stated | same |
| certification data type, size | 0x2FC u16, 0x2FE u32 | LE | `abi` | A.3.8–A.3.9 | every fixture test |
| QE report MISCSELECT, ISVPRODID, ISVSVN | 0x312 u32, 0x402 u16, 0x404 u16 | LE | `abi` | A.3.10 | **not pinned**: no check judges these values |
| QE report signature | 0x482, 2×32 | BE | `abi` | A.3.5 | signature test |
| QE auth data size; PCK chain type, size | 0x4C2 u16; 0x4E4 u16, 0x4E6 u32 | LE | `abi` | A.3.7, A.3.9 | every fixture test |

Two things to know:

- go-tdx-guest verifies the signatures over bytes **rebuilt from the parsed fields**, not over the bytes received. A
  field read in the wrong order and written back the same way would still verify, so a passing signature says nothing
  about byte order. Only fields whose values are checked (version, key type, TEE type, the sizes) are pinned by the
  library; TDATTRIBUTES is pinned by our own tests.
- The TDATTRIBUTES bit numbering is pinned by the fixture, not by the DCAP document. Intel's TDX Module ABI
  specification would be the authority; it was not retrieved.

## `GRUB_FORCE_PARTUUID`

Researched 2026-11-11. What is **confirmed** has a source; the rest is marked.

**What it is (confirmed).**

- Ubuntu's cloud-image build (`livecd-rootfs`, `live-build/functions`) sets it for images with a non-generic kernel,
  such as `linux-gcp`.
- At image build time the function `force_boot_without_initramfs` writes the root partition's PARTUUID, as a literal
  value, into `/etc/default/grub.d/40-force-partuuid.cfg`. Our instance has exactly that file (2026-11-11).
- The GRUB side is Ubuntu's own patches to `grub2`: `ubuntu-support-initrd-less-boot.patch` and
  `ubuntu-add-initrd-less-boot-fallback.patch`.
- Purpose: boot time ("no IO involved to lift the initrd… particularly relevant to cloud images", Launchpad bug
  1754586).
- The kernel can find `root=PARTUUID=` without an initramfs, but not a file-system `UUID=`
  (`block/early-lookup.c`). That is why turning the setting off switches the command line to `root=UUID=`, as we saw.

**The fallback (confirmed, Launchpad bug 1870189).**

- Each stock entry records `initrdfail=1` in `grubenv` before booting.
- `grub-initrd-fallback.service` clears it once local file systems are up.
- If the initrdless boot panics first (`panic=-1` reboots at once), the next boot uses the initrd.
- Every later boot tries initrdless again: "on each fresh reboot it again attempts an initramfsless boot before
  falling back".

**The two concerns raised.**

- **Migration: partly valid, for a narrower reason (inferred).** GCP Confidential VMs cannot live-migrate; every
  maintenance event is a stop and start (GCP's Confidential VM docs, confirmed). Cloning a disk copies both the
  partition GUID and the file-system UUID, so neither is more unique.
  - The real risk is that the PARTUUID in the config file is a fixed value that `update-grub` never refreshes. If the
    root file system lands on a partition with a new GUID (re-partitioning, file-level migration), both the
    initrdless entry and its fallback point at a partition that no longer exists, and the VM does not boot.
  - Moving to a machine type whose disk driver is a module rather than built in costs one panic and reboot, then the
    fallback recovers.
- **"Docker might not build": not supported by what we found (inferred).** Docker's kernel needs (overlay,
  br_netfilter, nf_tables, veth, cgroup v2) are loaded from `/lib/modules` on the root file system after boot,
  whether or not an initramfs was used. It would matter only if the application ran from the initramfs and never
  switched to the real root; that is a design choice, not this setting. Not tested.

**For us: off.** An application placed in the initrd needs the initrd loaded on every boot.

| | leave it ON | turn it OFF (what we did on `attestamp-snp-vendor`, see `docs/build/sev-snp.md`) |
|---|---|---|
| initrd loaded | never by the stock entries, so the application is absent and no initrd is measured | by every entry |
| boot | faster (amount not found) | slightly slower |
| root device | a fixed PARTUUID written at image build time | `root=UUID=`, regenerated by `update-grub` |
| missing disk driver | one panic and reboot, then the fallback | handled by the initramfs |
| custom one-shot entry (`grub-reboot`) | a stale `initrdfail=1` can redirect it (Ubuntu's `00_header` code, inferred from the patch) | no fallback in the way |
| still needed | a custom entry made the default | a custom entry and initrd file of our own; the stock initrd is the distribution's, regenerated on kernel updates |

Not found: a measured boot-time saving; the list of drivers built into `linux-gcp`.

## Before or after the base initrd

Researched 2026-11-11. The question: is "appending the application archive fails" a kernel limitation, or
something about how the file was made?

**What the kernel does (confirmed).** Sources: mainline `init/initramfs.c`, read on GitHub master on 2026-11-11 and at
commit `067c5c24cef2`, and `Documentation/driver-api/early-userspace/buffer-format.rst`.

- `unpack_to_rootfs` walks the initrd as a sequence of segments.
- An uncompressed cpio segment is recognised only when `*buf == '0' && !(this_header & 3)`: its first byte is `0` and
  its offset from the start of the whole initrd is a multiple of 4. The buffer-format document states the same rule
  (`cpio_file := ALGN(4) + …`).
- NUL bytes between segments are skipped one at a time.
- After a compressed segment the offset moves to `saved_offset + my_inptr`, the number of bytes the decompressor
  consumed. zstd reports one frame's length, so data after a zstd segment is supported.
- A segment that is neither an aligned `0` nor a known compression magic ends with `invalid magic at start of
  compressed archive`.
- The kernel logs `Initramfs unpacking failed: <reason>` and does not stop. Files already unpacked, the whole base
  initramfs included, stay in place.

**What the files look like (confirmed).**

- Ubuntu 24.04's `mkinitramfs` writes the early cpios, then `cpio | zstd`, with no padding at the end, so the base
  initrd's length mod 4 is arbitrary.
- `cpio.Join` (`internal/setup/build/cpio`) and the scripts' `cat base app.cpio` add no padding either.
- GRUB zero-pads each file to 4 bytes only when it loads several (`initrd a b`, `grub-core/loader/linux.c`). One
  combined file gets no padding.

**Conclusion (inferred, not yet tested).**

- Appending fails when the base initrd's length is not a multiple of 4. That is a 3-in-4 chance for any given base
  file, and it is not a limit of zstd or of the kernel.
- Prepending works because our archive is padded to 512 bytes, so the base initrd that follows starts aligned.
- What this does not explain: an alignment failure should still let TDX boot (without the application). The note that
  the TDX guest "failed" may mean the application was missing rather than that the boot stopped. The serial log of that
  run was not kept.

**Consequence for SEV-SNP (inferred, important).**

- The 2026-11 SEV-SNP runs appended the archive and "the guests booted". That does not prove the application was
  unpacked: if the base length was not a multiple of 4, the kernel dropped the archive with one log line and booted
  the base initramfs.
- GRUB still measures the whole combined file, so INITRD matches the expected value either way. **A matching INITRD
  does not by itself prove the application ran.**
- To be settled before relying on `cpio.AppLast`:
  - check `stat -c %s` of the base initrd mod 4;
  - search the serial console for `Initramfs unpacking failed`;
  - check that the application's files exist after boot;
  - then pad the base to a multiple of 4 (`truncate -s %4`) and append again.
- Padding inside `cpio.Join` would change the combined file and so the measured value. It has not been made.

## The instance

`e2e/cloud/gcp-cvm.sh tdx create <roles…>` creates it.

| setting | value | why |
|---|---|---|
| machine type | `c3-standard-4` | TDX on GCP runs on C3 (Sapphire Rapids) or C4 (script header, checked against Google's docs 2026-11) |
| confidential type | `--confidential-compute-type=TDX` | |
| CPU platform | none | the script header says TDX must be created **without** `--min-cpu-platform` |
| maintenance | `--maintenance-policy=TERMINATE` | these VMs cannot live-migrate |
| image | `ubuntu-os-cloud/ubuntu-2404-lts-amd64` | must carry `TDX_CAPABLE`; the script checks it |
| zone | `us-central1-a` (default) | |

Our recorded guest, `attestamp-tdx-vendor` (read with `gcloud compute instances describe` and `gcloud compute disks
describe` on 2026-11-11):

| | |
|---|---|
| image | `ubuntu-os-cloud/global/images/ubuntu-2404-noble-amd64-v20261105` (the family moves; this is the exact image) |
| boot disk | 64 GB, `pd-balanced` |
| Shielded VM | Secure Boot **off**, vTPM on, integrity monitoring on |
| maintenance | `TERMINATE` |
| OS, kernel | Ubuntu 24.04.5, `7.0.0-1017-gcp` |
| quote interface | ConfigFS TSM, provider `tdx_guest` |

## From scratch: record and check one quote

1. `bash e2e/cloud/gcp-cvm.sh tdx create vendor`, then on the instance `sudo shutdown -h +240` as a safety stop.
2. Record a quote with ConfigFS TSM, as in `internal/tee/parse/tdx/testdata/README.md`.
3. Copy `quote.bin` back (`gcloud compute scp`, one file per call) and stop the instance.
4. `att-user ra-parse --doc quote.bin --format intel-tdx --nonce <128 hex chars>` prints MRTD and RTMR0–2.

## Not done yet

| what | why |
|---|---|
| `tee/request/<role>/tdx`, a `--extractor` in `cmd/ra-responder` | Phase 2; needs a running guest |
| `setup/build` and `setup/launch` for TDX | Phase 3: build the archive, put it first, boot a custom GRUB entry once, read the values back |
| TCB level, CRLs | collateral is not fetched (see above) |
| quote version 5 | the go-tdx-guest revision in `go.mod` decodes version 4 only; version 5 needs a newer one and its own fixture |
| the PCR→RTMR mapping and the VM-shape dependence | not confirmed on GCP |
| the append-alignment explanation | inferred from the kernel source; not yet tested by a boot (see "Before or after the base initrd") |
