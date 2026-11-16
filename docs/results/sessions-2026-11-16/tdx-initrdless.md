# TDX evidence from a guest booted without an initrd, measured 2026-11-16

The question: SEV-SNP refuses evidence from an initrdless boot (`internal/tee/parse/snp/measure.go`, exit 7: "GRUB
loaded no initrd"); TDX has no counterpart. What does the TDX path do with such evidence, all the way to `verify`?
Measured, not changed.

**Setup.** `d3-tdx-vendor` (session 2's guest, `ubuntu-2404-noble-amd64-v20261105`, `c3-standard-4`) after step 13
had put it back to Google's stock boot: kernel command line `root=PARTUUID=27c09ec4-… ro console=ttyS0,115200
panic=-1`, no initrd, `/boot/initrd-attestamp.img` absent. `att-ra-responder ra-serve --extractor tdx` and the local
binaries built at `1b981df`/`57b4350` (no change to the TDX parser since). Local anvil 1.8.0.

| step | command | result |
|---|---|---|
| 1 | `att-user ra-request` + `ra-parse --format intel-tdx` | **exit 0**: `signature ok`, `nonce ok`; MRTD `f21e6b48…`, RTMR0 `8847ec5d…`, RTMR1 `6e1a3f88…` (the same as the application boots), RTMR2 **`8d9317e9…`** |
| 2 | `att-user verify` against a registry holding the application's Option 1 values (RTMR2 `0f36ec64…`, registered with `att-auditor register`) | **exit 1**: `✗ nobody: this registry's approver never registered the value this guest runs` |
| 3 | `att-auditor audit --build-measurements <the application's values>` against this guest | **exit 1**: `RTMR2: values differ`, `the guest is not running what I built, so this audit is not registering anything` |
| 4 | `att-auditor audit` with `--tee-addr` only (no build values: registers what the guest attests) into a new registry | **exit 0**, `PASS: audited and registered` |
| 5 | `att-user verify` against that registry | **exit 0**, `trusted: this guest runs exactly what 0x7099… registered` |
| 6 | on the guest, `att-vendor launch --platform intel-tdx --check` | **exit 1**: `event log: RTMR2 has no GRUB initrd command, so no initrd was loaded` |

**What this shows.**

- Wherever the initrdless quote is compared with an **application's** values (2, 3), it is refused: RTMR2 covers the
  kernel command line and whether an initrd was loaded, so the absence moves it. The refusal says only "nobody
  registered" or "RTMR2: values differ"; it does not say that no initrd was loaded (only `launch --check`, 6, does).
- The parser itself accepts it (1), unlike SEV-SNP's.
- It passes to `trusted` only when someone has registered the initrdless values themselves (4, 5). `audit` without
  build values registers whatever the guest attests, with no check of what that boot loaded; the registration then
  vouches for a boot that carries no application.

What to do about it is an open question (`docs/plan/build-info-gaps.md`, "Design questions to review", Q2). The guest
was deleted afterwards (with its disk).
