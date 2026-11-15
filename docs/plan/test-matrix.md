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
| **2026-11 run** | observed in the 2026-11 SEV-SNP runs, made with scripts that are no longer available and cannot be re-run. Kept as a record of what was seen, not as a result this repository can reproduce. Where this repository has measured the same thing, the cell shows our result and the 2026-11 observation is in footnote ¹⁰ |
| **Nitro baseline 2026-11-12** | run on two real Nitro instances at commit `9a5668e`, no chain; results in `docs/results/aws-nitro-2026-11-12.md` and its `.jsonl` |
| **script, never run** | a Nitro e2e script exists for the row but has never been run against real instances |
| **needs VM** | nothing has run; it needs instances (and, for TDX and SEV-SNP, the request/build/launch code of Phases 2–4) |
| **missing** | no script and no test |

| id | test | expected | Nitro | TDX | SEV-SNP (GCP) |
|---|---|---|---|---|---|
| T1 | same source built on two machines | identical measurement | **Nitro baseline 2026-11-12**: two x86_64 machines gave identical PCR0/1/2 for the test application (`arch-measurement-compare.sh`). The arm64 side (expected: different) **not run**, no `ARM64_HOST` | **measured 2026-11-11** for the application image ⁹ | **measured 2026-11-11** for the application image ⁹ (the same image on the SEV-SNP and the TDX guest); the cpio archive's bytes are pinned by **code** (`internal/setup/build/cpio` golden tests) ¹⁰ |
| T2 | only the Go version differs | different measurement (Nitro: `build` refuses) | **code**: `TestToolchain_E2E_WrongToolchainRefused` (refused). **Nitro baseline 2026-11-12** (`toolchain-pin-check.sh`): an `att-vendor` built with go1.26.2 is refused by `build` on the instance, the pinned one passes, and the responder's bytes differ between the two toolchains. The differing PCRs themselves were not built, since `build` refuses ² | **needs VM** | **needs VM** for the SEV-SNP measurement itself. What it rests on is measured: the responder's bytes differ under go1.26.1 and go1.26.2 (the same linux/amd64 binary on every platform; Nitro baseline 2026-11-12), and **code** refuses a SEV-SNP build config without `build_env` (T7) ¹⁰ |
| T3 | only the build path differs, with `-trimpath` | identical measurement | **code** for the responder: `TestBuildResponderForArch_ReproducibleAcrossCheckoutPaths`. **Nitro baseline 2026-11-12**: the whole EIF built from two checkout paths (on two machines) gave identical PCR0/1/2; `build` refused a responder without `-trimpath` and one with VCS stamping (`arch-measurement-compare.sh`, `responder-check.sh`) ³ | **needs VM** | **code** for the responder (`TestBuildResponderForArch_ReproducibleAcrossCheckoutPaths`, the same binary as on Nitro); the whole initrd built from two paths: **needs VM** ¹⁰ |
| T4 | normal flow: Auditor rebuilds, registers, User verifies | trusted, exit 0 | **code**: `TestRun_E2E_AuditorVendorUser` (recorded document, anvil). Real instances: **script, never run** (`flow1-auditor-registers/1,2,3`) | **code**: `TestRun_E2E_TDX_AuditorUser` (recorded quote, anvil; first passed 2026-11-11). **Live 2026-11-12** on one guest, twice (stock boot; boot with the dm-verity application mounted): audit registered, verify exit 0 with `✓ matches`, but the Auditor's values came from the guest, not from a rebuild | **code** for verification only: `TestCmd_E2E_SNP_ParseInitrdBoot`; the audit/verify chain on recordings in `TestRun_E2E_T5_ApplicationSwapIsRefused/snp-*`. **Live 2026-11-12** on one guest (`amd-sev-snp-initrd`): audit registered, verify exit 0 with `✓ matches`, but the Auditor's values came from the guest, not from a rebuild |
| T5 | application swapped after registration | not trusted, exit 1 | **code** in part: an unregistered measurement is not trusted (`TestRun_E2E_User_NothingRegistered`). The swap itself: **script, never run** (`flow1-auditor-registers/4,5`) | **code**, both options: `TestRun_E2E_T5_ApplicationSwapIsRefused/tdx-option1`, `/tdx-option2` (recordings of 2026-11-11) ⁸ | **code**, both options: `…/snp-option1` (padded pair), `…/snp-option2` ⁸ |
| T6 | Vendor self-registers a different image, Auditor disagrees | not trusted when both registries are required | **script, never run** (`flow2-vendor-self-registers/1..5`) | **needs VM** | **needs VM** |
| T7 | build environment (OS / arch / Go) does not match | `receive` refuses | **code**: `TestBuildEnv_ReceiveRejectsOtherArch`, `…OtherGoVersion`, `…ConfigWithoutBuildEnv`. **Nitro baseline 2026-11-12** on the x86_64 Vendor (`build-env-check.sh`): a config declaring arm64 is refused naming both architectures, `--ignore-env` only warns, amd64 passes; `send` refuses a record without `build_env` (`arch-record-check.sh`). The arm64 mirror **not run** | **code** in part: an `intel-tdx` build config without `build_env` is refused (`TestBuildConfig_BuildEnvRequiredOnTDX`) | **code** in part: an `amd-sev-snp-initrd` or `amd-sev-snp-verity` build config without `build_env` is refused (`TestBuildConfig_BuildEnvRequiredOnSNP`, added 2026-11-12) |
| T8 | TEE in debug mode | measurements refused | **Nitro, live 2026-11-15** (`debug-mode-check.sh`): the same EIF started with `--debug-mode` gave a genuinely signed document with the right nonce and all-zero PCRs; `ra-parse` refused it with exit 7 ("every PCR is zero"). The normal launch of the same EIF, the control, verified ⁴ | **code**: `TestParse_DebugTDIsRefused` (debug bit set in a copy of the recorded quote) | **code**: `TestParse_DebugGuestIsRefused` (debug bit set in a copy of the recorded report) |
| T9 | replay of an earlier attestation | refused (nonce) | **code**: `TestCmd_E2E_ParseRejectsTamperedNonce` and the nonce unit tests. **Nitro, live 2026-11-15** (`replay-check.sh`): two fresh documents from the running enclave each verified against their own nonce and differed; document 1 presented against request 2's nonce was refused with exit 6 ("document nonce is not the one sent") ⁵ | **code**: `tdx` nonce tests (a different REPORTDATA is refused); **live 2026-11-12**: a fresh quote presented against the next request's nonce, refused with exit 6 (`e2e/cvm/in-guest-verify.sh`) | **code**: `snp` nonce tests; **live 2026-11-12**: a fresh document from the guest, presented against the next request's nonce, refused with exit 6 (`e2e/cvm/in-guest-verify.sh`) ¹⁰ |
| T10 | two attestations from different machines, combined | refused | **not applicable** ⁶ | **not applicable** ⁶ | **not reproduced in this repository.** The **2026-11 run** record says it **fails** (the combination verified), on GCP and AWS ⁷ ¹⁰. `TestVerifySig_MixedRecordingsFailTheBinding` is **not** this row: it mixes two recordings of one VM, which the binding refuses by construction. Procedure to measure it: `docs/plan/app-fixture-recording.md`, "Reproducing T10" (written, not run) |

¹ `docs/build/sev-snp.md`, "Variables", keeps the 2026-11 observations as a record. The 2026-11 numbering of "the seven
SEV-SNP tests" was the scripts' own and is not the same as T1–T10 here.

² Nitro behaves differently here by design. Attestamp pins the toolchain and `build` **refuses**
to run under another patch version, rather than producing a different measurement. The underlying fact
is the same (`toolchain-pin-check.sh` records that go1.26.2 and go1.26.8 give different binaries
even with `-trimpath`), but the product behaviour differs, so record both: refused, and — in a
separate run with the pin bypassed — the measurement that differs.

³ Since `e6ef9b4` every build also uses `-buildvcs=false`, and `build` refuses a responder without it: measured,
identical source committed under two commits gave two different responder binaries without it
(`docs/build/aws-nitro.md`). T1 and T3 for Nitro must be run after that commit and after `a379a2c`, which changed the
entrypoint inside the EIF, and `60de6b0`, which changed the responder: **the baseline of 2026-11-12 was taken at
`9a5668e`, after all three.**

⁴ `tee/parse/aws/measure.go` treats an all-zero PCR0 as debug (`Info.Debug`) and `Parse` fails when every PCR is
zero. A debug document is signed, so unlike TDX and SEV-SNP it cannot be made from the recorded one by flipping a bit;
it has to be recorded from a debug enclave. `e2e/debug-mode-check.sh` did that on 2026-11-15 (code at `1663a5d`; the
same test-application PCRs as the 2026-11-12 baseline). The debug document's signature and nonce verified and it was
marked `debug true`; then every PCR was zero and it was refused. The document is **not committed** as a fixture: its
signed `module_id` carries the EC2 instance ID, which may not be committed (`CLAUDE.md`, rule 4). So T8 on Nitro stays
a live result, not a code test.

⁵ The nonce check is inside `verify` and has unit and command tests. `e2e/replay-check.sh` replayed a fresh document
against the next request's nonce on a live Nitro enclave on 2026-11-15 (the enclave `debug-mode-check.sh` relaunched;
no chain), and it was refused. The same check passed live on TDX and SEV-SNP (`e2e/cvm/in-guest-verify.sh`), so T9 is
now live on all three.

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

⁷ The claim: a vTPM attestation from an approved VM plus an SNP report from a different VM passes verification. It is
the reason SEV-SNP is weaker than the other two, and it has to appear in the table rather than in a footnote. By the
structure of the evidence it should hold with the SHA3-512 binding too (the relaying VM writes the hash of the
other's attestation into its own report), but the only observation is the 2026-11 record, made with the SHA-512
binding and scripts that are gone. Until "Reproducing T10" is run, this repository cannot show it.

¹⁰ The 2026-11 observations these SEV-SNP cells used to show, kept as a record (scripts no longer available, results
not reproducible; `docs/build/sev-snp.md`, "Variables"):

| row | 2026-11 observation | what replaced it in the cell |
|---|---|---|
| T1 | the application, its cpio archive and the dm-verity image hashed the same on two GCP machines | our measurement of the image on two guests (2026-11-11) and the cpio golden tests |
| T2 | Go 1.22.2 and 1.23.4 gave different application binaries | our responder bytes under two Go patch versions (2026-11-12); the SEV-SNP measurement itself is "needs VM" |
| T3 | without `-trimpath`, two folders gave two binaries | our code test of the responder across checkout paths; the whole initrd is "needs VM" |
| T9 | replayed evidence was refused | our live replay refusal on the guest (2026-11-12) |
| T10 | a vTPM attestation from one VM with an SNP report from another verified | nothing yet: the cell says so, and "Reproducing T10" is the plan |

## Making the results machine-readable

The scripts today print `✅` / `❌` and set an exit code. That is enough for a person watching and
not enough to assemble a three-column table automatically.

Each verdict appends one line to `e2e-work/results/<platform>.jsonl` (done 2026-11-12: `result()` in `e2e/env.sh`,
called by every Nitro e2e script, and the same fields from `e2e/cvm/in-guest-verify.sh` for the confidential VMs):

```json
{"platform":"aws-nitro","chain":"sepolia","test":"T5","check":"user-verify-after-swap","script":"e2e/flow1-auditor-registers/5-User-tamper-verify.sh","result":"rejected","expected":"rejected","reason":"verify exit 1 after the Vendor swapped the enclave","detail":"registry 0x…","run":"20261112T010000Z","at":"2026-11-12T01:02:03Z"}
```

| field | meaning |
|---|---|
| `platform` | the format string: `aws-nitro`, `intel-tdx`, `amd-sev-snp-initrd`, `amd-sev-snp-verity` |
| `chain` | `NET` of the run: `sepolia`, `mainnet` or `local` (anvil; always `local` from `in-guest-verify.sh`). Meaningful on T4–T6, the rows that use the chain. Added 2026-11-15; lines written before have no `chain` field, and none of them was on a public chain |
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
  requires `CONFIRM_MAINNET=1`. `NET=local` needs no ETH and no key files: a local anvil and its development accounts
  (`e2e/README.md`, "Running without a public testnet")
- `e2e/env.local.sh`, copied from `env.local.example.sh`, holding the instance addresses. It is
  gitignored; never commit it

SEV-SNP and TDX need GCP instances only (SEV-SNP on AWS was decided against on 2026-11-11). They do not all have to
exist at the same time: each platform's rows can be filled in its own session, as long as every run writes to the
same JSONL contract.
