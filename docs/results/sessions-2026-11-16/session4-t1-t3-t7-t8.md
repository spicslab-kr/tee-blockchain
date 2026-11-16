# Session 4 of 2026-11-16: T1, T3, T7 and T8 on the confidential VMs

Run by Claude with the operator present. Binaries built at commit `e319431` (`att-auditor` `3d4b6635…`, `att-vendor`
`3a30019c…`, go1.26.1, `CGO_ENABLED=0 -trimpath -buildvcs=false`); the T7 script was committed as `e9af1c9` during
the session, byte-identical to the copy that ran (`95e915e5…`). 15 result lines, every one with `result` equal to
`expected`: 8 in `session4-amd-sev-snp-initrd.jsonl`, 1 in `session4-amd-sev-snp-verity.jsonl`, 6 in
`session4-intel-tdx.jsonl`. The T7 lines were written by `e2e/cvm/build-env-check.sh`, the others by hand
(`"script":"manual: …"`). T8 has no lines: it was closed as not applicable, not run.

## Setup

| guest | made | used for |
|---|---|---|
| `d3-snp-auditor` | session 3, `ubuntu-2404-noble-amd64-v20261105`, `n2d-standard-2`, `us-central1-a` | T7 and T3 on SEV-SNP; T1 machine type (changed to `n2d-standard-4`, then `n2d-highmem-2`, then back) |
| `t1zone-snp-vendor` | this session, same image, `n2d-standard-2`, **`us-central1-b`** | T1 zone |
| `t1type-tdx-vendor` | this session, same image, `c3-standard-4`, `us-central1-a` | T1 machine type on TDX (changed to `c3-standard-8`, then back); T7 on TDX |

The application in T1 and T3 was the one session 2 and 3 used for T2 and T3: `cmd/ra-responder` built with go1.26.1
(`c61c988e…`), alone in `app/`, Option 1 on the image's own base initrd (`4703ec07…` on every guest).

State at the end of the session: all four GCP guests **stopped** (`d3-snp-vendor` was not started). The three that
booted an Attestamp entry still had it as the GRUB default and `GRUB_FORCE_PARTUUID` commented out.

**Afterwards, the same day: all four deleted, without the undo.** On the operator's decision, `d3-snp-vendor`,
`d3-snp-auditor`, `t1zone-snp-vendor` and `t1type-tdx-vendor` were deleted with `gcloud compute instances delete`,
after `d3-snp-auditor` had been used once more for the IAP check (`e2e/cloud/README.md`, "SSH to the guests"). Step 13
(the undo of the boot entry and of `/var/lib/attestamp`) was **not** run on any of them: the guests were being
deleted, so there was nothing to restore. Each boot disk was set to auto-delete, and afterwards the project listed no
instances, no disks, no snapshots and no custom images. Every value measured on them is in this file and the
`session4-*.jsonl` lines; new guests are made with `e2e/cloud/gcp-cvm.sh`.

## T7: the build environment on a guest

`e2e/cvm/build-env-check.sh`, new, runs on the guest. What it adds over the Nitro script: the Vendor's `att-vendor
send` on the guest fills in `build_env` itself, and the Auditor's `receive` is checked against that record.

| check | `amd-sev-snp-initrd` (`d3-snp-auditor`) | `intel-tdx` (`t1type-tdx-vendor`) |
|---|---|---|
| 0. `send` without `build_env` records the guest's own | `linux/amd64 go1.26.1 trimpath=true` | the same |
| 1. a config declaring `arm64` | refused, exit 1, naming both | the same |
| 2. the same with `--ignore-env` | warning, exit 0 | the same |
| 3. the config `send` recorded | `build env matches the Vendor's`, exit 0 | the same |

The first run used a script whose printout of the recorded `build_env` showed only two lines; the verdicts were the
same. The printout was fixed and the script run again; the lines kept are from the second run.

## T3: SEV-SNP, only the build path differs

The same binary, from two checkout paths (session 2), put into `~/t3/app` and `/tmp/t3/a/much/deeper/build/path/app`
on the guest and built there with `att-auditor build`:

| | path 1 | path 2 |
|---|---|---|
| Option 1 initrd | `48e09d59…` | **the same**; `build-value.json` byte-identical |
| Option 2 `app.img`, hash tree | `1787fbe5…`, `f4eb43d6…` | **the same** |
| Option 2 root hash, predicted PCR12 | `4a517772…`, `0932dbed…` | **the same** |

The Option 1 initrd booted on the guest: `launch --check` passed with INITRD `48e09d59…` (equal to session 3's T2
build of the same binary). Option 2 was built, not booted. The deeper path's build logs were in `/tmp` and were lost
in the reboot; their hashes and output are in the session's terminal record.

## T1: machine type and zone

Each machine-type change: stop, `gcloud compute instances set-machine-type`, start, `launch --check`. Nothing on the
disk changed between the boots, so only the VM's shape differs. `set-machine-type` works on a stopped Confidential VM.

**SEV-SNP** (`amd-sev-snp-initrd`):

| guest | MEASUREMENT | PCR4, KERNEL, CMDLINE, INITRD |
|---|---|---|
| `n2d-standard-2`, 2 vCPUs, 8 GB, `us-central1-a` | `f6790085…` | `5b65d2d3…`, `3cf8c2c3…`, `80dfd7d3…`, `48e09d59…` |
| `n2d-standard-4`, 4 vCPUs, 16 GB | **`2279d213…`** | the same |
| `n2d-highmem-2`, 2 vCPUs, 16 GB | `f6790085…` | the same |
| a new guest, `n2d-standard-2`, **`us-central1-b`** | `f6790085…` | the same |

The vCPU count moves MEASUREMENT; the memory and the zone did not. This fits the structure of SEV-SNP: the launch
digest covers one initial register state (VMSA) per vCPU.

**TDX** (`intel-tdx`, Option 1, initrd SHA-384 `162372c6…`):

| guest | MRTD | RTMR0 | RTMR1 | RTMR2 |
|---|---|---|---|---|
| new guest, `c3-standard-4`, 4 vCPUs, 16 GB | `f21e6b48…` | `8847ec5d…` | `6e1a3f88…` | `4ea92ca9…` |
| the same guest at `c3-standard-8`, 8 vCPUs, 32 GB | the same | **`eba48352…`** | the same | the same |

The `c3-standard-4` values equal session 2's guests' for the same application, so a third TDX guest of this image
agreed again. `c3-highmem-4` and `c3-highcpu-8`, tried to separate vCPUs from memory, were refused by GCP for a TDX
VM ("Confidential Instance Config can only be set on supported and compatible Confidential VM machine types"). Zone on
TDX: not measured.

**Consequence, written into both guides ("The same inputs"):** the Auditor's guest must have the Vendor's machine
type. On SEV-SNP the vCPU count is what matters; on TDX it could not be narrowed beyond the type.

## T8: a debug guest on GCP

**Closed as not applicable on GCP.** A debug confidential VM cannot be requested:

- the Compute API's `ConfidentialInstanceConfig` (discovery documents for `v1` and `alpha`, revision 20261106) has
  `enableConfidentialCompute` and `confidentialInstanceType` (`SEV`, `SEV_SNP`, `TDX`, …); `alpha` adds only
  `confidentialParavisorConfig` (an interrupt mode and a vTPM type). No field in either API's schemas has "debug" in
  its name.
- `gcloud compute instances create --help` (SDK 592.0.0) offers `--confidential-compute-type` and no debug option.

What is live: every report and quote these guests produced went through the parser that refuses POLICY.DEBUG and
TDATTRIBUTES.DEBUG, and passed, so each had the bit clear. The refusal itself stays tested in code
(`TestParse_DebugGuestIsRefused`, `TestParse_DebugTDIsRefused`) on a recorded document with the bit set, and it stays
in the parser for hosts that can launch a debug guest (`docs/build/ra-comparison.md`, "Debug mode").

## What was done differently

- After session 3's undo, `GRUB_FORCE_PARTUUID` was active again on `d3-snp-auditor`, and the first `launch --reboot`
  refused to install. The guide's own fix (comment the line out, `update-grub`) was applied and `launch` run again.
- Right after a start, SSH failed one to three times while the guest was still booting (at `n2d-highmem-2` the boot
  took over a minute). Each was retried, not investigated. Every restart gave the guest a new external address, so
  PuTTY asked to replace a cached host key; it was answered `y`.
