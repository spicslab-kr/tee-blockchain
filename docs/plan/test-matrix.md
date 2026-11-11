# The common test matrix

The claim this project wants to support is: **the same flow, the same checks and the same verdicts
on three TEEs.** That claim is only worth anything if the same tests are actually run on all three
and the results are recorded side by side.

Nitro is the baseline. It must be run on real instances and its results recorded in the same shape
as the other two, not quoted from the README.

## State on 2026-11-12

Each cell says how far the row is proven, and by what:

| mark | meaning |
|---|---|
| **code** | a test in this repository passes on recorded evidence, with no instance running (`go test ./...`, anvil 1.8.0). It proves the software's verdict, not the platform's behaviour on a fresh run |
| **2026-11 run** | observed in the 2026-11 SEV-SNP runs, made with scripts outside this repository and with the SHA-512 binding of that time. Not re-run since |
| **script, never run** | a Nitro e2e script exists for the row but has never been run against real instances. No Nitro baseline exists |
| **needs VM** | nothing has run; it needs instances (and, for TDX and SEV-SNP, the request/build/launch code of Phases 2–4) |
| **missing** | no script and no test |

| id | test | expected | Nitro | TDX | SEV-SNP (GCP) |
|---|---|---|---|---|---|
| T1 | same source built on two machines | identical measurement | **script, never run** (`arch-measurement-compare.sh`) | **measured 2026-11-11** for the application image ⁹ | **measured 2026-11-11** for the application image ⁹; also **2026-11 run**: the application, its cpio archive and the dm-verity image hashed the same on two GCP machines ¹ |
| T2 | only the Go version differs | different measurement (Nitro: `build` refuses) | **code**: `TestToolchain_E2E_WrongToolchainRefused` (refused). The differing measurement itself: **script, never run** (`toolchain-pin-check.sh`) ² | **needs VM** | **2026-11 run**: Go 1.22.2 and 1.23.4 gave different binaries ¹ |
| T3 | only the build path differs, with `-trimpath` | identical measurement | **code** for the responder: `TestBuildResponderForArch_ReproducibleAcrossCheckoutPaths`. The whole EIF: **script, never run** ³ | **needs VM** | **2026-11 run** (without `-trimpath`, two folders gave two binaries) ¹ |
| T4 | normal flow: Auditor rebuilds, registers, User verifies | trusted, exit 0 | **code**: `TestRun_E2E_AuditorVendorUser` (recorded document, anvil). Real instances: **script, never run** (`flow1-auditor-registers/1,2,3`) | **code**: `TestRun_E2E_TDX_AuditorUser` (recorded quote, anvil; first passed 2026-11-11). **Live 2026-11-12** on one guest, twice (stock boot; boot with the dm-verity application mounted): audit registered, verify exit 0 with `✓ matches`, but the Auditor's values came from the guest, not from a rebuild | **code** for verification only: `TestCmd_E2E_SNP_ParseInitrdBoot`; the audit/verify chain on recordings in `TestRun_E2E_T5_ApplicationSwapIsRefused/snp-*`. **Live 2026-11-12** on one guest (`amd-sev-snp-initrd`): audit registered, verify exit 0 with `✓ matches`, but the Auditor's values came from the guest, not from a rebuild |
| T5 | application swapped after registration | not trusted, exit 1 | **code** in part: an unregistered measurement is not trusted (`TestRun_E2E_User_NothingRegistered`). The swap itself: **script, never run** (`flow1-auditor-registers/4,5`) | **code**, both options: `TestRun_E2E_T5_ApplicationSwapIsRefused/tdx-option1`, `/tdx-option2` (recordings of 2026-11-11) ⁸ | **code**, both options: `…/snp-option1` (padded pair), `…/snp-option2` ⁸ |
| T6 | Vendor self-registers a different image, Auditor disagrees | not trusted when both registries are required | **script, never run** (`flow2-vendor-self-registers/1..5`) | **needs VM** | **needs VM** |
| T7 | build environment (OS / arch / Go) does not match | `receive` refuses | **code**: `TestBuildEnv_ReceiveRejectsOtherArch`, `…OtherGoVersion`, `…ConfigWithoutBuildEnv`. On instances: **script, never run** (`build-env-check.sh`) | **code** in part: an `intel-tdx` build config without `build_env` is refused (`TestBuildConfig_BuildEnvRequiredOnTDX`) | **code** in part: an `amd-sev-snp-initrd` or `amd-sev-snp-verity` build config without `build_env` is refused (`TestBuildConfig_BuildEnvRequiredOnSNP`, added 2026-11-12) |
| T8 | TEE in debug mode | measurements refused | **missing** ⁴ | **code**: `TestParse_DebugTDIsRefused` (debug bit set in a copy of the recorded quote) | **code**: `TestParse_DebugGuestIsRefused` (debug bit set in a copy of the recorded report) |
| T9 | replay of an earlier attestation | refused (nonce) | **code**: `TestCmd_E2E_ParseRejectsTamperedNonce` and the nonce unit tests. A replay against a live verifier: **missing** ⁵ | **code**: `tdx` nonce tests (a different REPORTDATA is refused); **live 2026-11-12**: a fresh quote presented against the next request's nonce, refused with exit 6 (`e2e/cvm/in-guest-verify.sh`) | **code**: `snp` nonce tests; **2026-11 run** replayed evidence and saw it refused; **live 2026-11-12**: a fresh document from the guest, presented against the next request's nonce, refused with exit 6 (`e2e/cvm/in-guest-verify.sh`) |
| T10 | two attestations from different machines, combined | refused | **not applicable** ⁶ | **not applicable** ⁶ | **fails today**: reproduced in the **2026-11 run** on GCP (and on AWS, before AWS was dropped for SEV-SNP) ⁷ |

¹ From `docs/build/sev-snp.md`, "Variables". The 2026-11 numbering of "the seven SEV-SNP tests" is the scripts' own
and is not the same as T1–T10 here; only rows whose meaning matches are filled.

² Nitro behaves differently here by design. Attestamp pins the toolchain and `build` **refuses**
to run under another patch version, rather than producing a different measurement. The underlying fact
is the same (`toolchain-pin-check.sh` records that go1.26.2 and go1.26.8 give different binaries
even with `-trimpath`), but the product behaviour differs, so record both: refused, and — in a
separate run with the pin bypassed — the measurement that differs.

³ Since `e6ef9b4` every build also uses `-buildvcs=false`, and `build` refuses a responder without it: measured,
identical source committed under two commits gave two different responder binaries without it
(`docs/build/aws-nitro.md`). T1 and T3 for Nitro must be run after that commit and after `a379a2c`, which changed the
entrypoint inside the EIF: **the Nitro baseline is taken after both**.

⁴ `tee/parse/aws/measure.go` treats an all-zero PCR0 as debug (`Info.Debug`) and `Parse` fails when every PCR is
zero, but no test or script launches a debug-mode enclave to prove it. A debug document is signed, so unlike TDX and
SEV-SNP it cannot be made from the recorded one by flipping a bit; it has to be recorded from a debug enclave.

⁵ The nonce check is inside `verify` and has unit and command tests, but no e2e script replays a saved document
against a live verifier.

⁶ Nitro and TDX return one document from one TEE; there is nothing to combine.

⁸ Moved from "needs VM" to "code" on 2026-11-11 with one VM session per platform: the evidence was recorded twice on
the same guest, with the application changed by one byte, for each option (`docs/plan/app-fixture-recording.md`).
The test registers v1 on a local anvil chain, checks the User trusts it, then serves v2 and requires exit 1 with
"nobody has registered". From now on it is proved by `go test`, with no VM. What the test cannot show: whether the
application in the recording actually ran. That is shown by the boot logs next to the fixtures (`*-boot.txt`); the
SEV-SNP unpadded pair, whose application was discarded by the kernel, measures and verifies just the same and is
therefore not used here.

⁹ The dm-verity application image built on `attestamp-snp-vendor` and on `attestamp-tdx-vendor` was byte-identical:
two machines, two platforms, one image (`app.img` sha256 `28bdfcff…ec1c`, root hash `e29ec085…af0c9` for v1;
`b8e2656f…fb27` and `a2f4df92…51e1` for v2). Settings that made it reproducible: squashfs with `-mkfs-time 0
-all-time 0`, veritysetup with a fixed `--salt` and `--uuid`, and a fixed umask, **`umask 022`**, with every folder
and file set to mode 755 (`internal/setup/build/cpio/testdata/make-verity-image.sh`; squashfs-tools 4.6.1, cryptsetup
2.7.0 on both). This covers the image, not a whole measurement: the base initrd and the boot chain were the
distribution's on both guests.

⁷ A vTPM attestation from an approved VM plus an SNP report from a different VM passes verification. This row is the
honest reason SEV-SNP is weaker than the other two, and it has to appear in the table rather than in a footnote. The
switch to the SHA3-512 binding (2026-11-11) does not change it.

## Making the results machine-readable

The scripts today print `✅` / `❌` and set an exit code. That is enough for a person watching and
not enough to assemble a three-column table automatically.

Each verdict appends one line to `e2e-work/results/<platform>.jsonl` (done 2026-11-12: `result()` in `e2e/env.sh`,
called by every Nitro e2e script, and the same fields from `e2e/cvm/in-guest-verify.sh` for the confidential VMs):

```json
{"platform":"aws-nitro","test":"T5","check":"user-verify-after-swap","script":"e2e/flow1-auditor-registers/5-User-tamper-verify.sh","result":"rejected","expected":"rejected","reason":"verify exit 1 after the Vendor swapped the enclave","detail":"registry 0x…","run":"20261112T010000Z","at":"2026-11-12T01:02:03Z"}
```

| field | meaning |
|---|---|
| `platform` | the format string: `aws-nitro`, `intel-tdx`, `amd-sev-snp-initrd`, `amd-sev-snp-verity` |
| `test` | the row above (`T1`…`T10`), or `-` for a check that is not a row (for example the local Go version) |
| `check` | which verdict within the script; one script can write several lines for one row |
| `script` | the script that wrote it |
| `result`, `expected` | one of `accepted`, `rejected`, `identical`, `different`, `refused`, or `error` (skipped or broken: the reason says which) |
| `reason` | what the script printed |
| `detail` | optional: measured values, registry address |
| `run`, `at` | the script run's start time and the line's time, UTC |

A line passes when `result` equals `expected`. A run that quietly stops passing shows up as a missing or `error` line
rather than as nothing at all.

Rows each script writes: `arch-measurement-compare.sh` T1, T3; `toolchain-pin-check.sh` T2; `responder-check.sh` T3;
`build-env-check.sh` and `arch-record-check.sh` T7; flow 1 T4, T5; flow 2 T6; `in-guest-verify.sh` T4, T9.

## What running the baseline needs

From `e2e/README.md`:

- two Nitro-enabled EC2 instances (Vendor, Auditor) in `eu-north-1`, plus an optional arm64 (Graviton) for T1/T3.
  The vCPU quota there was requested; its current value has to be read with an authenticated `aws` (the session
  expired on 2026-11-11)
- `git`, `ssh`, and exactly the pinned Go version on the local machine
- an SSH key in the repository root, and **two funded key files**: `auditor.key` (approver in flow 1)
  and `deployer.key` (pays gas, approver in flow 2), with their addresses set as `AUDITOR_ADDR` and `VENDOR_ADDR` in
  `e2e/env.local.sh`
- **test ETH on Sepolia** for both keys, which is the default network. `NET=mainnet` spends real ETH and additionally
  requires `CONFIRM_MAINNET=1`
- `e2e/env.local.sh`, copied from `env.local.example.sh`, holding the instance addresses. It is
  gitignored; never commit it

SEV-SNP and TDX need GCP instances only (SEV-SNP on AWS was decided against on 2026-11-11). They do not all have to
exist at the same time: each platform's rows can be filled in its own session, as long as every run writes to the
same JSONL contract.
