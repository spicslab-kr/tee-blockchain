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

The guest was deleted afterwards (with its disk).

## Measured again after the fix, at `fa871e1`

The fix (`fix(cmd/auditor): refuse to audit a TDX or SEV-SNP guest without --build-measurements`): `audit` on a CVM
format needs a value the Auditor built. The parser was not changed, because TDX evidence is the quote alone and the
event log that shows whether an initrd was loaded stays on the guest (`docs/plan/build-info-gaps.md`, Q2: the real fix
puts that log into the evidence, in a later session). Measured on `trial1116-tdx-vendor`, another guest of the same
image on the same stock boot (`root=PARTUUID=… panic=-1`, no initrd); its quote gave the same RTMR2 `8d9317e9…`.
Binaries at `fa871e1`, local anvil 1.8.0.

| step | before (at `1b981df`/`57b4350`) | after (at `fa871e1`) |
|---|---|---|
| 1 `ra-parse` | exit 0 | **exit 0**, unchanged (the parser cannot see the missing initrd) |
| 2 `verify` against the application's registration | exit 1, `nobody` | **exit 1**, `nobody`, unchanged |
| 3 `audit` with the application's build values | exit 1, `RTMR2: values differ` | **exit 1**, unchanged |
| 4 `audit` with `--tee-addr` only | exit 0, **registered** | **exit 1, refused before any request**: `refusing to audit a intel-tdx guest without --build-measurements: without a value I rebuilt myself, this would register whatever the guest reports, and a boot that never measured the application (one without an initrd, for example) would be registered and then trusted. Rebuild and launch on your own guest, and pass its out/measurements.json (docs/build/tdx.md, "The Auditor's half")` |
| 5 `verify` against that registry | exit 0, **trusted** | **exit 1**, `nobody`: nothing was registered in step 4 |
| 6 `launch --check` on the guest | exit 1, `no initrd was loaded` | **exit 1**, the same (given the Option 1 build value) |

Row 4 is the change, and row 5 follows from it: the path by which an initrdless boot became `trusted` is closed in
`audit`. **What it does not close:** an Auditor that calls `register` by hand with the initrdless values, or passes
them as its "build" values, still registers them. That is not a bypassed check but an Auditor vouching for something
it did not build, which the trust model already has to assume does not happen; it is written down so that nobody
reads the `audit` refusal as a guarantee.
