# Porting Attestamp from Nitro to confidential VMs

Status (2026-11-16, evening):

- **Done, and run on real guests:**
  - Phases 0 to 2: fixtures, TDX and SEV-SNP parse (D3, D4, D5 decided), request; the live request ran on both guests
    (2026-11-12, then from another machine 2026-11-15).
  - Phase 3 (TDX) and Phase 4b (SEV-SNP) build and launch, both options/formats: install, reboot and `launch --check` on
    real guests (2026-11-15), and followed from the documents alone (the clean trial, 2026-11-16).
  - **The independent Auditor, both platforms** (Phase 3 item 6, Phase 4b item 5): on two guests made in one call from
    one image, the Auditor rebuilt on its own guest, got the Vendor's values, registered them with `audit
    --vendor-tee`, and a User's `verify` of the Vendor's guest passed; a one-byte change on the Vendor's side was
    refused (2026-11-16, `docs/results/sessions-2026-11-16/`). T2, T3 (TDX) and T6 live too.
  - **T10 reproduced** on SEV-SNP (2026-11-16): the relay verifies; a property of the platform, recorded in
    `docs/build/ra-comparison.md`, not fixed.
- **Decided and done 2026-11-16:** `audit` refuses to register without the Auditor's own build values, on every platform (an initrdless
  TDX guest had been registered that way and then trusted); `audit --vendor-tee` exits 12 when the Vendor's guest
  differs. **Planned after publication:** the TDX parser's own initrdless check, which needs the CCEL in TDX evidence
  (a wire-format change). All three in `docs/plan/build-info-gaps.md`, "Design questions to review", Q2 and Q3.
- **Measured in session 4 (2026-11-16):** T1, T3 (SEV-SNP), T7 on guests; T8 closed as not applicable on GCP
  (`docs/results/sessions-2026-11-16/session4-t1-t3-t7-t8.md`). The machine type changes the values (SEV-SNP
  MEASUREMENT with the vCPU count, TDX RTMR0), so both guides now require the Vendor's machine type; the zone changed
  nothing on SEV-SNP. No test-matrix cell needs a guest any more.
- **Open, not measured:** the zone on TDX; the dm-verity tamper case at boot (an image that does not match its root
  hash); TDX `AppLast`; a mitigation for T10.
- **Done 2026-11-16, after the guest work:** the project's port 22 moved from `default-allow-ssh` (`0.0.0.0/0`, deleted)
  to IAP only, `attestamp-ssh-iap`: `35.235.240.0/20`, limited to the `attestamp-ra` tag. ssh and scp through IAP were
  checked after the deletion, and plain ssh timed out (`e2e/cloud/README.md`, "SSH to the guests").
- Phase 5 (a locked guest) is designed below, not started.
- The day-by-day record: `docs/plan/night-log.md` and the trial and session reports.

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
| 0.2 | Working rules for this repository | done; kept outside the repository since `5b310cf`, and the rules a contributor needs are in `CONTRIBUTING.md` and `STRUCTURE.md` |
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
| 2.2 | `--extractor tdx`, `snp-initrd`, `snp-verity` in `cmd/ra-responder` | the responder answers a request from another machine | code done. **Answered on the guest over 127.0.0.1, 2026-11-12** (`tdx`, `snp-initrd`); **from another machine on 2026-11-15** (TDX: `docs/plan/trial-2026-11-16.md`) |
| 2.3 | End to end: `att-user verify` against a running TDX and SEV-SNP instance | exit 0 and a `✓ matches` line | **Passed 2026-11-12, all on one guest** (`e2e/cvm/in-guest-verify.sh`, 11 of 11 checks each): `amd-sev-snp-initrd` on a stock boot; `intel-tdx` on a stock boot and again on a boot with the dm-verity application mounted (3.4) |

**How 2.3 was run.** Without a firewall rule, everything runs on the guest and talks over `127.0.0.1`: the responder
(as root), anvil 1.8.0, `att-auditor registry-deploy` and `audit`, `att-user verify`. Two consequences to keep in mind:

- The Auditor's "build" values are the first live document's own values, because no build exists on the guest yet
  (Phase 3.5). The run proves request, verification, registration and lookup on fresh evidence; it does not prove a
  rebuild.
- The verifier ran inside the attested guest. A verifier on another machine is the remaining part of 2.2.

`amd-sev-snp-verity` was refused on the same boot, as it must be: the boot had no `attestamp.roothash=` on the command
line, and `ra-parse` says "the kernel command line has no attestamp.roothash=, so no disk image was checked". Passing it
needs a verity boot.

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
| 3.4 | Option 2: squashfs + dm-verity (ext4 is not reproducible even with fixed UUID and hash_seed) | two builds give the same root hash. **Done 2026-11-11 for the image**: the same image and root hash on the SEV-SNP and the TDX guest. **Done 2026-11-12 on TDX at boot**: the initrd verifies the image against its root hash, opens it with dm-verity and mounts it read-only on `/opt/app-verity` (status `V`), for v1 and v2; an image that does not match its root hash is not opened (`fail`, nothing mounted), but the boot continues. Fixtures `gcp-c3-opt2m-*`. Still a test setup: scripts in `tdx/testdata/record/`, not `setup/build` |
| 3.5 | `setup/launch/<role>/tdx/` — write a GRUB entry, boot into it once, read the measurement back | the measured value equals the built value. Designed 2026-11-15, below |
| 3.6 | A `--verify` equivalent: build twice and refuse on a mismatch | a deliberate one-byte change fails the build. Designed 2026-11-15, below |

### Phase 3 design: TDX build and launch in the tool (2026-11-15)

Until now Phase 3 existed as test scripts under `testdata/`; `att-vendor build` and `launch` were Nitro only. This is
the design for putting TDX into the tool. **TDX only**: SEV-SNP (4b) follows the same shape once this is in place.

**The same skeleton on every platform.** Nitro's `build` (`internal/setup/build/<role>/aws/build.go`) is five steps.
The TDX build keeps the five, so all three platforms read alike:

| step | Nitro (`aws`) | TDX (`tdx`) |
|---|---|---|
| 1 tools | install `docker`, `nitro-cli` | check `mksquashfs` and `veritysetup` (Option 2 only) and record their versions; nothing is installed |
| 2 the software | `docker build` of the Vendor's Dockerfile | read the application tree (`--app-dir`) as files; a link or special file is refused, as `cpio.FromDir` does |
| 3 what we add | the attestation responder, layered into the image | the **boot marker** (`conf/conf.d/zz-attestamp-app-marker`, the one `mkappinitrd` used) that lets `launch` prove the archive was unpacked; for Option 2, the squashfs + dm-verity image of the tree |
| 4 make it reproducible | repack the docker layers in name order with time 0 | write the cpio archive with fixed metadata (`internal/setup/build/cpio`) and put it **before** the Vendor's base initrd (`cpio.AppFirst`) |
| 5 the measured thing | `nitro-cli build-enclave` → EIF and PCR0·1·2 | the combined initrd and its **SHA-384**, the digest GRUB extends into RTMR2 |

**Where they differ.**

- **Docker → EIF becomes files → cpio/squashfs → initrd.** There is no container: the application is a tree of files
  that lands under `/opt/app` in the initramfs (Option 1), or inside a read-only squashfs image verified by dm-verity
  (Option 2, `/opt/app/{app.img,app.hash,roothash}` in the archive, mounted read-only on `/opt/app-verity` by the
  base initrd's local-bottom script).
- **The responder is not in the measured image.** On Nitro it has to be inside, because an enclave has no other way
  in. In a confidential VM it runs in the guest OS and only relays a quote the hardware signs, so it cannot forge one;
  its bytes stay out of the measurement, and Go version and `-trimpath` matter only for the application itself. When
  the guest is locked (Phase 5), the responder moves into the locked root image.
- **The build cannot produce the registered values.** Nitro's build ends with PCR0·1·2. A TDX build ends with the
  initrd's digest only: MRTD is the firmware, RTMR0 its configuration, RTMR1 the boot loader and kernel, and RTMR2
  also holds the GRUB commands and the kernel command line, all of which exist only on the machine. So **`launch` boots
  once and reads the four values back**; it is Nitro's `launch --build-measurements` and the measuring step in one.
- **The base initrd is the Vendor's binary (D1 option (a)).** `mkinitramfs` writes a random seed, so the Auditor uses
  the Vendor's published base initrd and reproduces only the application part. For Option 2 that base must carry the
  dm-verity hook (`internal/tee/parse/tdx/testdata/record/initramfs-hook.sh` and `initramfs-local-bottom.sh`).
- **One format string.** Option 1 and Option 2 both move only RTMR2 (`docs/build/tdx.md`), so both register
  `intel-tdx` with MRTD, RTMR0, RTMR1, RTMR2. The build record says which option was built.

**Shell or Go (brief item 2.2).**

| material | decision | why |
|---|---|---|
| `internal/setup/build/cpio` | used as it is | already the reproducible writer, pinned by the mkcpio golden files |
| `cpio/testdata/mkappinitrd` | its logic (entries, marker, `cpio.Join`) moves into the build; the command stays in `testdata` as the recorder that made the fixtures | the product must not depend on a `testdata` main package; the recorder stays as the record of how the fixtures were made |
| `make-verity-image.sh` | **ported to Go**, still calling `mksquashfs` and `veritysetup` through exec | the fixed parameters (squashfs flags, times 0, 4096-byte blocks and padding, salt, UUID, modes) decide the root hash, so they belong in reviewed, tested code in one place. The tools themselves stay: no Go writer is known to produce the same bytes, and writing one would be a new reproducibility risk. A test reads the script and fails if its parameters and the Go constants drift apart; the script stays as the record of the measured images |
| `make-grub-entry.sh` | **ported to Go** (the entry is a text transform of the stock entry; `update-grub` is exec'd) | the entry decides the GRUB commands that go into RTMR2; a Go function is testable without a VM |
| `initramfs-local-bottom.sh`, `initramfs-hook.sh` (TDX) | **stay shell** | they run inside the initramfs, under busybox `sh`, where there is no Go. They are part of the Vendor's base initrd, not of `build` |
| `snp/testdata/record/initramfs-local-bottom.sh` | untouched | SEV-SNP (PCR12); 4b |

**Code layout**, the "one core + role adapters" shape of `tee/parse` and `tee/request`:

| path | what |
|---|---|
| `internal/setup/build/tdx/` | the build: five steps, Option 1 and 2, `--verify` |
| `internal/setup/build/{Vendor,auditor}/tdx/` | thin adapters |
| `internal/setup/launch/tdx/` | the launch: GRUB entry, install, and the post-boot check (quote, event-log replay, compare) |
| `internal/setup/launch/{Vendor,auditor}/tdx/` | thin adapters |
| `internal/setup/{build,launch}/*/aws/` | **unchanged** |

**Launch (3.5), in two runs separated by a reboot.**

1. `launch --platform intel-tdx --initrd out/initrd-app.img`: refuses if `GRUB_FORCE_PARTUUID` is set (the initrd
   would never be loaded, `docs/build/tdx.md`), copies the initrd into `/boot`, writes a GRUB entry made from the stock
   `Ubuntu` entry with only the title and initrd file changed, makes it the default, runs `update-grub`. With `--reboot`
   it reboots.
2. After the boot, `launch --platform intel-tdx --check`: a fresh quote over a random nonce from ConfigFS TSM; the
   quote is verified with `internal/tee/parse/tdx` (chain to the Intel root, nonce, debug bit); the CC event log
   (`/sys/firmware/acpi/tables/data/CCEL`) is replayed against the quote's RTMR0–2; in RTMR2's events, the file GRUB
   loaded with `initrd` must have the build's SHA-384; the boot marker must be present (and for Option 2
   `/run/attestamp-status` must read `ok <root hash>`). Then MRTD and RTMR0–2 go to `measurements.json`.
   **Pass: the measured initrd digest equals the built one.** A digest that does not match, a log that does not replay,
   or a missing marker stops it.

**`--verify` (3.6).** The build runs steps 2–5 a second time from the files, into a temporary folder, and compares the
combined initrd byte for byte (for Option 2 also the image, hash tree and root hash). A difference fails the build and
names what differed: the archive entries, or the image. Pass: a one-byte change between the two builds fails it.

**What can be tested without a VM (2.6).** Everything in the build (Option 2 with fake `mksquashfs`/`veritysetup`
runners), `--verify` with a one-byte change, the GRUB entry text, the install step against a temporary root, the quote
check on the recorded quotes, and the event-log replay on a constructed log. **What needs a VM** is listed in
"Phase 3: what waits for a VM" below.

### Phase 3: where it stands (2026-11-15)

| # | state | proved by |
|---|---|---|
| 3.5 build | **done without hardware**, both options: `internal/setup/build/tdx` and adapters (`63d138f`), `att-vendor`/`att-auditor build --platform intel-tdx` (`fc0f460`) | `internal/setup/build/tdx` tests (also run on Linux under WSL); `TestCmd_E2E_TDXBuild_VendorAndAuditorAgree` (on Linux the two roles' builds are byte-identical) |
| 3.5 launch | **done, and run on real guests** (2026-11-15, again 2026-11-16 on two guests): install and the post-boot check (`400d2cd`, CLI `fc0f460`) | GRUB entry and install against a temporary root; a recorded quote verifies and a foreign event log is refused; the replay, the initrd comparison and the marker checks on constructed input |
| 3.6 `--verify` | **done**: a one-byte change between the two builds fails the build and names `/opt/app/app` (Option 1) or the root hash (Option 2); a replaced base initrd is named too | `TestVerify_OneByteChangedFailsTheBuild`, `TestVerify_ChangedBaseInitrdIsNamed` |

### Phase 3: what waits for a VM

Each item says what to run and what must come out. The pass criterion of 3.5 is the last line of the second step.

1. **Done 2026-11-15, no VM** (`docs/plan/trial-2026-11-16.md`): in a clean Ubuntu 24.04 WSL with squashfs-tools 4.6.1 and cryptsetup 2.7.0, `--option 2` on the recordings' v1 application gave root hash `e29ec085…af0c9` and `app.img` `28bdfcff…ec1c`, the measured values. What was asked: **The build with the real tools** (any Ubuntu 24.04 machine with squashfs-tools 4.6.1 and cryptsetup 2.7.0; not a
   TDX VM). `att-vendor build --platform intel-tdx --option 2 --app-dir <a folder holding one file app containing
   "attestamp app v1">` must give root hash `e29ec08585853c0fec829d6a5d7fd49d73dec6f4af88e5055752a52f6c3af0c9`, the
   value `make-verity-image.sh` gave on both guests on 2026-11-11 (`tdx/testdata/gcp-c3-opt2-v1-boot.txt`). The Go port
   lays the image out the same way (application under `opt/app`, every mode 755, every time 0, the same flags, padding,
   salt and UUID), and a test keeps the flags equal to the script's, but only the real tools can show the bytes are the
   same. WSL on the development machine had squashfs-tools 4.7.5 and no veritysetup, so this was not run.
2. **Done 2026-11-15** (`docs/plan/trial-2026-11-16.md`): on a new guest, install, reboot and `--check` passed; the CC event log replayed, GRUB's initrd event carried the path of its `initrd` command and the SHA-384 of the file, the marker was there. What was asked: **Launch on a TDX guest** (`gcp-cvm.sh tdx create vendor`, with approval). On the guest, `GRUB_FORCE_PARTUUID`
   commented out (the install refuses otherwise):
   ```
   sudo ./att-vendor build --platform intel-tdx --app-dir app --base-initrd /boot/initrd.img-$(uname -r) --option 1 --verify
   sudo ./att-vendor launch --platform intel-tdx --initrd out/initrd-app.img --reboot
   # after the boot
   sudo ./att-vendor launch --platform intel-tdx --check
   ```
   Things only the guest can show, each a reason the check might stop:
   - `/sys/firmware/acpi/tables/data/CCEL` exists and **replays** to the quote's RTMR0–2 with go-eventlog (it replays
     Google's own TDX logs in its tests; ours is not yet confirmed);
   - GRUB's file event for the initrd is named with the same path as its `initrd` command
     (`/boot/initrd-attestamp.img`), and its digest is the **SHA-384 of the file**; on SEV-SNP's vTPM the same
     convention holds (`tee/parse/snp/measure.go`), on TDX's CC measurement it is assumed;
   - `/dev/attestamp-app-marker` is present after the boot.
   **Pass: the check prints "✓ the same" and writes `out/measurements.json`.** Its RTMR2 differs from the stock boot's;
   MRTD, RTMR0 and RTMR1 equal the stock boot's (as the 2026-11-11 recordings showed).
3. **Done 2026-11-15**: check passed, `/run/attestamp-status` `ok <root hash>`, `dmsetup` `V`, the application readable; the root hash equal to the one built in WSL. What was asked: **Option 2 on the guest.** The base initrd built with `tdx/testdata/record/initramfs-hook.sh` and
   `initramfs-local-bottom.sh` installed (as for the `gcp-c3-opt2m-*` recordings), then the same three commands with
   `--option 2`. Pass as above, and `/run/attestamp-status` reads `ok <root hash>`.
4. **Measured 2026-11-15: the values match.** Two guests made five days apart from one image have the same root UUID, PARTUUID and stock base initrd (they come with the image), so the same application gave the same initrd and **the same MRTD and RTMR0–2**. Both sides must pin the exact image, not the family: since 2026-11-16 `gcp-cvm.sh` takes `IMAGE=<exact name>` and prints the name it resolved (`3520a56`), and both procedures say why ("Use the same image as the other side"). Not measured: whether RTMR0 follows the machine type. What was asked: **The Auditor's half.** A second TDX guest builds from the same files and the Vendor's base initrd: the initrd
   SHA-384 must be equal (`build-value.json`), and after its own launch the four values must equal the Vendor's, if both
   guests have the same firmware, kernel and command line. The command line names the root disk's UUID, which differs
   per disk (`docs/build/tdx.md`, "Variables"): **expect RTMR2 to differ between two guests until the command line is
   fixed**, for example by a root UUID set at image build. This is the open question that decides whether an
   independent rebuild can ever match on TDX, and it is the first thing to measure.
5. **Done 2026-11-15**: from a WSL on another machine, `ra-request`, `audit` (registered on a local anvil) and `verify` (exit 0, `✓ matches`). What was asked: **The verifier from another machine.** `att-user verify` against the guest's responder (Phase 2.2), with
   `measurements.json` registered by the Vendor or audited by the Auditor.
6. **Done 2026-11-16, both options, every pass below met** (`docs/results/sessions-2026-11-16/intel-tdx.md`, at
   `1b981df`): two guests from `ubuntu-2404-noble-amd64-v20261105` in one call; equal builds and equal values; `audit
   --vendor-tee` registered the Auditor's own value; the User's `verify` of the Vendor's guest exit 0; the negative
   control refused. The procedure needed one fix (A2's copy of `app`). What was asked: **the Auditor's half as a
   procedure** (`docs/build/tdx.md`, "The Auditor's half", A1 to A9), for each option:
   1. **Two guests from one exact image**, the same machine type and zone, in one call so they cannot differ:
      `IMAGE=<exact name> bash e2e/cloud/gcp-cvm.sh tdx create vendor auditor` (with approval; two billable VMs).
      Pass: `gcp-cvm.sh tdx status` shows the same image on both disks.
   2. **Each builds and launches.** The Vendor: getting-started steps 4 to 9 with `att-vendor`. The Auditor: A3 to A7
      with `att-auditor`, the Vendor's `app` folder and, for Option 2, the Vendor's `base-initrd-v.img`. Pass: both
      `launch --check` print `✓ the boot measured the built initrd`, and `initrd_sha384` (Option 2: also `root_hash`)
      is equal in the two `out/build-value.json`.
   3. **The two guests' values are equal.** Fetch both `out/measurements.json`; `att-auditor compare --attested
      vendor-measurements.json --expected auditor-measurements.json` exits 0. Pass: MRTD, RTMR0, RTMR1 and RTMR2 equal.
   4. **The Auditor registers its own values** (A8): `audit --tee-addr <auditor-guest>:5000 --build-measurements
      <its own> --vendor-tee <vendor-guest>:5000`. Pass: `identical: the guest runs exactly what I built`, `same: the
      Vendor's guest runs exactly what I built`, `PASS: audited and registered`, exit 0.
   5. **The User verifies the Vendor's guest** against that registry (A9). Pass: exit 0, `✓ matches`.
   6. **Negative control**: one byte changed in the Vendor's `app`, rebuilt and relaunched on the Vendor's guest only.
      Pass: `audit --vendor-tee` prints `the Vendor's guest runs something else`, and the User's `verify` against the
      Auditor's registry exits 1.

   Record the verdicts as JSONL lines (T4 for 4 and 5, T5 for 6) so they land in `docs/plan/test-matrix.md`.

## Phase 4 — AMD SEV-SNP

4a (no hardware, once 0.4 is done):

| # | Task | Pass criterion |
|---|---|---|
| 4.1 | Decide the container format that carries **two** documents in one `RADoc.Raw`, and the format string(s) | written down in `docs/build/sev-snp.md` |
| 4.2 | `internal/tee/parse/snp/`: AMD signature on the report, cloud AK chain on the vTPM quote, `Report Data == SHA3-512(vTPM evidence)` (SHA-512 until 2026-11-11), event-log replay against the signed PCRs, `POLICY.DEBUG`, TCB version | `go test` against the fixtures |
| 4.3 | Adapters, maps and CLI cases, as in 1.2-1.4 | `go build ./...`, `go test ./...` |

### Phase 4b design: SEV-SNP build and launch (2026-11-15)

The same shape as TDX (`internal/setup/{build,launch}/snp/` core, `{Vendor,auditor}/snp/` adapters, `build` and
`launch --platform amd-sev-snp-initrd|amd-sev-snp-verity`), and where TDX and SEV-SNP do the same thing they now share
one implementation: the dm-verity image maker moves from `setup/build/tdx` to `setup/build/verity`, and the GRUB entry
and install from `setup/launch/tdx` to `setup/launch/grub`. What differs from TDX:

| | TDX | SEV-SNP |
|---|---|---|
| formats | one, `intel-tdx`, for both options | **two**: `amd-sev-snp-initrd` (Option 1, 5 values) and `amd-sev-snp-verity` (Option 2, 6 values); the option picks the format |
| Option 1 placement | archive **before** the base (`cpio.AppFirst`) | archive **after** the base zero-padded to 4 bytes (`cpio.AppLast`), as the `opt1p` recordings were made; without the padding the kernel drops the archive (`docs/build/sev-snp.md`, "Correction") |
| Option 2 image | inside the initrd, covered by RTMR2 | **on disk** (`/var/lib/attestamp/app.img`, `app.hash`); the initrd is the Vendor's base built with the hook (`snp/testdata/record/initramfs-hook.sh`, `initramfs-local-bottom.sh`), and the root hash goes on the **kernel command line** (`attestamp.roothash=`) |
| Option 2 at boot | the hook opens and mounts the image; nothing is extended | the hook verifies, opens and mounts the image and **extends vTPM PCR12** with SHA-256(`attestamp-verity-<ok\|fail>:` root hash); so the build can predict PCR12 = SHA-256(32 zero bytes ‖ SHA-256(`attestamp-verity-ok:` ‖ root hash)) |
| what the build can predict | the initrd's SHA-384 | the initrd's **SHA-256** (INITRD) and, for Option 2, the root hash and **PCR12**. MEASUREMENT, PCR4, KERNEL, CMDLINE come from the machine |
| the evidence | one quote | **two documents**: the SNP report and the vTPM attestation, bound by REPORT_DATA = SHA3-512(vTPM attestation); the check takes it from `tee/request/snp` and verifies it with `tee/parse/snp`, which already replays the vTPM event log and returns INITRD, CMDLINE and PCR12 |
| launch's own event-log code | yes (CCEL, go-eventlog) | **none**: `tee/parse/snp` already does it (go-tpm-tools) |
| marker | `/dev/attestamp-app-marker` | Option 1: the same marker, in the appended archive. Option 2: `/run/attestamp-status` reads `ok` (the SNP hook writes the status without the root hash) |

**Pass criteria.** Build: as TDX (`--verify`, a one-byte change fails). Launch check: INITRD from verified evidence
equals the built SHA-256; for Option 2 also PCR12 equals the predicted value, and the command line carries the built
root hash (`tee/parse/snp` refuses a verity format without it).

**Without a VM.** Unlike TDX, the launch check can be run end to end on recorded evidence: the `opt1p` and `opt2`
recordings' boot logs give the combined initrd's sha256 (`a4504052…`) and the root hash (`e29ec085…`) the guest built,
so a check fed the recorded evidence and those build values must pass, and fail for any other value.

### Phase 4b: where it stands (2026-11-15)

| | state | proved by |
|---|---|---|
| shared code | the verity image (`setup/build/verity`) and the GRUB install (`setup/launch/grub`) moved out of TDX (`66e28d9`) | the TDX tests unchanged, and `verity`/`grub` tests |
| build | **done without hardware**, both formats, `--verify` (`ce08d93`, CLI `cac5d28`) | Option 1's archive from the recordings' one-file application is **byte-identical** to the archive the `opt1p-v1` and `opt1p-v2` guests booted (`TestBuild_Option1_TheArchiveTheGuestBooted`); a one-byte change fails `--verify`; run on Linux too |
| launch check | **done on recorded evidence** (`b5d7086`) | `opt1p-v1` passes with its INITRD `a4504052…`; `opt2-v1` passes with INITRD `b0fd172d…` and the PCR12 predicted from root hash `e29ec085…`; v2's values are refused; a replayed nonce is refused |
| launch install | **done, and run on real guests** (2026-11-15, again 2026-11-16 on two guests, both formats); in code against a temporary root | `TestInstall_Option2PutsImageAndRootHash` |

### Phase 4b: what waits for a VM

1. **Done 2026-11-15, no VM**: root hash `e29ec085…` (v1) and `a2f4df92…` (v2), and the predicted PCR12 `470379c7…` equals the recorded `opt2-v1` PCR12 (`docs/plan/trial-2026-11-16.md`). What was asked: **The real tools for Option 2** (any Ubuntu 24.04, as for TDX): `build --platform amd-sev-snp-verity` with the
   one-file v1 application must give root hash `e29ec085…af0c9`; the check above already showed that this root hash
   gives the recorded PCR12.
2. **Done 2026-11-15** (`docs/plan/trial-2026-11-16.md`): on a new guest, both options: install, reboot and `--check` passed; Option 2's PCR12 in the verified evidence was the predicted value; `audit` and `verify` passed from another machine; the undo returned the stock boot. What was asked: **Install and reboot on a SEV-SNP guest** (`gcp-cvm.sh snp create vendor`, with approval), `GRUB_FORCE_PARTUUID`
   commented out. Option 1: `build --platform amd-sev-snp-initrd --app-dir app --base-initrd /boot/initrd.img-$(uname
   -r) --verify`, `launch --platform amd-sev-snp-initrd --reboot`, then `launch … --check`. **Pass: "✓ the same".**
   Option 2: a base initrd built with `snp/testdata/record/initramfs-hook.sh` and `initramfs-local-bottom.sh`
   installed, then the same with `amd-sev-snp-verity`; pass also needs `/run/attestamp-status` `ok` and PCR12 as
   predicted.
3. **Measured 2026-11-15: CMDLINE is the same.** The root UUID comes with the image, so two guests of `ubuntu-2404-noble-amd64-v20261105` gave the same MEASUREMENT, PCR4, KERNEL, CMDLINE and INITRD for the same application. Pin the exact image on both sides: `gcp-cvm.sh` `IMAGE=` since 2026-11-16 (`3520a56`). Not measured: whether MEASUREMENT follows the machine type. What was asked: **The Auditor's half**: as on TDX, the command line names the root disk (`root=UUID=`), so CMDLINE will differ
   between two guests' disks; and the Auditor uses the Vendor's base initrd (D1 (a)). Measure first whether CMDLINE
   can be made equal.
4. **T10** stays as it is: the build and launch do not change what the evidence can say about the same VM. **Reproduced 2026-11-16** on the two guests of item 5: the relay verifies (`docs/build/ra-comparison.md`, "The SEV-SNP relay, measured").
5. **Done 2026-11-16, both formats, every pass below met** (`docs/results/sessions-2026-11-16/amd-sev-snp.md`, at
   `57b4350`), Option 2 on two guests included; and T10 reproduced on the same two guests (item 4). What was asked:
   **the Auditor's half as a procedure** (`docs/build/sev-snp.md`, "The Auditor's half", A1 to A9), for each format
   (`amd-sev-snp-initrd`, `amd-sev-snp-verity`):
   1. **Two guests from one exact image**, the same machine type and zone, in one call:
      `IMAGE=<exact name> bash e2e/cloud/gcp-cvm.sh snp create vendor auditor` (with approval; two billable VMs).
      Pass: `gcp-cvm.sh snp status` shows the same image on both disks.
   2. **Each builds and launches.** The Vendor: getting-started steps 4 to 9 with `att-vendor`. The Auditor: A3 to A7
      with `att-auditor`, the Vendor's `app` folder and, for Option 2, the Vendor's `base-initrd-v.img`. Pass: both
      `launch --check` print `✓ the boot measured the build`, and `initrd_sha256` (Option 2: also `root_hash` and
      `pcr12`) is equal in the two `out/build-value.json`.
   3. **The two guests' values are equal.** `att-auditor compare --attested vendor-measurements.json --expected
      auditor-measurements.json` exits 0. Pass: MEASUREMENT, PCR4, KERNEL, CMDLINE, INITRD (and PCR12) equal.
   4. **The Auditor registers its own values** (A8), with `--vendor-tee`. Pass: `identical: the guest runs exactly what
      I built`, `same: the Vendor's guest runs exactly what I built`, `PASS: audited and registered`, exit 0.
   5. **The User verifies the Vendor's guest** against that registry (A9). Pass: exit 0, `✓ matches`.
   6. **Negative control**: one byte changed in the Vendor's `app`, rebuilt and relaunched on the Vendor's guest only.
      Pass: `audit --vendor-tee` prints `the Vendor's guest runs something else`, and `verify` exits 1.

   Record the verdicts as JSONL lines (T4 for 4 and 5, T5 for 6).

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
| 5.5 | **Tamper case:** break the verity image on purpose (change one data block of the application image, and separately of the root image) and boot or read it | the change shows: PCR12 records `fail` (SEV-SNP Option 2), or the read fails with an I/O error, or the boot stops. This is the evidence for "if it is not there, it shows". Starting point: the TDX tamper boot of 2026-11-12 (v2's image under v1's root hash: `fail`, nothing mounted, the boot went on; `docs/plan/night-log.md`). Still to do: the same on SEV-SNP, a changed data block under an unchanged root hash, and the root image. (*Record:* the 2026-11 script for this flipped one byte at offset 5000; it is no longer available) |

5.4 and 5.5 turn layer 3 from reasoning into a measured result; until they pass, `runtime-immutability.md` keeps
calling it reasoned.

*Record:* the October–November SEV-SNP runs used test scripts outside this repository (`option1-initrd/`,
`option2-dm-verity/`, `vendor/`, `auditor/`, `user/go-verifier/`, `relay-test/`). Their author has left the lab and
they are no longer available (build-info gaps #5, #6); nothing here depends on them. What replaced them is in
`docs/plan/app-fixture-recording.md` and the recordings under `internal/tee/parse/snp/testdata/`. The one result not
yet replaced is T10 (two machines' evidence combined); its procedure is "Reproducing T10" in the same document.

## Open decisions

These change the design. D3, D4 and D5 are decided (see "Changed from the 2026-10-23 slides" below and
`docs/build/{tdx,sev-snp}.md`); D1, D2 and D6 are open.

**D1. The hand-off.** On Nitro the Auditor clones the Vendor's repository and rebuilds from
source, which is the project's central claim. On a CVM the distribution base initrd is not
reproducible — `mkinitramfs` and `dracut` write a random seed — so the Auditor has to take the
Vendor's base initrd as a binary. Options:

- (a) the Vendor publishes the base initrd; the Auditor reproduces only the application part.
  Fastest, matches what the 2026-11 test scripts did and what `docs/plan/app-fixture-recording.md` does, and weakens
  the Auditor's independence.
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
| Measured (2026-11-11, GCP, `attestamp-snp-vendor`) | with the unpadded append the 2026-11 scripts used (*record:* `cat i.bin app.cpio`; the scripts are no longer available), the base initrd was 50,100,283 bytes (**mod 4 = 3**); the kernel logged `Initramfs unpacking failed: invalid magic at start of compressed archive`; the application's marker was **absent**. The guest booted and INITRD changed with the application anyway, so the evidence looked right while the application never ran |
| Fix | zero-pad the base initrd to a multiple of 4 before appending. Measured: with the padding the archive unpacked and the marker held the application's text. `cpio.Join(AppLast)` now does this (commit `3c57d4c`); padding changes the measured initrd |
| What stands | SEV-SNP Option 2 (image on disk, verifying initrd, PCR12) worked as claimed: `status ok`, the application mounted, PCR12 as computed, recorded twice. TDX prepends (`AppFirst`), which needs no padding |
| Not known | whether the 2026-11 SEV-SNP Option 1 runs on GCP and AWS loaded the application: it depends on the length of the base initrd each run used, and those files were not kept |
| Where | `docs/build/sev-snp.md`, "Correction: SEV-SNP Option 1 as appended in 2026-11"; fixtures with boot logs in `internal/tee/parse/snp/testdata/gcp-n2d-opt1-*` and `gcp-n2d-opt1p-*` |

### Other decisions made after the slides

Whether the slides mention these was not checked; look for them when correcting the deck.

| | before | now | decided |
|---|---|---|---|
| SEV-SNP binding | REPORT_DATA = SHA-512 of the vTPM attestation (*record:* the 2026-11 scripts, no longer available) | SHA3-512 | 2026-11-11 |
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
| 0.0b | Add the JSONL result line to each e2e script, and the two missing tests (T8 debug mode, T9 replay) | `e2e-work/results/aws-nitro.jsonl` has one line per test. **JSONL done 2026-11-12** (`result()` in `e2e/env.sh`; the CVM script writes the same fields); T8 and T9: see the night log |

0.0b comes before any TDX or SEV-SNP script is written, so all three are written against the same
output contract.
