# The common test matrix

The claim this project wants to support is: **the same flow, the same checks and the same verdicts
on three TEEs.** That claim is only worth anything if the same tests are actually run on all three
and the results are recorded side by side.

Nitro is the baseline. It must be run on real instances and its results recorded in the same shape
as the other two, not quoted from the README.

This page says what is proven, and by what. Its pair, [build-info-gaps.md](build-info-gaps.md), says what the
repository still lacks and which design questions are open.

## State

Updated through 2026-11-20.

Each cell says how far the row is proven, and by what:

| mark | meaning |
|---|---|
| **code** | a test in this repository passes on recorded evidence, with no TEE running (`go test ./...`, anvil 1.8.0). It proves the software's verdict, not the platform's behaviour on a fresh run |
| **old-script run** | observed in the SEV-SNP runs of the old out-of-repository scripts, which are no longer available and cannot be re-run. Kept as a record of what was seen, not as a result this repository can reproduce. Where this repository has measured the same thing, the cell shows our result and the old scripts' observation is in footnote ¹⁰ |
| **Nitro baseline 2026-11-12** | run on two real Nitro instances at commit `ab55208`, no chain; results in `docs/results/aws-nitro-2026-11-12.md` and its `.jsonl`. Its PCR0/PCR2 values are from before `a402827` and `1967749d`, which changed them; **taken again on 2026-11-16 at `23b83f9`**, with arm64: `docs/results/aws-nitro-2026-11-16.md` ¹⁶; and **again on 2026-11-17 at `83cb2dc`**, after the empty build ID: `docs/results/aws-nitro-2026-11-17.md` ¹⁸; and **a second time on 2026-11-17 at `6146c10`**, after the Nitro request code moved into a shared package: `docs/results/aws-nitro-2026-11-17-shared-aws.md` ¹⁹. Since `89c0063` (2026-11-20) `build` layers another file into every image, so these PCR0/PCR2 values no longer describe what `build` makes; not measured again yet (`build-info-gaps.md`, row 38) |
| **Nitro, live …, local chain** | run on the two real Nitro instances with the registry on a local anvil, not a public network (footnote ¹¹) |
| **script, never run** | a Nitro e2e script exists for the row but has never been run against real instances |
| **needs VM** | nothing has run; it needs instances or guests (since session 4 of 2026-11-16 no cell is left in this state; see "Cells still open" below the table) |
| **not applicable** | the row cannot be run on this platform as we use it; the cell says why |
| **missing** | no script and no test |

| id | test | expected | Nitro | TDX | SEV-SNP (GCP) |
|---|---|---|---|---|---|
| T1 | same source built on two machines | identical measurement | **Live 2026-11-17 at `6146c10`** ¹⁹ (responder `b9ae17f3…`, shared Nitro request package): two x86_64 machines, the second from another checkout path, gave **identical** PCR0 `05662eae…`, PCR1 `4b4d5b36…`, PCR2 `a3049e0d…`; **arm64 differs** in all three (PCR0 `a653991e…`, PCR1 `3b4a7e1b…`, PCR2 `9660bc5e…`), as expected. **Earlier the same day at `83cb2dc`** ¹⁸ (responder `d6e3422d…`, empty build ID): two x86_64 machines, the second from another checkout path, gave **identical** PCR0 `bfa0a428…`, PCR1 `4b4d5b36…`, PCR2 `aa36ce6f…`; **arm64 differs** in all three (PCR0 `4f94d9c9…`, PCR1 `3b4a7e1b…`, PCR2 `79d0abea…`), as expected. **Before that, live 2026-11-16 at `23b83f9`** ¹⁶ (responder `c61c988e…`): two x86_64 machines, the second from another checkout path, gave **identical** PCR0 `0c905480…`, PCR1 `4b4d5b36…`, PCR2 `6347a146…`; **arm64 differs** in all three (PCR0 `f1d4d679…`, PCR1 `3b4a7e1b…`, PCR2 `6e865138…`), as expected. Earlier, each at its own commit: the 2026-11-12 baseline at `ab55208` (two machines identical, PCR0 `d5bbd4af…`; arm64 not run); a build three days later differed through the `/etc/shadow` build day, fixed by `a402827` ¹¹; the clean trial at `6a92a1f` (one machine, PCR0 `2aacdf41…`) ¹³. The values differ across those commits because the code does (`a402827`, `1967749d`), not because builds disagree | **measured 2026-11-11** for the application image ⁹. **Live 2026-11-15, whole measurement**: two guests made five days apart from one image built the same application into the same initrd and measured **the same MRTD and RTMR0–2** (the root UUID and the base initrd come with the image); the Option 2 image also came out the same in a WSL and on a guest (`docs/results/trial-2026-11-15.md`) ¹². **Machine type, live 2026-11-16** ¹⁷: the same guest and boot at `c3-standard-8` moved **RTMR0** (`8847ec5d…` → `eba48352…`), nothing else; a new `c3-standard-4` guest gave session 2's four values again. **Zone, live 2026-11-16** ¹⁷: a guest in `us-central1-b` gave the same four values as `us-central1-a`; the zone did not move them | **measured 2026-11-11** for the application image ⁹ (the same image on the SEV-SNP and the TDX guest); the cpio archive's bytes are pinned by **code** (`internal/setup/build/cpio` golden tests) ¹⁰ **Live 2026-11-15, whole measurement**: two guests of one image gave the same MEASUREMENT, PCR4, KERNEL, CMDLINE and INITRD for the same application; the Option 2 image and root hash came out the same in a WSL and on the guests, and equal to the 2026-11-11 images for the recordings' application (`docs/results/trial-2026-11-15.md`) ¹². **Machine type and zone, live 2026-11-16** ¹⁷: the same guest and boot at `n2d-standard-4` (4 vCPUs) moved **MEASUREMENT** (`f6790085…` → `2279d213…`), nothing else; `n2d-highmem-2` (2 vCPUs, 16 GB) and a new guest in `us-central1-b` gave `f6790085…`. The vCPU count moves it, the memory and the zone did not |
| T2 | only the Go version differs | different measurement (Nitro: `build` refuses) | **code**: `TestToolchain_E2E_WrongToolchainRefused` (refused). **Nitro baseline 2026-11-12** (`toolchain-pin-check.sh`): an `att-vendor` built with go1.26.2 is refused by `build` on the instance, the pinned one passes, and the responder's bytes differ between the two toolchains. The differing PCRs themselves were not built, since `build` refuses ². **Again 2026-11-16 at `23b83f9`** ¹⁶: refused with Nitro's own reason; responders `c61c988e…` (go1.26.1) vs `24724239…` (go1.26.2). **Again 2026-11-17 at `ab714cd`** ¹⁸, empty build ID: refused; responders `d6e3422d…` (go1.26.1) vs `8343745f…` (go1.26.2). **Again later on 2026-11-17 at `6146c10`** ¹⁹: refused; responders `b9ae17f3…` (go1.26.1) vs `fde4857c…` (go1.26.2) | **Live 2026-11-16** ¹⁴: a Go application (`cmd/ra-responder`) built with go1.26.1 and go1.26.2 and booted on one guest: initrds `162372c6…` vs `e94699bd…`, **RTMR2 `4ea92ca9…` vs `1c181218…`**, MRTD and RTMR0–1 equal. `att-vendor` built with go1.26.2 refuses `build` on the guest | **Live 2026-11-16** ¹⁵: the same Go application built with go1.26.1 and go1.26.2 and booted on one guest: **INITRD `48e09d59…` vs `bca95760…`**, MEASUREMENT, PCR4, KERNEL, CMDLINE equal; `att-vendor` built with go1.26.2 refuses `build` with SEV-SNP's own reason. Before that, the measurement itself was "needs VM". What it rests on is measured: the responder's bytes differ under go1.26.1 and go1.26.2 (the same linux/amd64 binary on every platform; Nitro baseline 2026-11-12), and **code** refuses a SEV-SNP build config without `build_env` (T7) ¹⁰ |
| T3 | only the build path differs, with `-trimpath` | identical measurement | **code** for the responder: `TestBuildResponderForArch_ReproducibleAcrossCheckoutPaths`. **Nitro baseline 2026-11-12**: the whole EIF built from two checkout paths (on two machines) gave identical PCR0/1/2; `build` refused a responder without `-trimpath` and one with VCS stamping (`arch-measurement-compare.sh`, `responder-check.sh`) ³. **Again 2026-11-16 at `23b83f9`** ¹⁶: two checkout paths identical; the three wrong responders refused, the correct one accepted. **Again 2026-11-17 at `83cb2dc`** ¹⁸: two checkout paths identical; a fourth wrong responder, one that carries a Go build ID, **refused** on the instance; the arm64 one refused when its step was run by hand (the scripted SSH was reset). **Again later on 2026-11-17 at `6146c10`** ¹⁹: two checkout paths identical; the four wrong responders refused (the arm64 one again by hand, after the same SSH resets), the correct one accepted | **Live 2026-11-16** ¹⁴: the same Go application from two checkout paths (all three settings) gave identical binaries; on the guest, built from two `app` folders of different depth, the same initrd `162372c6…`, booted to RTMR2 `4ea92ca9…` | **code** for the responder (`TestBuildResponderForArch_ReproducibleAcrossCheckoutPaths`, the same binary as on Nitro) ¹⁰. **Live 2026-11-16** ¹⁷: the same Go application from two checkout paths, built on the guest from two `app` folders of different depth: the same initrd `48e09d59…`, booted to INITRD `48e09d59…`; Option 2 the same image and root hash `4a517772…` (built, not booted) |
| T4 | normal flow: Auditor rebuilds, registers, User verifies | trusted, exit 0 | **code**: `TestRun_E2E_AuditorVendorUser` (recorded document, anvil). **Nitro, live 2026-11-15, local chain** ¹¹ (`flow1-auditor-registers/1,2,3`): the Auditor's rebuild matched its enclave and was registered, the Vendor's enclave matched the handed-over values and the registration, `verify` exit 0 with `✓ matches` | **code**: `TestRun_E2E_TDX_AuditorUser` (recorded quote, anvil; first passed 2026-11-11). **Live 2026-11-12** on one guest, twice (stock boot; boot with the dm-verity application mounted): audit registered, verify exit 0 with `✓ matches`, **Live 2026-11-15 from another machine**: `build` and `launch` on a new guest, then `audit` and `verify` from a WSL on another computer, exit 0 with `✓ matches` (local anvil; `docs/results/trial-2026-11-15.md`) ¹². In both live runs the Auditor's values came from the guest, not from a rebuild on its own guest. **Live 2026-11-16 from the documents alone**, both options, exit 0 ¹³. **The independent Auditor, live 2026-11-16, both options** ¹⁴: two guests from one exact image; the Auditor rebuilt on its own guest (the same initrd), booted it, `launch --check` passed, the two guests' values were equal, `audit --vendor-tee` registered its own value (`same: the Vendor's guest runs exactly what I built`), and the User's `verify` of the **Vendor's** guest against the Auditor's registry gave exit 0, `trusted`. **The scripted flow (`e2e/cvm`), live 2026-11-18, both options, local chain** ²⁰: the same steps by script; `audit --vendor-tee` `same`, exit 0; the printed `verify` line exit 0 | **code** for verification only: `TestCmd_E2E_SNP_ParseInitrdBoot`; the audit/verify chain on recordings in `TestRun_E2E_T5_ApplicationSwapIsRefused/snp-*`. **Live 2026-11-12** on one guest (`amd-sev-snp-initrd`): audit registered, verify exit 0 with `✓ matches`, but the Auditor's values came from the guest, not from a rebuild **Live 2026-11-15 from another machine**: `build` and `launch` on a new guest (both options), then `audit` and `verify` from a WSL on another computer, exit 0 with `✓ matches` (local anvil) ¹². **Live 2026-11-16 from the documents alone**, both options, exit 0 ¹³. **The independent Auditor, live 2026-11-16, both options** ¹⁵: two guests from one exact image; the Auditor's own build equalled the Vendor's, both guests measured the same values, `audit --vendor-tee` registered the Auditor's own value, and the User's `verify` of the **Vendor's** guest gave exit 0, `trusted`. **The scripted flow (`e2e/cvm`), live 2026-11-19, `amd-sev-snp-verity` and `amd-sev-snp-initrd`, local chain** ²⁰: the same; `audit --vendor-tee` `same`, exit 0; the printed `verify` line exit 0 |
| T5 | application swapped after registration | not trusted, exit 1 | **code** in part: an unregistered measurement is not trusted (`TestRun_E2E_User_NothingRegistered`). **Nitro, live 2026-11-15, local chain** ¹¹ (`flow1-auditor-registers/4,5`): after the Vendor swapped the enclave, the genuine document's value had no registration, `verify` exit 1 ("✗ nobody") | **code**, both options: `TestRun_E2E_T5_ApplicationSwapIsRefused/tdx-option1`, `/tdx-option2` (recordings of 2026-11-11) ⁸. **Live 2026-11-16**, both options: one byte changed, relaunched, `verify` exit 1 ¹³; and against the independent Auditor's registry, one byte changed on the Vendor's side only: `audit --vendor-tee` `runs something else`, `verify` exit 1, both options ¹⁴. **The scripted flow, live 2026-11-18** ²⁰: one byte changed on the Vendor's guest; `audit --vendor-tee` exit 12, the printed `verify` line exit 1, both options | **code**, both options: `…/snp-option1` (padded pair), `…/snp-option2` ⁸. **Live 2026-11-16**, both options: one byte changed, relaunched, `verify` exit 1 ¹³; and against the independent Auditor's registry, one byte changed on the Vendor's side only: `audit --vendor-tee` `runs something else`, `verify` exit 1, both options ¹⁵. **The scripted flow, live 2026-11-19** ²⁰: one byte changed on the Vendor's guest; `audit --vendor-tee` exit 12, the printed `verify` line exit 1, both formats |
| T6 | Vendor self-registers a different image, Auditor disagrees | not trusted when both registries are required | **Nitro, live 2026-11-15, local chain** ¹¹ (`flow2-vendor-self-registers/1..5`): the Vendor's own registry vouched for its honest build and then for the swapped image (exit 0 each), `audit --vendor-tee` reported "runs something else", and `verify` with both registries required gave exit 1 ("only 1 of 2 registries vouch") | **Live 2026-11-16** ¹⁴: the Vendor self-registered its guest running a one-byte-changed application in its own registry (exit 0), `verify` against that registry alone exit 0, `audit --vendor-tee` `runs something else`, `verify` with both registries required **exit 1** ("only 1 of 2 registries vouch") | **Live 2026-11-16** ¹⁵: the same as TDX: self-registered changed guest exit 0, its registry alone exit 0, `audit --vendor-tee` `runs something else`, both registries required **exit 1** |
| T7 | build environment (OS / arch / Go) does not match | `receive` refuses | **code**: `TestBuildEnv_ReceiveRejectsOtherArch`, `…OtherGoVersion`, `…ConfigWithoutBuildEnv`. **Nitro baseline 2026-11-12** on the x86_64 Vendor (`build-env-check.sh`): a config declaring arm64 is refused naming both architectures, `--ignore-env` only warns, amd64 passes; `send` refuses a record without `build_env` (`arch-record-check.sh`). **Again later on 2026-11-17 at `6146c10`** ¹⁹, x86_64 and arm64: all verdicts as before, `nitro-tools-check.sh` included. **Again 2026-11-17** ¹⁸, x86_64 and arm64, with the configs carrying the host's own `ami_release`: all verdicts as before; and `receive` refuses another Amazon Linux release (`nitro-tools-check.sh`, gap 27). **The arm64 mirror, live 2026-11-16 at `23b83f9`** ¹⁶: on a `c7g.xlarge`, a config declaring amd64 is refused naming both architectures, `--ignore-env` only warns, arm64 passes; `uname -m` `aarch64` maps to `arm64`. The x86_64 side passed again | **code**: an `intel-tdx` build config without `build_env` is refused (`TestBuildConfig_BuildEnvRequiredOnTDX`). **Live 2026-11-16 on a guest** ¹⁷ (`e2e/cvm/build-env-check.sh`): `att-vendor send` recorded the guest's own `build_env` (`linux/amd64 go1.26.1 trimpath=true`); a config declaring arm64 was refused naming both architectures, `--ignore-env` only warned, the recorded one passed | **code**: an `amd-sev-snp-initrd` or `amd-sev-snp-verity` build config without `build_env` is refused (`TestBuildConfig_BuildEnvRequiredOnSNP`, added 2026-11-12). **Live 2026-11-16 on a guest** ¹⁷: the same four verdicts as TDX, with an `amd-sev-snp-initrd` config |
| T8 | TEE in debug mode | measurements refused | **Nitro, live 2026-11-15** (`debug-mode-check.sh`): the same EIF started with `--debug-mode` gave a genuinely signed document with the right nonce and all-zero PCRs; `ra-parse` refused it with exit 7 ("every PCR is zero"). The normal launch of the same EIF, the control, verified ⁴ | **code**: `TestParse_DebugTDIsRefused` (debug bit set in a copy of the recorded quote). Live: **not applicable** on GCP ¹⁷: GCP offers no debug confidential VM (no field in the Compute API, no `gcloud` flag; checked 2026-11-16). Every live quote so far passed the check | **code**: `TestParse_DebugGuestIsRefused` (debug bit set in a copy of the recorded report). Live: **not applicable** on GCP ¹⁷, for the same reason. Every live report so far passed the check |
| T9 | replay of an earlier attestation | refused (nonce) | **code**: `TestCmd_E2E_ParseRejectsTamperedNonce` and the nonce unit tests. **Nitro, live 2026-11-15** (`replay-check.sh`): two fresh documents from the running enclave each verified against their own nonce and differed; document 1 presented against request 2's nonce was refused with exit 6 ("document nonce is not the one sent") ⁵ | **code**: `tdx` nonce tests (a different REPORTDATA is refused); **live 2026-11-12**: a fresh quote presented against the next request's nonce, refused with exit 6 (`e2e/cvm/in-guest-verify.sh`); **live 2026-11-16 from another machine** (`replay-check.sh`), exit 6 ¹³ | **code**: `snp` nonce tests; **live 2026-11-16 from another machine** (`replay-check.sh`, on our boot), exit 6 ¹³; **live 2026-11-12**: a fresh document from the guest, presented against the next request's nonce, refused with exit 6 (`e2e/cvm/in-guest-verify.sh`) ¹⁰ |
| T10 | two attestations from different machines, combined | refused | **not applicable** ⁶ | **not applicable** ⁶ | **Fails: reproduced live 2026-11-16** ¹⁵. B's AMD-signed report requested over SHA3-512 of A's vTPM attestation (`record-snp -relay-att`) verifies, **exit 0**, with A's values, and `lookup` finds the Auditor's registration for A, while B runs an unregistered application. Controls: A's and B's own pairs exit 0; B's own report next to A's attestation is refused at the binding, exit 5. So the binding works and the relay passes it (`docs/build/ra-comparison.md`, "The SEV-SNP relay, measured"). A **live result**, like Nitro's T8: the recordings carry instance identifiers and are not committed. `TestVerifySig_MixedRecordingsFailTheBinding` is the naive mix, not this row |

### Cells still open (2026-11-17), and why

| cell | state | why it is open |
|---|---|---|
| T1, Nitro, across days | the same commit on two machines agrees; values differ across commits and days | by design: the code changed (`a402827`, `1967749d`, `45887c8`, `ec14e63`, `f0fb80d`), and the test application's Dockerfile pins neither its base image nor its packages (`docs/results/aws-nitro-2026-11-17-shared-aws.md`) |
| T1, TDX, vCPUs vs memory | not separable on GCP | RTMR0 follows the machine type; GCP refused `c3-highmem-4` and `c3-highcpu-8` for TDX, and Google's list of TDX machine types (read 2026-11-18) has only `standard` shapes, where vCPUs and memory grow together |
| T8, TDX and SEV-SNP, live | **not applicable** on GCP | GCP offers no debug confidential VM. Running it live needs a host that can launch one (a KVM host of one's own); the refusal stays a code test |
| T10, Nitro and TDX | not applicable | one signed document, nothing to combine (⁶) |
| T10, SEV-SNP, as a code test | live only | the relay recordings carry instance identifiers and are not committed |

Closed by session 4 of 2026-11-16 (¹⁷), and what closed each:

| cell | was | closed by |
|---|---|---|
| T1, TDX and SEV-SNP, machine type | not measured | one guest per platform, only `set-machine-type` changed between boots: SEV-SNP MEASUREMENT follows the vCPU count (`n2d-standard-2`/`-4`/`highmem-2`), TDX RTMR0 follows the type (`c3-standard-4`/`-8`); `session4-*.jsonl`, T1 lines |
| T1, SEV-SNP, zone | not measured | a guest in `us-central1-b` gave the same five values as `us-central1-a` |
| T1, TDX, zone | not measured (the last cell that needed a guest) | a TDX guest of its own in `us-central1-b` (same image and type, tag `attestamp-ra`, reached through IAP) gave the same MRTD and RTMR0–2 as `us-central1-a`; measured on TDX, not inferred from SEV-SNP. `session4-intel-tdx.jsonl`, the `zone` line |
| T3, SEV-SNP, the whole initrd from two paths | needs VM | two `app` folders of different depth on the guest, the same initrd, booted and measured; Option 2 the same image and root hash |
| T7, TDX and SEV-SNP | code in part | `e2e/cvm/build-env-check.sh` on a guest of each, four verdicts each, written by the script |
| T8, TDX and SEV-SNP | code only, GCP not checked | the Compute API (`v1`, `alpha`) and `gcloud` checked: no debug confidential VM, so the live run is not applicable (above) |

¹ `docs/build/sev-snp.md`, "Variables", keeps the observations of the old out-of-repository scripts as a record. The old scripts' numbering of "the seven
SEV-SNP tests" was the scripts' own and is not the same as T1–T10 here.

² Nitro behaves differently here by design. Attestamp pins the toolchain and `build` **refuses**
to run under another patch version, rather than producing a different measurement. The underlying fact
is the same, but the product behaviour differs, so record both: refused, and — in a
separate run with the pin bypassed — the measurement that differs.

³ Since `31ed378` every build also uses `-buildvcs=false`, and `build` refuses a responder without it: measured,
identical source committed under two commits gave two different responder binaries without it
(`docs/build/aws-nitro.md`). T1 and T3 for Nitro must be run after that commit and after `b4ae86d`, which changed the
entrypoint inside the EIF, and `58b56624`, which changed the responder: **the baseline of 2026-11-12 was taken at
`ab55208`, after all three.**

⁴ `tee/parse/aws/measure.go` treats an all-zero PCR0 as debug (`Info.Debug`) and `Parse` fails when every PCR is
zero. A debug document is signed, so unlike TDX and SEV-SNP it cannot be made from the recorded one by flipping a bit;
it has to be recorded from a debug enclave. `e2e/debug-mode-check.sh` did that on 2026-11-15 (code at `7ceb9c8`; the
same test-application PCRs as the 2026-11-12 baseline). The debug document's signature and nonce verified and it was
marked `debug true`; then every PCR was zero and it was refused. The document is **not committed** as a fixture: its
signed `module_id` carries the EC2 instance ID, which may not be committed (`CONTRIBUTING.md`, "Secrets and certificates"). So T8 on Nitro stays
a live result, not a code test.

⁵ The nonce check is inside `verify` and has unit and command tests. `e2e/replay-check.sh` replayed a fresh document
against the next request's nonce on a live Nitro enclave on 2026-11-15 (the enclave `debug-mode-check.sh` relaunched;
no chain), and it was refused. The same check passed live on TDX and SEV-SNP (`e2e/cvm/in-guest-verify.sh`), so T9 is
now live on all three.

⁶ Nitro and TDX return one document from one TEE; there is nothing to combine.

⁸ Moved from "needs VM" to "code" on 2026-11-11 with one VM session per platform: the evidence was recorded twice on
the same guest, with the application changed by one byte, for each option (`docs/build/app-fixture-recording.md`).
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
other's attestation into its own report), but the only observation is the record of the old out-of-repository scripts, made with the SHA-512
binding and scripts that are gone. Until "Reproducing T10" is run, this repository cannot show it. *(Run on 2026-11-16: the relay reproduced, row T10 and ¹⁵.)*

¹⁰ The observations of the old out-of-repository scripts these SEV-SNP cells used to show, kept as a record (scripts no longer available, results
not reproducible; `docs/build/sev-snp.md`, "Variables"):

| row | old-script observation | what replaced it in the cell |
|---|---|---|
| T1 | the application, its cpio archive and the dm-verity image hashed the same on two GCP machines | our measurement of the image on two guests (2026-11-11) and the cpio golden tests |
| T2 | Go 1.22.2 and 1.23.4 gave different application binaries | our responder bytes under two Go patch versions (2026-11-12); the SEV-SNP measurement itself was "needs VM" until it was measured live on 2026-11-16 ¹⁵ |
| T3 | without `-trimpath`, two folders gave two binaries | our code test of the responder across checkout paths; the whole initrd was "needs VM" until it was measured live on 2026-11-16 ¹⁷ |
| T9 | replayed evidence was refused | our live replay refusal on the guest (2026-11-12) |
| T10 | a vTPM attestation from one VM with an SNP report from another verified | *(until 2026-11-16)* nothing yet: the cell says so, and "Reproducing T10" is the plan |

¹¹ **Local chain, not Sepolia.** The T4, T5 and T6 Nitro runs of 2026-11-15 used real enclaves on the two Nitro
instances and a registry on a local anvil 1.8.0 on the machine running the scripts (`NET=local`, anvil's development accounts as
signing keys), because no Sepolia ETH was available. What these rows test, that a registered value is trusted and
anything else is refused, does not depend on which chain holds the same contract; what they do not show is a public
network's behaviour (confirmation delays, gas, a public RPC). Every result line of these runs carries `"chain":"local"`
(`docs/results/aws-nitro-2026-11-15-local-chain.jsonl`), so a later Sepolia run, whose lines carry
`"chain":"sepolia"`, can be told apart and the cells can name both. The same run found the build-day difference noted
under T1 (`docs/results/aws-nitro-2026-11-15-local-chain.md`, "Finding").

¹² **The CVM trial of 2026-11-15** (`docs/results/trial-2026-11-15.md`), three results behind the TDX and SEV-SNP cells of
T1 and T4:
- **`launch` on real guests, for the first time.** On TDX and SEV-SNP, both options: install, reboot, `launch --check`
  (TDX: the CC event log replayed against the quote and GRUB's initrd digest was the built one; SEV-SNP: INITRD the
  built one, and for Option 2 PCR12 the predicted value), then `audit` and `verify` from another machine, exit 0, and
  the undo back to the stock boot.
- **Option 2 built with the real tools.** In a clean Ubuntu 24.04 WSL (squashfs-tools 4.6.1, cryptsetup 2.7.0) the
  build gave byte for byte the image and root hash the guests made on 2026-11-11 (`e29ec085…` for v1), `--verify`
  passed, and the PCR12 it predicts equals the recorded one (`470379c7…`).
- **CMDLINE and the disk UUID do not differ per guest.** They come with the image version: two guests made five days
  apart from `ubuntu-2404-noble-amd64-v20261105` gave the same TDX MRTD and RTMR0–2 and the same five SEV-SNP values
  for the same application. Since 2026-11-16 `gcp-cvm.sh` pins the exact image (`IMAGE=`, `32531e8`). Not measured:
  another machine type.

¹³ **The clean trial of 2026-11-16** (`docs/results/trial-2026-11-16-clean.md`, at `6a92a1f`): all three platforms followed
from the documents alone, by someone without the earlier sessions' knowledge, on a local anvil (`NET=local`). The
verdicts were the same on all three, and on both options of TDX and SEV-SNP:
- **T4**: `verify` exit 0 with `✓ matches`. On TDX and SEV-SNP the Auditor again registered the guest's own values (no
  rebuild on a guest of its own).
- **T5, by one byte**: one byte changed in the application, rebuilt and relaunched, `verify` exit 1. On Nitro too, in
  addition to flow 1's whole-image swap: PCR0 and PCR2 moved, PCR1 did not. TDX moved only RTMR2; SEV-SNP Option 1 only
  INITRD, Option 2 CMDLINE and PCR12.
- **Unregistered**: the genuine TEE against an empty registry, exit 1, on all three.
- **T9 from another machine**: `replay-check.sh` against the TDX and SEV-SNP guests over the network, exit 6 for the
  replay. A SEV-SNP guest on the **stock** boot answers with evidence every format refuses (exit 7, no initrd), where TDX's
  parses.
- **T1, TDX and SEV-SNP**: a third guest of `v20261105`, made a day later, gave the same values as 2026-11-15.
- **Nitro values at `6a92a1f`** (responder `5573b538…`), the first after `a402827` and `e57f02d`: PCR0 `2aacdf41…`, PCR1
  `4b4d5b36…` (unchanged since 2026-11-12), PCR2 `e5504779…`. Only the Auditor built, so this is **not** T1. These are
  stale too: `1967749d` changed the responder to `c61c988e…` the same day; the two-machine baseline was retaken at
  `23b83f9` (¹⁶).

The 20 script-written result lines are in `docs/results/trial-2026-11-16-clean/`.

¹⁴ **Session 2 of 2026-11-16, TDX on two guests** (`docs/results/sessions-2026-11-16/intel-tdx.md` and `.jsonl`, 18
lines, all as expected): two guests made in one call from `ubuntu-2404-noble-amd64-v20261105`. The Auditor's half of
`docs/build/tdx.md` (A1–A9) was followed for the first time, both options, and **D3 passed**: the Auditor's own build
equalled the Vendor's, both guests measured the same four values, `audit --vendor-tee` registered the Auditor's own
value, and the User's `verify` of the Vendor's guest against it gave exit 0; with one byte changed on the Vendor's side
only, `audit --vendor-tee` said `runs something else` and `verify` gave exit 1. One more negative control came
unplanned: a failed copy left the Auditor an empty application, and its build (initrd `7c738a30…`, not the Vendor's
`bc024887…`) was stopped by A4's comparison before anything was launched or registered. T2 and T3 used a Go program as the
application, since on TDX the measured thing is the initrd's content and `att-ra-responder` is not in it. T6 is flow 2
done by hand.

¹⁵ **Session 3 of 2026-11-16, SEV-SNP on two guests** (`docs/results/sessions-2026-11-16/amd-sev-snp.md`,
`amd-sev-snp-initrd.jsonl` and `amd-sev-snp-verity.jsonl`, 21 lines): two guests made in one call from the same image
as session 2. **D3 passed** for both options, with the negative control, as on TDX; a stale local copy gave a second
unplanned negative control, again stopped by A4 before registration. T2 and T6 as on TDX. **T10 reproduced**: the relay
verifies with A's values and finds A's registration; it is the one line whose `result` (accepted) differs from
`expected` (refused), and that difference is the finding. The relay recordings are kept locally, not committed.

¹⁶ **Session 1 of 2026-11-16, the Nitro baseline taken again at `23b83f9`** (`docs/results/aws-nitro-2026-11-16.md`
and `.jsonl`, 29 lines): the 2026-11-12 pair of `m6i.xlarge` started again and a new `c7g.xlarge`; responder
`c61c988e…`. The five chain-free scripts; for the first time with `ARM64_HOST`, so T1's arm64 side and T7's arm64
mirror ran. Two lines miss their expectation, both SSH aborts on this network, each followed by a passing line (the
file explains them). Every value in these cells carries its commit, because the responder's bytes, and with them
PCR0/PCR2, change with any change to code it can run or to the lines that code sits on; until the empty build ID of
2026-11-17 they changed with any change to a package it links, even code it never runs (`1967749d`;
`docs/build/ra-comparison.md`).

¹⁷ **Session 4 of 2026-11-16, T1, T3, T7 and T8 on the confidential VMs**
(`docs/results/sessions-2026-11-16/session4-t1-t3-t7-t8.md` and three `session4-*.jsonl`, 16 lines, all as expected; the TDX zone line was added later the same day, on a guest of its own):
binaries at `f826eca`, the T7 script at `483eef6`. Session 3's SEV-SNP Auditor guest for T3, T7 and the machine type;
one new SEV-SNP guest in `us-central1-b` for the zone; one new TDX guest for the machine type and T7. T1 and T3 used
`cmd/ra-responder` (`c61c988e…`) as the application, as T2 did. The machine type is now a required input of the
Auditor's half in both guides ("The same inputs"). T8 has no result lines: closed as not applicable, with the API
fields checked named in the session file and in `docs/build/ra-comparison.md`, "Debug mode".

¹⁸ **2026-11-17, the Nitro baseline for the empty build ID** (`docs/results/aws-nitro-2026-11-17.md` and `.jsonl`, 44
lines, two written by hand): three new instances, the x86_64 pair on a newer image (`ami-0f71b90bee7050a75`, release
2023.12.20261112) with the same nitro-cli 1.5.0 and docker; binaries at `83cb2dc`, responder `d6e3422d…` (amd64) /
`f581d95c…` (arm64). The five chain-free scripts, and for the first time `nitro-tools-check.sh` (gap 27, `docs/results/build-info-gaps-2026-11.md`: `send`
records the release and the docker and nitro-cli versions, `receive` compares the release, `build --config` the
versions). The lines that miss their expectation are SSH resets on this network and one bug in our own script, each
followed by a passing line; the file explains them. PCR1 did not move on either architecture; PCR0 and PCR2 did,
because the responder changed (`45887c8`, `ec14e63`).

¹⁹ **2026-11-17, second run: the Nitro baseline after the shared request package**
(`docs/results/aws-nitro-2026-11-17-shared-aws.md` and `.jsonl`, 32 lines, one written by hand; six more added later, from a `responder-check.sh` run with longer SSH retry waits): three new instances
on the same images as ¹⁸ (x86_64 `ami-0f71b90bee7050a75`, arm64 `ami-0ce2f8dd9b0fcfb35`, release 2023.12.20261112,
nitro-cli 1.5.0, docker 25.0.18); binaries and scripts at `6146c10`, responder `b9ae17f3…` (amd64) / `294efd1b…`
(arm64). The six chain-free scripts. The one line that misses its expectation is the arm64 responder step, whose SSH
was reset three times; run by hand, it was refused. PCR1 did not move; PCR0 and PCR2 did, because the responder
changed (`f0fb80d`), and that is the only difference from ¹⁸.

²⁰ **2026-11-18 and 2026-11-19, the CVM flow scripts** (`e2e/cvm/`, "Flow 1 on TDX and SEV-SNP" in `e2e/README.md`): two guests of one image per platform, `NET=local`, every step passing, 7 result lines per run. TDX Option 1 and Option 2: `docs/results/cvm-flow-rehearsal-2026-11-18.md`; `amd-sev-snp-verity` and `amd-sev-snp-initrd`: `docs/results/cvm-flow-rehearsal-2026-11-19.md`. The `verify` scenes were typed on the machine that ran the flow, not on another computer. Not yet run on Sepolia.

## Making the results machine-readable

The scripts today print `✅` / `❌` and set an exit code. That is enough for a person watching and
not enough to assemble a three-column table automatically.

Each verdict appends one line to `e2e-work/results/<platform>.jsonl` (done 2026-11-12: `result()` in `e2e/env.sh`,
called by every Nitro e2e script and, since 2026-11-18, by the CVM flow scripts in `e2e/cvm/flow1-auditor-registers/`; and the same fields from `e2e/cvm/in-guest-verify.sh`):

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

The platforms' rows do not have to be filled at the same time: each platform's rows can be filled in its own
session, on its own instances or guests, as long as every run writes this same contract. What a run needs is in `e2e/README.md`
(Nitro, and the scripted CVM flow) and in "Getting started" of `docs/build/tdx.md` and `docs/build/sev-snp.md`.

Rows each script writes: `arch-measurement-compare.sh` T1, T3; `toolchain-pin-check.sh` T2; `responder-check.sh` T3;
`build-env-check.sh` and `arch-record-check.sh` T7; flow 1 T4, T5; flow 2 T6; `in-guest-verify.sh` T4, T9;
`cvm/build-env-check.sh` T7; the CVM flow (`cvm/flow1-auditor-registers/`) T4, T5.
