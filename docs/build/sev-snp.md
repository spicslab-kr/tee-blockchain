# AMD SEV-SNP: what is measured, what changes it, and what is not done yet

**Status.** The verifying side works on recorded evidence: `ra-parse` checks a GCP SEV-SNP evidence pair and
extracts its values (`internal/tee/parse/snp`, formats `amd-sev-snp-initrd` and `amd-sev-snp-verity`). Requesting
evidence from a running guest, building the image and launching it are not implemented. A known weakness is
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

The binding was SHA-512 until **2026-11-11**, when it was changed to SHA3-512 by decision of the project lead. SHA-512
is what the 2026-11 test scripts used (`vendor/vendor_evidence.sh`); they were changed the same day, and their
SHA-512 versions are kept next to them as `<file>.sha512.bak` (outside this repository; see the night log). Results
from 2026-11-10 and earlier were produced with SHA-512. Evidence bound with SHA-512 is now refused
(`TestVerifySig_SHA512BindingIsRefused`).

The SHA3-512 binding applies to the GCP path, the only one we use; if an AWS path is ever added, its binding must be
changed to match.

### Only GCP

SEV-SNP is done on GCP only (decided 2026-11-11); AWS is used for Nitro Enclaves. **The AWS path is not ours:** on
AWS, SEV-SNP is an instance option and the vTPM (NitroTPM) an AMI attribute, so a guest with both needs a snapshot of
a running instance re-registered as an AMI with TPM support (`e2e/cloud/aws-snp.sh`, kept for reference). The 2026-11
AWS runs remain a valid record: they gave the same results as GCP, including the same-VM weakness below.

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

Why SHA-256: the verity hook (`verity_boot.sh`) extends only the SHA-256 bank, and the 2026-11 Auditor scripts
computed their expected values with SHA-256.

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

Each row changed a hash in the 2026-11 tests, with the same source code (observed on GCP and AWS).

| variable | what changed | how we kept it fixed |
|---|---|---|
| Go version | Go 1.22.2 and 1.23.4 gave different application binaries | the same Go version on both sides |
| build path | without `-trimpath`, two folders gave two different binaries | `-trimpath` |
| cpio tool | GNU cpio 2.13 (Amazon Linux 2023) and 2.15 (Ubuntu) packed the same files into different bytes (directory link counts) | our own cpio writer: `internal/setup/build/cpio` (sorted names, time 0, owner 0, fixed inode numbers), byte-identical to the scripts' `mkcpio.py` |
| distribution base initrd | `mkinitramfs` and `dracut` build a different file every time (it contains a random seed) | none possible. The Auditor uses the base initrd the Vendor publishes and rebuilds only the application part |
| folder permissions (umask) | the same files in a squashfs image gave a different image and root hash under umask 022 (folders 755) and umask 002 (folders 775) | build under the same umask. The script fixes file modes but not folder modes, so this is still open |
| squashfs timestamps | `mksquashfs` writes the build time by default | `-mkfs-time 0 -all-time 0`, file times set to 0 |
| dm-verity salt and UUID | `veritysetup format` picks them at random | fixed `--salt` and `--uuid` |

With all of the above fixed, the application, its cpio archive and the dm-verity image hashed the same on GCP
(Ubuntu 24.04, 26.04) and AWS (Amazon Linux 2023), across squashfs-tools 4.4, 4.6.1 and 4.7.5.

What else changes the registered values (from the evidence's structure; **not measured by changing each one**):

| value | changes with |
|---|---|
| MEASUREMENT | the virtual firmware GCP launches the guest with; GCP may update it |
| PCR4 | shim and GRUB, i.e. the OS image and its updates |
| KERNEL | the kernel package |
| CMDLINE | the boot entry. It names the root by an identifier that differs per disk (the file system's UUID on the stock entry with an initrd, the partition's PARTUUID on the initrdless one), so the Auditor must use the command line the Vendor publishes |
| INITRD | the base initrd and the application archive, and **where the archive goes**: on SEV-SNP it is appended after the base initrd (`cpio.AppLast`). The 2026-11 runs on GCP did that and the guests booted (observed). On TDX the same append failed to decompress and the archive goes first (`cpio.AppFirst`); why the two differ has not been established |

**Initrdless boot.** GCP's Ubuntu 24.04 image sets `GRUB_FORCE_PARTUUID`, and GRUB then boots the stock entry
**without any initrd** (confirmed: our first recording, `gcp-n2d-initrdless-*`, logs `GRUB_FORCE_PARTUUID set,
attempting initrdless boot.` and measures no initrd). An application in an initrd is then never loaded or measured.
`ra-parse` refuses evidence without an initrd measurement. The 2026-11 scripts avoided it by booting their own GRUB
entry (`grub-reboot`); turning the setting off, as below, makes the stock entry load the initrd.

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

Our recorded guest: Ubuntu 24.04.5, kernel `7.0.0-1017-gcp`, `/dev/sev-guest` present, ConfigFS TSM provider
`sev_guest`, SEV-SNP active in `dmesg`.

## From scratch: record and check one evidence pair

1. Create the instance: `bash e2e/cloud/gcp-cvm.sh snp create vendor`. On the instance, set a safety stop:
   `sudo shutdown -h +240`.
2. Build the recorder (`internal/tee/parse/snp/testdata/README.md`, "Recording another one") and copy it over with
   `gcloud compute scp`.
3. On the instance: `sudo ./record -out fx -nonce-text <your text>`. It writes `att.pb`, `report.bin`, `auxblob.bin`
   and `record.txt` (the nonce and REPORT_DATA).
4. Copy the files back and stop the instance: `gcloud compute instances stop <name> --zone <zone>`.
5. Pack and check them the way `cmd/snp_e2e_test.go` does: `snp.Encode(report, auxblob, att)`, then
   `att-user ra-parse --doc <file> --format amd-sev-snp-initrd --nonce <hex nonce>`.

On a stock GCP boot step 5 verifies both signatures and the nonce and then refuses, because the boot was initrdless.
Turn `GRUB_FORCE_PARTUUID` off first, as in "The boot setting changed on `attestamp-snp-vendor`" above; the stock
entry then loads the distribution initrd and `amd-sev-snp-initrd` yields five values. To boot an initrd that carries
an application, or the verity setup, use `option1-initrd/gcp_boot_option1.sh` (Option 1) or
`option2-dm-verity/gcp_boot_option2.sh` (Option 2) from the 2026-11 scripts, after replacing their hard-coded disk
UUID, PARTUUID, kernel version and UEFI boot entry number with the new instance's.

## Same VM

The binding proves that the AMD-signed report covers this vTPM evidence. It does not prove that both come from the
same VM: a vTPM attestation from an approved VM X, hashed into an SNP report requested on another VM Y, verifies.
That was reproduced on GCP and AWS in the 2026-11 runs (`relay-test/relay_attack.sh`; test matrix row T10). Nitro and
TDX return one document from one TEE and do not have this problem. It goes away only if the boot files are in the SNP
launch measurement itself (direct kernel boot; whether GCP allows it is not confirmed) or if the vTPM runs inside
the SNP guest's own trust boundary.

## Not done yet

| what | why |
|---|---|
| `tee/request/<role>/snp`, a `--extractor` in `cmd/ra-responder` | needs a running guest; the recorder in `testdata/record` is the starting point |
| `setup/build`, `setup/launch` for SEV-SNP | Phase 4b; the 2026-11 scripts do it by hand |
| a recorded verity boot | `gcp-n2d-*` booted the distribution initrd, so INITRD is tested on real evidence but PCR12 is not: no recording has the verity hook. Nor does any recording carry an application archive in its initrd |
| TCB minimums, CRLs | not applied; the platform's patch level is not judged |
| AMD chain errors as `ErrChain` | go-sev-guest checks the chain and the signature in one call |
| the seven 2026-11 tests on a new instance | `snpguest` and `gotpm`, which those scripts call, could not be installed on the instance (see the night log of 2026-11-10) |

AWS evidence is not on this list: SEV-SNP on AWS was decided against (see "Only GCP").

**The 2026-10/11 shell scripts.** The scripts the SEV-SNP tests use (`vendor/`, `auditor/`, `user/`, `option1-initrd/`,
`option2-dm-verity/`, `relay-test/`) are the scaffolding of the 2026-10 and 2026-11 experiments, and they live outside
this repository. The final implementation goes into Go, in `internal/setup/build/snp` and `internal/setup/launch/snp`.
Until then, whenever results of the seven tests are reported, the procedure that produced them is written into this
document in enough detail to reproduce them.

## Checking

There is no `--verify` for SEV-SNP yet. Until there is, build twice and on two machines, and compare the hashes of the
application, the cpio archive and the disk image before registering anything.
