# Intel TDX, session 2 of 2026-11-16: the Auditor's own rebuild ([D3](../trial-2026-11-16-clean.md#every-difference-found)), T2, T3, T6

Run on 2026-11-16 at commit `a429597` (binaries built there; the `self-register` wording change
`fix(cmd/vendor)` came during the run and only changes its text). 18 result lines in `intel-tdx.jsonl`, written by hand
in the results contract (`"script":"manual: …"`), every one with `result` equal to `expected`. Local anvil 1.8.0
(`NET=local`), verifier on the operator's Windows machine.

## Setup

| what | value |
|---|---|
| guests | `d3-tdx-vendor` and `d3-tdx-auditor`, made in **one** call: `PREFIX=d3-tdx IMAGE=ubuntu-2404-noble-amd64-v20261105 bash e2e/cloud/gcp-cvm.sh tdx create vendor auditor` |
| image | `ubuntu-2404-noble-amd64-v20261105` (exact version; the family `ubuntu-2404-lts-amd64` pointed at it), kernel `7.0.0-1017-gcp`, `c3-standard-4`, `us-central1-a` |
| guest binaries | `att-vendor` `620a45d7…`, `att-auditor` `1ad5df46…`, `att-ra-responder` `c61c988e…` (go1.26.1, `CGO_ENABLED=0 -trimpath -buildvcs=false`) |
| firewall | `attestamp-ra` was set to an earlier address of the operator's; with the operator's approval this machine's `/32` was **added** (two single addresses, nothing wider) |
| state now | both **stopped** (TERMINATED); the boot entry undone (step 13) before the stop, `/opt/app-verity` left in place |

## D3: the Auditor's half, followed as written

`docs/build/tdx.md`, "The Auditor's half", A1 to A9, both options. **Passed, with the negative control.**

| | Option 1 | Option 2 |
|---|---|---|
| base initrd | each guest's own `/boot` file, `4703ec07…` on both | the Vendor's `base-initrd-v.img`, `f6c6477c…`, copied to the Auditor |
| Vendor's build | initrd `bc024887…`, archive `b740e415…` | initrd `18298ad3…`, root hash `5dff1f1d…` |
| Auditor's build (A4) | **the same** `bc024887…` | **the same** `18298ad3…`, `5dff1f1d…` |
| both guests' values | MRTD `f21e6b48…`, RTMR0 `8847ec5d…`, RTMR1 `6e1a3f88…`, RTMR2 `0f36ec64…`, **equal** | the same MRTD and RTMR0-1, RTMR2 `517d1e67…`, **equal** |
| A8 `audit --vendor-tee` | `identical: the guest runs exactly what I built`, `same: the Vendor's guest runs exactly what I built`, `PASS` | the same |
| A9 `verify` of the **Vendor's** guest against the Auditor's registry | **exit 0**, `trusted: this guest runs exactly what 0x7099… registered` | **exit 0**, the same |
| negative: one byte changed on the Vendor's side only | Vendor RTMR2 `f36c5202…`; `audit --vendor-tee`: `the Vendor's guest runs something else` (`RTMR2: values differ`), the Auditor's own value registered, exit 0; `verify`: **exit 1**, nobody registered | root hash `15e18e5e…`, Vendor RTMR2 `e98d1547…`; the same two results |

MRTD and RTMR0-2 for Option 1 equal those of the two earlier trials (2026-11-15 and the clean trial of 2026-11-16):
four guests of one image, made over six days, measured the same.

**An unplanned negative control: the wrong input was caught before registering.** *(Rewritten later on 2026-11-16, `70a7dcf`: this paragraph was first headed "What the procedure got wrong, and what was changed", with the same facts. The 18th JSONL line was added then, and the count in the first paragraph went from 17 to 18.)* A2's `gcloud compute scp
--recurse vendor-app <guest>:app` failed with Windows' `pscp` (`unable to open app`) and left an empty `app` folder,
so the Auditor's first build was of an **empty** application: initrd `7c738a30…` instead of the Vendor's
`bc024887…`. A4's comparison of `initrd_sha384` with the Vendor's stopped the procedure there; nothing was launched or
registered. That is the design working as meant: an Auditor whose inputs differ from the Vendor's gets another value
and does not vouch for it. It is the 18th line of `intel-tdx.jsonl` (T5, `rejected`, as expected). The copy command is
also changed in both docs (copy to the home folder, then `mv vendor-app app` on the guest), so the same slip does not
cost a rebuild.

**What was done differently from the page, and why.** Every guest command was sent through
`gcloud compute ssh --command` rather than typed. `launch --reboot` was started with `nohup … &` because two plain
attempts died with `Network error: Software caused connection abort` before the command ran (this network aborts bursts
of new SSH connections; `e2e/env.sh` documents the same). The first `pscp` to each guest needed the host key answered
`y` (piped). Waiting for each reboot used the boot ID, as step 7 says.

## T2: only the Go version differs

The application is a Go program (`cmd/ra-responder`) built with all three settings, once with go1.26.1 and once with
go1.26.2, each alone in `app/`, built with `att-auditor build --option 1` and booted on the Auditor's guest.

| | go1.26.1 | go1.26.2 |
|---|---|---|
| binary | `c61c988e…` | `24724239…` |
| initrd | `162372c6…` | `e94699bd…` |
| RTMR2 on the guest | `4ea92ca9…` | **`1c181218…`** |
| MRTD, RTMR0, RTMR1 | equal | equal |

Also: an `att-vendor` built with go1.26.2 refuses `build` on the guest (exit 1), as on Nitro. Its message gives the
responder's bytes as the reason, which is Nitro's reason; on TDX the responder is not measured. *(Fixed since in
`718b3b2`: the refusal now gives each platform's own reason; noted 2026-11-20.)*

## T3: only the build path differs

The same commit cloned into two folders of different depth and built with all three settings: identical binaries
(`c61c988e…`, also equal to a build from the repository's own path). On the guest, the second build put into a much
deeper folder (`/tmp/t3/a/much/deeper/build/path/app`) gave the same initrd `162372c6…` as the first, and that initrd
booted with RTMR2 `4ea92ca9…`.

## T6: the Vendor self-registers

With the Vendor's guest running the one-byte-changed application: the Vendor deployed its own registry (approver
anvil account 0) and `self-register`ed (exit 0). `verify` against the Vendor's registry alone: exit 0, `trusted` (the
approver is the Vendor itself). `audit --vendor-tee`: `the Vendor's guest runs something else`. `verify` with both the
Vendor's and the Auditor's registries required: **exit 1**, `only 1 of 2 registries vouch`. The same verdicts as
Nitro's flow 2.
