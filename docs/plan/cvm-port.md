# Porting Attestamp from Nitro to confidential VMs

Status (2026-11-11): Phase 0.3 and 0.4 are done (fixtures recorded). Phase 1 (TDX parse) and phase 4a (SEV-SNP
parse, D3 decided in `docs/build/sev-snp.md`) are implemented and tested on those fixtures; the anvil chain test for
TDX is written but has not run. D4 is decided in `docs/build/tdx.md`. Everything needing a running guest is not
started. See `docs/plan/night-log.md`.

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
| 0.1 | Clone the repository from GitHub (the current working copy has no `.git`) | `git status` works |
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

## Phase 2 — TDX request (needs a GCP TDX instance)

| # | Task | Pass criterion |
|---|---|---|
| 2.1 | `internal/tee/request/{Vendor,auditor}/tdx/` — get a quote for a given nonce | the returned quote parses with the Phase 1 parser |
| 2.2 | `--extractor tdx` in `cmd/ra-responder` | the responder answers a request from another machine |
| 2.3 | End to end: `att-user verify` against a running TDX instance | exit 0 and a `✓ matches` line |

## Phase 3 — TDX build and launch (needs two GCP TDX instances)

| # | Task | Pass criterion |
|---|---|---|
| 3.1 | Deterministic cpio of the application — fixed mtime, sorted order | two builds on two machines hash the same |
| 3.2 | Prepend the application cpio to the base initrd (appending breaks decompression) | the instance boots |
| 3.3 | Turn off `GRUB_FORCE_PARTUUID` so the initrd is actually read and measured | RTMR[2] changes when the initrd changes |
| 3.4 | Option 2: squashfs + dm-verity (ext4 is not reproducible even with fixed UUID and hash_seed) | two builds give the same root hash |
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

Existing reference material: the SEV-SNP test scripts from the October–November runs
(`option1-initrd/`, `option2-dm-verity/`, `vendor/`, `auditor/`, `user/go-verifier/`,
`relay-test/`). They are not committed here. Known blockers before any of it is reusable:
hard-coded home paths, disk UUIDs, kernel versions and instance IPs; two separate Go modules
outside `attestamp`; expected-value计算 in Python; Korean comments.

## Open decisions

These change the design and are not settled.

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
(`amd-sev-snp-initrd`, `amd-sev-snp-verity`) avoid this.

**D4. What TDX registers.** The October slides are inconsistent: one says MRTD, RTMR[0], RTMR[1]
and RTMR[2] are all in scope; another says the registered value is RTMR[2] alone. Pick one.
MROWNER differs per machine and must be excluded either way, like Nitro's PCR3/4/8.

**D5. Freshness without a timestamp.** Nitro judges the certificate chain at the document's own
timestamp. If an SNP report carries no equivalent field, decide what replaces it.

**D6. SEV-SNP cannot prove both documents came from the same VM.** Reproduced on GCP, and on AWS before AWS was
dropped for SEV-SNP (that AWS run stays as a record that the result is not GCP-specific): a
vTPM attestation from an approved VM combined with an SNP report from a different VM passes
verification. Nitro does not have this problem and TDX does not either. Until D1(c) or an in-VM
vTPM is used, SEV-SNP support is honest only if the limitation is documented.

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
