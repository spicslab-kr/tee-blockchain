# Porting Attestamp from Nitro to confidential VMs

Status (2026-11-11, evening):

- Done:
  - Phase 0.3 and 0.4: the first fixtures.
  - Phase 1 (TDX parse) and phase 4a (SEV-SNP parse), with D3, D4 and D5 decided.
  - The anvil chain tests, which run.
  - Application fixtures for all four platform/option cells, which make T5 a code test (`docs/plan/app-fixture-recording.md`).
  - Phase 2 code (TDX and SEV-SNP request), tested on the recorded evidence; its run on a live guest (2.3) is pending.
- Phase 3 is partly done: 3.2–3.4 were measured in the recording session.
- Phase 5 (a locked guest) is designed below, not started.
- See `docs/plan/night-log.md`.

Clouds (decided 2026-11-11): **Nitro on AWS, TDX on GCP, SEV-SNP on GCP.** Each platform runs on one cloud. AWS is
used for Nitro Enclaves only; SEV-SNP on AWS is not pursued. The AWS SEV-SNP runs of 2026-11 stay in the record as
evidence that AWS gave the same results (D6 below), and `e2e/cloud/aws-snp.sh` is kept for reference.

Order: **Intel TDX first, AMD SEV-SNP second.** TDX attestation has the same shape as Nitro's —
one request, one signed document, measurements inside it. SEV-SNP is the exception: two documents
from two signers, joined by a hash. Building the TDX path first means SEV-SNP only has to add the
joining step on top of a route that already works.

## What stays and what is rewritten

| | |
|---|---|
| **Unchanged** | `contracts/evm/Registry.sol`, `internal/chain/register`, `internal/core/compare`, `internal/cli`, the wire protocol in `tee/request/*/wire.go` |
| **Changed after all** | `internal/chain/lookup`: `Reader` always looked up under `aws-nitro`, and the record key includes the platform, so it gained `Reader.Platform` |
| **New platform module** | `tee/parse/<plat>`, `tee/request/<role>/<plat>` |
| **Rewritten, not swapped** | `setup/build` (Nitro's is Docker + `nitro-cli` end to end), `setup/launch` (on a CVM, booting the VM *is* the launch), `cmd/ra-responder` (vsock and `/dev/nsm` are Nitro-only) |
| **Changed assumption** | the hand-off. See "Open decisions" below. |

## What Nitro gives today, for comparison

One CBOR COSE_Sign1 document per request. The fields the code uses:

| Field | Used for | Where |
|---|---|---|
| PCR0, PCR1, PCR2 | the registered and compared value | `tee/parse/aws/measure.go`, `imagePCRs = {0,1,2}` |
| PCR3, PCR4, PCR8 | **discarded** — they identify the machine, not the image | same file |
| `nonce` | replay protection | `tee/parse/aws/nonce.go` |
| `timestamp` | the time the certificate chain is judged at (a Nitro leaf lives ~3 h) | `tee/parse/aws/verify.go` |
| `certificate` + `cabundle` | chain to the AWS Nitro root | same file |
| COSE signature (ES384) | proof the platform signed it | same file |
| PCR0 all zero | debug-mode detection; the values are then worthless | `tee/parse/aws/measure.go` |

Four properties the port has to reproduce on each new platform:

1. One request returns one self-contained, signed piece of evidence.
2. The measurement is the identity — lookup is by measurement, so it must be reproducible.
3. The nonce is inside the signature, so freshness is proven by the hardware.
4. Debug mode is detected and refused.

## Phase 0 — groundwork (no hardware)

| # | Task | Pass criterion |
|---|---|---|
| 0.1 | Clone the repository from GitHub with `--recursive` (a zip extract lacks `.git` and the OpenZeppelin submodule) | `git status` works; done |
| 0.2 | `CLAUDE.md`, `.claude/settings.json`, `.claude/commands/`, `.claude/hooks/` | done |
| 0.3 | **Record one TDX quote and commit it as a fixture** | a file under `testdata/` plus the nonce used |
| 0.4 | Record one SEV-SNP report and one vTPM attestation from the same GCP instance | same |

**0.3 and 0.4 are the gate for everything else.** Nitro's whole test suite runs off one recorded
document. With the same fixtures in place, phases 1 and 4a below can be written and tested with no
instance running. `github.com/google/go-tdx-guest` ships a `testing/testdata` package that may be
usable for 0.3 as a starting point; a quote from our own GCP instance is still needed eventually,
because the PCK chain and the RTMR contents are what we have to parse.

## Phase 1 — TDX parse (no hardware, once 0.3 is done)

| # | Task | Pass criterion |
|---|---|---|
| 1.1 | `internal/tee/parse/tdx/`: quote decode, Intel PCK chain verification, nonce check, MRTD/RTMR extraction, DEBUG-bit check | `go test ./internal/tee/parse/tdx/...` against the fixture |
| 1.2 | `internal/tee/parse/{Vendor,auditor,user}/tdx.go` — three thin adapters | `go build ./...` |
| 1.3 | `TEEInfoKeys["intel-tdx"]`, `MeasurementNames["intel-tdx"]` in both `core/transfer` role copies | `go test ./internal/core/transfer/...` |
| 1.4 | One `case` per role in `cmd/{vendor,auditor,user}/parse.go` | `att-user ra-parse --doc <fixture> --format intel-tdx` prints the measurements |
| 1.5 | A `cmd/*_e2e_test.go` run driving the fixture through verify against a local anvil chain | `go test ./cmd/...` |

Library: `github.com/google/go-tdx-guest`. Its README describes `client`, `verify` and `rtmr`, and
the recommended way to fetch a quote is `GetQuoteProvider()` over ConfigFS — `OpenDevice()` on
`/dev/tdx_guest` is documented as deprecated. The README's `verify.TdxQuote` takes a `*pb.QuoteV4`;
**confirm whether the current release also handles QuoteV5 before writing against it.**

## Phase 2 — TDX and SEV-SNP request (code done; the live run needs instances)

**Why Phase 2 comes before locking the guest (Phase 5).** Runtime immutability's second layer
(`docs/build/runtime-immutability.md`) removes `sshd`. Once that is done, the way fixtures are recorded today, logging in
over SSH and running a recorder by hand, is impossible. The responder serving evidence over TCP is the only way to get
a quote out of a locked VM. So Phase 2 is a precondition of layer 2.

Built on 2026-11-11 to the approved design: the role-independent work once in `internal/tee/request/{tdx,snp}`, thin
adapters in `internal/tee/request/{Vendor,auditor}/{tdx,snp}`, and `request/{Vendor,auditor}/aws` untouched.

| # | Task | Pass criterion | State |
|---|---|---|---|
| 2.1 | `internal/tee/request/tdx` (ConfigFS TSM, REPORTDATA = nonce then zeros) and `internal/tee/request/snp` (GCE vTPM attestation, SHA3-512 into REPORT_DATA, SNP report and certificate table, the parse/snp container), with role adapters | the returned evidence parses with the Phase 1 and 4a parsers | **done, without hardware**: a fake TSM (go-configfs-tsm `faketsm`) answers only for the REPORTDATA each recording was made for, and the quote and evidence produced verify with the parsers |
| 2.2 | `--extractor tdx`, `snp-initrd`, `snp-verity` in `cmd/ra-responder` | the responder answers a request from another machine | code done; not run on a guest |
| 2.3 | End to end: `att-user verify` against a running TDX and SEV-SNP instance | exit 0 and a `✓ matches` line | needs instances |

**Listening.** `--listen` defaults to `tcp:127.0.0.1:5000`. A confidential VM is reached over its own network, so the
responder has to be started with `--listen tcp:0.0.0.0:5000` explicitly, and the firewall must allow port 5000 only
from the verifier's address (narrow the existing `attestamp-ra` rule's source range to that address rather than
`0.0.0.0/0`). The responder hands out evidence to anyone who connects; the evidence is not secret, but there is no
reason to offer it to the internet.

**The responder is shared with Nitro.** `att-ra-responder` is also layered into the Nitro EIF, so adding these
extractors changed its bytes, and Nitro PCR2 with them. The Nitro baseline has to be taken after commit `60de6b0` as
well. The request packages do not import the parsers, so no verifier code is linked into the responder.

## Phase 3 — TDX build and launch (needs two GCP TDX instances)

| # | Task | Pass criterion |
|---|---|---|
| 3.1 | Deterministic cpio of the application — fixed mtime, sorted order | two builds on two machines hash the same |
| 3.2 | Prepend the application cpio to the base initrd (appending broke decompression in 2026-10; on 2026-11-11 the cause was measured on SEV-SNP as missing 4-byte padding, and `cpio.Join` now pads; TDX + append was not re-tested) | the instance boots. **Done 2026-11-11**: four boots with `AppFirst`, the application unpacked every time (marker present) |
| 3.3 | Turn off `GRUB_FORCE_PARTUUID` so the initrd is actually read and measured | RTMR[2] changes when the initrd changes. **Done 2026-11-11**: RTMR2 changed with the application; MRTD, RTMR0, RTMR1 did not |
| 3.4 | Option 2: squashfs + dm-verity (ext4 is not reproducible even with fixed UUID and hash_seed) | two builds give the same root hash. **Done 2026-11-11 for the image**: the same image and root hash on the SEV-SNP and the TDX guest. Opening and mounting it inside a TDX boot is not built |
| 3.5 | `setup/launch/<role>/tdx/` — write a GRUB entry, boot into it once, read the measurement back | the measured value equals the built value |
| 3.6 | A `--verify` equivalent: build twice and refuse on a mismatch | a deliberate one-byte change fails the build |

## Phase 4 — AMD SEV-SNP

4a (no hardware, once 0.4 is done):

| # | Task | Pass criterion |
|---|---|---|
| 4.1 | Decide the container format that carries **two** documents in one `RADoc.Raw`, and the format string(s) | written down in `docs/build/sev-snp.md` |
| 4.2 | `internal/tee/parse/snp/`: AMD signature on the report, cloud AK chain on the vTPM quote, `Report Data == SHA3-512(vTPM evidence)` (SHA-512 until 2026-11-11), event-log replay against the signed PCRs, `POLICY.DEBUG`, TCB version | `go test` against the fixtures |
| 4.3 | Adapters, maps and CLI cases, as in 1.2-1.4 | `go build ./...`, `go test ./...` |

4b (needs instances): request, build and launch, mirroring phases 2 and 3, **on GCP only**. SEV-SNP on AWS (VLEK
signing, NitroTPM as the vTPM, an AMI re-registered with TPM support) was **decided against on 2026-11-11** and is not
part of 4b.

## Phase 5 — Lock the guest (runtime immutability, layers 2 and 3; after Phase 2)

Designed, not started. The reasoning and what is confirmed versus reasoned are in
`docs/build/runtime-immutability.md`. Order:

| # | Task | Pass criterion |
|---|---|---|
| 5.1 | Build a locked root image: the root file system as a squashfs + dm-verity image with no `sshd` and no other login, writable paths on `tmpfs`, and outbound traffic blocked by a firewall configuration that lives inside the image; `att-ra-responder` started at boot with `--extractor tdx` or `snp-verity` and `--listen tcp:0.0.0.0:5000` | two builds give the same root hash (the Option 2 settings: squashfs times 0, fixed salt and UUID, `umask 022`) |
| 5.2 | Boot it, with the root image's verity root hash on the kernel command line (`root=/dev/mapper/...` set up by the initrd against that hash) | the guest boots; SSH is refused; the responder answers from outside (Phase 2.3) |
| 5.3 | Record evidence from the locked guest through the responder, not over SSH | a fixture recorded with `att-user ra-request` from the verifier's machine |
| 5.4 | A test in which the verifier reads the root hash out of the measured command line (CMDLINE on SEV-SNP, the command-line event behind RTMR2 on TDX) and compares it with the hash of the image it was given | the hash in the evidence equals the published image's; a different image gives a different measurement |
| 5.5 | **Tamper case:** break the verity image on purpose (change one data block of the application image, and separately of the root image) and boot or read it | the change shows: PCR12 records `fail` (SEV-SNP Option 2), or the read fails with an I/O error, or the boot stops. This is the evidence for "if it is not there, it shows". Starting point: the 2026-11 script `option2-dm-verity/verity_tamper.sh` (flips one byte of the image at offset 5000 and reboots), outside the repository |

5.4 and 5.5 turn layer 3 from reasoning into a measured result; until they pass, `runtime-immutability.md` keeps
calling it reasoned.

Existing reference material: the SEV-SNP test scripts from the October–November runs
(`option1-initrd/`, `option2-dm-verity/`, `vendor/`, `auditor/`, `user/go-verifier/`,
`relay-test/`). They are not committed here. Known blockers before any of it is reusable:
hard-coded home paths, disk UUIDs, kernel versions and instance IPs; two separate Go modules
outside `attestamp`; expected values computed in Python; Korean comments.

## Open decisions

These change the design. D3, D4 and D5 are decided (see "Changed from the 2026-10-23 slides" below and
`docs/build/{tdx,sev-snp}.md`); D1, D2 and D6 are open.

**D1. The hand-off.** On Nitro the Auditor clones the Vendor's repository and rebuilds from
source, which is the project's central claim. On a CVM the distribution base initrd is not
reproducible — `mkinitramfs` and `dracut` write a random seed — so the Auditor has to take the
Vendor's base initrd as a binary. Options:

- (a) the Vendor publishes the base initrd; the Auditor reproduces only the application part.
  Fastest, matches what the test scripts already do, and weakens the Auditor's independence.
  If chosen, say so plainly in `README.md` and `docs/build/`.
- (b) build a reproducible base initrd ourselves. Keeps the original claim; much more work.
- (c) put the kernel and initrd into the launch measurement directly. `sev-snp-measure` accepts
  `--kernel`, `--initrd` and `--append`, so the path exists in principle; **whether GCP lets a
  tenant use it is unverified.** (AWS no longer matters here: SEV-SNP is GCP only since 2026-11-11.) If it works, the vTPM and the same-VM problem both
  disappear. Verify this before committing to (a) or (b).

**D2. Where the shared files live.** (a) and (b) both need the kernel, base initrd and disk image
to travel between Vendor and Auditor. Today `core/transfer` moves JSON and one image file through
git. Decide whether that is enough or a separate store is needed.

**D3. Option 1 vs Option 2 as separate platform strings.** Option 1 registers one value (the
initrd hash), Option 2 registers two (image hash + verifying-initrd hash). Because
`measurementHash` is computed over the sorted name/value list, one platform string with a variable
number of values produces hashes that cannot be compared. Two format strings
(`amd-sev-snp-initrd`, `amd-sev-snp-verity`) avoid this. **Decided (2026-11-11):** two format strings, but with more
values than one and two; see below.

**D4. What TDX registers.** The October slides are inconsistent: one says MRTD, RTMR[0], RTMR[1]
and RTMR[2] are all in scope; another says the registered value is RTMR[2] alone. Pick one.
MROWNER differs per machine and must be excluded either way, like Nitro's PCR3/4/8. **Decided (2026-11-11):** MRTD
and RTMR0–2; see below.

**D5. Freshness without a timestamp.** Nitro judges the certificate chain at the document's own
timestamp. If an SNP report carries no equivalent field, decide what replaces it. **Decided (2026-11-11):** there is
no time in either SNP document; chains are judged at verification time and freshness comes from the nonce only
(`docs/build/sev-snp.md`, "Time and freshness").

**D6. SEV-SNP cannot prove both documents came from the same VM.** Reproduced on GCP, and on AWS before AWS was
dropped for SEV-SNP (that AWS run stays as a record that the result is not GCP-specific): a
vTPM attestation from an approved VM combined with an SNP report from a different VM passes
verification. Nitro does not have this problem and TDX does not either. Until D1(c) or an in-VM
vTPM is used, SEV-SNP support is honest only if the limitation is documented.

## Changed from the 2026-10-23 slides

The registered values on both CVM platforms were decided differently from the 2026-10-23 presentation. The slides
have to be corrected; the reasons are collected here so that can be done from one place. Both changes come from the
same argument, which is also why Nitro registers PCR0, 1 and 2 together and not PCR2 alone: **a value that the boot
chain itself extends can be forged by a changed boot chain, unless the boot chain is registered too.**

### TDX: RTMR[2] alone → MRTD, RTMR0, RTMR1, RTMR2

| | |
|---|---|
| Original plan (slides) | RTMR[2] alone is the registered value (one slide; another listed MRTD and RTMR0–2 as in scope) |
| Changed to | four values, names `MRTD`, `RTMR0`, `RTMR1`, `RTMR2`, format `intel-tdx` |
| Reason | RTMR2 holds the kernel command line and the initrd, but RTMR2 is extended by the firmware, the boot loader and the kernel themselves. A changed kernel could extend exactly the expected RTMR2 while running something else; only MRTD (the firmware), RTMR0 (its configuration) and RTMR1 (boot loader and kernel) would show it. RTMR2 alone would prove nothing about who computed it |
| Left out, as before | MROWNER, MROWNERCONFIG, MRCONFIGID (set by the launcher; MROWNER is non-zero in our GCP quote), RTMR3 (run time), MRSEAM (the platform) |
| Cost | a GCP firmware update (MRTD) or a kernel update (RTMR1) changes the registered value; RTMR0 probably depends on the VM shape (not confirmed) |
| Where | `internal/tee/parse/tdx/measure.go` (`imageRTMRs`); `docs/build/tdx.md`, "The values"; tested on the recorded quote |

### SEV-SNP: the initrd hash → MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD

| | |
|---|---|
| Original plan (slides) | Option 1: one value, the initrd hash. Option 2: two values, the disk image hash and the verifying initrd's hash |
| Changed to | `amd-sev-snp-initrd`: `MEASUREMENT`, `PCR4`, `KERNEL`, `CMDLINE`, `INITRD` (5). `amd-sev-snp-verity`: the same and `PCR12` (6). In Option 2 the disk image is covered through its dm-verity root hash on the kernel command line (CMDLINE) and the result the verifying initrd extends into PCR12 |
| Reason | GRUB measures the initrd, and GRUB and the kernel are what load it. With INITRD alone, a changed boot loader or kernel could report the expected initrd hash while booting another. PCR4 covers shim and GRUB, KERNEL the kernel file, CMDLINE the command line (and in Option 2 the root hash). MEASUREMENT is the only value AMD signs (the virtual firmware); the other values are from Google's vTPM |
| Also decided | two format strings (D3), because the two lists have different lengths and `measurementHash` covers the name list |
| Cost | a kernel or GRUB update changes the registered value; the command line names the root disk, so the Auditor uses the command line the Vendor publishes |
| Where | `internal/tee/parse/snp/measure.go` (`initrdNames`, `verityNames`); `docs/build/sev-snp.md`, "The values"; all five initrd values tested on the recorded evidence (PCR12 has no recording yet) |

### Slide 7: "both options work on AWS and GCP" is wrong for SEV-SNP Option 1

| | |
|---|---|
| Claim (slides) | slide 7: both options work on AWS and GCP |
| Measured (2026-11-11, GCP, `attestamp-snp-vendor`) | with the unpadded append the 2026-11 scripts used (`cat i.bin app.cpio`), the base initrd was 50,100,283 bytes (**mod 4 = 3**); the kernel logged `Initramfs unpacking failed: invalid magic at start of compressed archive`; the application's marker was **absent**. The guest booted and INITRD changed with the application anyway, so the evidence looked right while the application never ran |
| Fix | zero-pad the base initrd to a multiple of 4 before appending. Measured: with the padding the archive unpacked and the marker held the application's text. `cpio.Join(AppLast)` now does this (commit `3c57d4c`); padding changes the measured initrd |
| What stands | SEV-SNP Option 2 (image on disk, verifying initrd, PCR12) worked as claimed: `status ok`, the application mounted, PCR12 as computed, recorded twice. TDX prepends (`AppFirst`), which needs no padding |
| Not known | whether the 2026-11 SEV-SNP Option 1 runs on GCP and AWS loaded the application: it depends on the length of the base initrd each run used, and those files were not kept |
| Where | `docs/build/sev-snp.md`, "Correction: SEV-SNP Option 1 as appended in 2026-11"; fixtures with boot logs in `internal/tee/parse/snp/testdata/gcp-n2d-opt1-*` and `gcp-n2d-opt1p-*` |

### Other decisions made after the slides

Whether the slides mention these was not checked; look for them when correcting the deck.

| | before | now | decided |
|---|---|---|---|
| SEV-SNP binding | REPORT_DATA = SHA-512 of the vTPM attestation (the 2026-11 scripts) | SHA3-512 | 2026-11-11 |
| SEV-SNP cloud | runs on GCP and AWS | GCP only; AWS for Nitro only | 2026-11-11 |
| Measurement names | — | `<NAME><n>` like Nitro's `PCR<n>` (`RTMR0`), and plain names for SNP (`KERNEL`) | 2026-11-10 |

## Prior work to read before deciding D1 and D6

- Narayanan et al., *Remote attestation of confidential VMs using ephemeral vTPMs*, ACSAC 2023
  (arXiv:2303.16463, doi:10.1145/3627106.3627112) — a vTPM running inside the SEV-SNP trust
  boundary, so the cloud provider need not be trusted.
- Wilke and Scopelliti, *SNPGuard: Remote Attestation of SEV-SNP VMs Using Open Source Tools*,
  SysTEX'24 (arXiv:2406.01186) — attesting the whole boot chain without a vTPM.

Only the abstracts have been read. Whether either covers binding a vTPM quote into an SNP report's
report data is still unchecked.

## The comparison this is all for

See [test-matrix.md](test-matrix.md). Nitro is the baseline and has to be run on real instances,
not quoted from the README. Phase 0 gains one step:

| # | Task | Pass criterion |
|---|---|---|
| 0.0 | Run the existing Nitro e2e suite end to end and record its results | every row of the matrix that applies to Nitro is filled from a real run |
| 0.0b | Add the JSONL result line to each e2e script, and the two missing tests (T8 debug mode, T9 replay) | `e2e-work/results/aws-nitro.jsonl` has one line per test |

0.0b comes before any TDX or SEV-SNP script is written, so all three are written against the same
output contract.
